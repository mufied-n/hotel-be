package guest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
)

// OTPNotifier mendefinisikan adapter untuk mengirim email OTP ke tamu.
type OTPNotifier interface {
	SendGuestOTP(ctx context.Context, email, otpCode string) error
}

// Service mendefinisikan use cases untuk autentikasi tamu dan "Booking Saya".
type Service interface {
	RequestChallenge(ctx context.Context, email string) (cooldownSec int, err error)
	VerifyChallenge(ctx context.Context, email, code string) (token string, session *GuestSession, err error)
	ValidateSession(ctx context.Context, rawToken string) (*GuestSession, error)
	RevokeSession(ctx context.Context, rawToken string) error
	GetSessionProfile(ctx context.Context, session *GuestSession) (*ProfileView, error)
	ListBookings(ctx context.Context, email, status string, limit int) ([]BookingSummary, error)
	GetBookingDetail(ctx context.Context, email, bookingID string) (*BookingDetail, error)
	GetBookingReceipt(ctx context.Context, email, bookingID string) (*ReceiptDTO, error)
	GenerateCalendarICS(receipt *ReceiptDTO) ([]byte, error)
}


// DefaultService mengimplementasikan Service interface.
type DefaultService struct {
	store    Store
	notifier OTPNotifier
	log      *slog.Logger
	nowFunc  func() time.Time
}

// NewService membuat instance baru DefaultService.
func NewService(store Store, notifier OTPNotifier, log *slog.Logger) *DefaultService {
	if log == nil {
		log = slog.Default()
	}
	return &DefaultService{
		store:    store,
		notifier: notifier,
		log:      log,
		nowFunc:  time.Now,
	}
}

// RequestChallenge memproses permintaan OTP 6 digit baru dengan proteksi rate limit dan anti-enumeration.
func (s *DefaultService) RequestChallenge(ctx context.Context, email string) (int, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(normEmail); err != nil || !strings.Contains(normEmail, ".") {
		return 0, ErrInvalidEmail
	}

	now := s.nowFunc()

	// 1. Buat OTP 6 digit acak aman
	otpCode, err := generateOTP()
	if err != nil {
		return 0, fmt.Errorf("guest: failed to generate OTP: %w", err)
	}

	codeHash := HashString(otpCode)
	challenge := &Challenge{
		Email:       normEmail,
		CodeHash:    codeHash,
		Attempts:    0,
		MaxAttempts: 3,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
	}

	// 2. Simpan secara atomik dengan proteksi cooldown 60 detik (BE-R03)
	if err := s.store.CreateChallengeWithCooldown(ctx, challenge, 60*time.Second); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return 0, ErrRateLimited
		}
		return 0, fmt.Errorf("guest: failed to store challenge: %w", err)
	}

	// 3. Kirim OTP via notifier
	if s.notifier != nil {
		if err := s.notifier.SendGuestOTP(ctx, normEmail, otpCode); err != nil {
			s.log.ErrorContext(ctx, "guest.otp_dispatch_failed", "email", normEmail, "err", err)
		}
	}

	return 60, nil
}

// VerifyChallenge memvalidasi kode OTP dan menerbitkan token sesi baru secara atomik (BE-R03).
func (s *DefaultService) VerifyChallenge(ctx context.Context, email, code string) (string, *GuestSession, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	cleanCode := strings.TrimSpace(code)
	if len(cleanCode) != 6 {
		return "", nil, ErrInvalidOrExpiredCode
	}

	now := s.nowFunc()

	// Buat token sesi acak 32-byte
	rawToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", nil, fmt.Errorf("guest: failed to generate session token: %w", err)
	}

	session := &GuestSession{
		GuestEmail:   normEmail,
		TokenHash:    tokenHash,
		ExpiresAt:    now.Add(24 * time.Hour), // 24 jam idle timeout
		LastActiveAt: now,
		CreatedAt:    now,
	}

	inputHash := HashString(cleanCode)
	createdSession, err := s.store.VerifyAndConsumeChallenge(ctx, normEmail, inputHash, now, session)
	if err != nil {
		return "", nil, err
	}

	return rawToken, createdSession, nil
}

// ValidateSession memeriksa validitas token sesi dan memperpanjang masa aktif.
func (s *DefaultService) ValidateSession(ctx context.Context, rawToken string) (*GuestSession, error) {
	cleanToken := strings.TrimSpace(rawToken)
	if cleanToken == "" {
		return nil, ErrSessionNotFound
	}

	tokenHash := HashString(cleanToken)
	session, err := s.store.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil || session == nil {
		return nil, ErrSessionNotFound
	}

	now := s.nowFunc()
	if now.After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Perpanjang idle TTL 24 jam, dibatasi max 7 hari dari pembuatan sesi
	newExpiresAt := now.Add(24 * time.Hour)
	maxExpiry := session.CreatedAt.Add(7 * 24 * time.Hour)
	if newExpiresAt.After(maxExpiry) {
		newExpiresAt = maxExpiry
	}

	if err := s.store.TouchSession(ctx, session.ID, now, newExpiresAt); err != nil {
		s.log.WarnContext(ctx, "guest.touch_session_failed", "session_id", session.ID, "err", err)
	} else {
		session.LastActiveAt = now
		session.ExpiresAt = newExpiresAt
	}

	return session, nil
}

// RevokeSession mencabut sesi tamu yang aktif (logout).
func (s *DefaultService) RevokeSession(ctx context.Context, rawToken string) error {
	cleanToken := strings.TrimSpace(rawToken)
	if cleanToken == "" {
		return nil
	}
	tokenHash := HashString(cleanToken)
	return s.store.DeleteSessionByTokenHash(ctx, tokenHash)
}

// GetSessionProfile mengembalikan profil ringkas dan jumlah reservasi aktif.
func (s *DefaultService) GetSessionProfile(ctx context.Context, session *GuestSession) (*ProfileView, error) {
	if session == nil {
		return nil, ErrSessionNotFound
	}
	count, err := s.store.CountActiveBookingsByEmail(ctx, session.GuestEmail)
	if err != nil {
		count = 0
	}
	return &ProfileView{
		Email:               session.GuestEmail,
		ActiveBookingsCount: count,
		LastActiveAt:        session.LastActiveAt,
		ExpiresAt:           session.ExpiresAt,
	}, nil
}

// ListBookings menyajikan daftar pesanan milik tamu terotentikasi.
func (s *DefaultService) ListBookings(ctx context.Context, email, status string, limit int) ([]BookingSummary, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.store.ListBookingsByEmail(ctx, normEmail, status, limit)
}

// GetBookingDetail menyajikan detail pemesanan lengkap dan aksi yang diizinkan (IDOR safe).
func (s *DefaultService) GetBookingDetail(ctx context.Context, email, bookingID string) (*BookingDetail, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	cleanBookingID := strings.TrimSpace(bookingID)

	detail, err := s.store.GetBookingDetailByEmail(ctx, normEmail, cleanBookingID)
	if err != nil || detail == nil {
		return nil, ErrBookingNotFound
	}

	detail.AllowedActions = computeAllowedActions(detail.Status)
	return detail, nil
}

// GetBookingReceipt mengambil faktur resmi / tanda terima pemesanan yang valid dan telah lunas.
func (s *DefaultService) GetBookingReceipt(ctx context.Context, email, bookingID string) (*ReceiptDTO, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	cleanBookingID := strings.TrimSpace(bookingID)

	receipt, err := s.store.GetBookingReceiptData(ctx, normEmail, cleanBookingID)
	if err != nil || receipt == nil {
		return nil, ErrBookingNotFound
	}

	// Status guard (BR-F04-01 & FR-03): Hanya booking yang lunas / confirmed / checked-in / checked-out yang memiliki receipt resmi
	if receipt.Status != "confirmed" && receipt.Status != "checked_in" && receipt.Status != "checked_out" {
		return nil, ErrReceiptNotAvailable
	}

	return receipt, nil
}

// GenerateCalendarICS membangun dokumen iCalendar RFC 5545 standar (Ponytail: Go standard library murni).
func (s *DefaultService) GenerateCalendarICS(receipt *ReceiptDTO) ([]byte, error) {
	if receipt == nil {
		return nil, errors.New("guest: receipt cannot be nil for ics generation")
	}

	checkInClean := strings.ReplaceAll(receipt.StayDetails.CheckInDate, "-", "")
	checkOutClean := strings.ReplaceAll(receipt.StayDetails.CheckOutDate, "-", "")

	summary := fmt.Sprintf("Menginap di Pulang ke Uttara (%s)", receipt.RoomItem.RoomTypeName)
	description := fmt.Sprintf("Kode Reservasi: %s\nTamu: %s\nKamar: %s (%d kamar)\nCheck-in: %s 14:00 WIB\nCheck-out: %s 12:00 WIB\nAlamat: Jl. Kaliurang Km 5.6 No. 1, Sleman, Yogyakarta\nTelepon: +62 274 5022888",
		receipt.BookingReference,
		receipt.GuestDetails.Name,
		receipt.RoomItem.RoomTypeName,
		receipt.RoomItem.NumRooms,
		receipt.StayDetails.CheckInDate,
		receipt.StayDetails.CheckOutDate,
	)

	nowUTC := s.nowFunc().UTC().Format("20060102T150405Z")

	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//Pulang ke Uttara//Hotel Booking Engine v1.0//ID\r\n")
	sb.WriteString("CALSCALE:GREGORIAN\r\n")
	sb.WriteString("METHOD:PUBLISH\r\n")
	sb.WriteString("BEGIN:VTIMEZONE\r\n")
	sb.WriteString("TZID:Asia/Jakarta\r\n")
	sb.WriteString("BEGIN:STANDARD\r\n")
	sb.WriteString("DTSTART:19700101T000000\r\n")
	sb.WriteString("TZOFFSETFROM:+0700\r\n")
	sb.WriteString("TZOFFSETTO:+0700\r\n")
	sb.WriteString("TZNAME:WIB\r\n")
	sb.WriteString("END:STANDARD\r\n")
	sb.WriteString("END:VTIMEZONE\r\n")
	sb.WriteString("BEGIN:VEVENT\r\n")
	sb.WriteString(fmt.Sprintf("UID:booking-%s@pulangkeuttara.id\r\n", receipt.BookingID))
	sb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", nowUTC))
	sb.WriteString(fmt.Sprintf("DTSTART;TZID=Asia/Jakarta:%sT140000\r\n", checkInClean))
	sb.WriteString(fmt.Sprintf("DTEND;TZID=Asia/Jakarta:%sT120000\r\n", checkOutClean))
	sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(summary)))
	sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(description)))
	sb.WriteString("LOCATION:Pulang ke Uttara, Jl. Kaliurang Km 5.6 No. 1, Caturtunggal, Depok, Sleman, D.I. Yogyakarta 55281\r\n")
	sb.WriteString("STATUS:CONFIRMED\r\n")
	sb.WriteString("BEGIN:VALARM\r\n")
	sb.WriteString("ACTION:DISPLAY\r\n")
	sb.WriteString("DESCRIPTION:Pengingat Check-in: Besok jadwal check-in di Pulang ke Uttara (14:00 WIB)\r\n")
	sb.WriteString("TRIGGER:-P1D\r\n")
	sb.WriteString("END:VALARM\r\n")
	sb.WriteString("END:VEVENT\r\n")
	sb.WriteString("END:VCALENDAR\r\n")

	return []byte(sb.String()), nil
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `;`, `\;`)
	s = strings.ReplaceAll(s, `,`, `\,`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}


func computeAllowedActions(status string) AllowedActions {
	switch status {
	case "pending":
		return AllowedActions{CanPay: true, CanCancel: true, CanRequestAssistance: true}
	case "confirmed":
		return AllowedActions{CanCancel: true, CanDownloadReceipt: true, CanRequestAssistance: true}
	case "checked_in":
		return AllowedActions{CanDownloadReceipt: true, CanRequestAssistance: true}
	case "checked_out":
		return AllowedActions{CanDownloadReceipt: true}
	default:
		return AllowedActions{}
	}
}

// HashString menghitung SHA-256 dari string input.
func HashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func generateOTP() (string, error) {
	var n uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &n)
	if err != nil {
		return "", err
	}
	code := 100000 + (n % 900000)
	return fmt.Sprintf("%06d", code), nil
}

func generateSessionToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := "gst_sess_" + hex.EncodeToString(b)
	return raw, HashString(raw), nil
}
