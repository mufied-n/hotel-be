package guest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
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

	// 1. Cek rate limit / cooldown 60 detik
	latest, err := s.store.GetLatestActiveChallenge(ctx, normEmail)
	if err == nil && latest != nil {
		if now.Sub(latest.CreatedAt) < 60*time.Second {
			return 0, ErrRateLimited
		}
	}

	// 2. Buat OTP 6 digit acak aman
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

	if err := s.store.CreateChallenge(ctx, challenge); err != nil {
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

// VerifyChallenge memvalidasi kode OTP dan menerbitkan token sesi baru.
func (s *DefaultService) VerifyChallenge(ctx context.Context, email, code string) (string, *GuestSession, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	cleanCode := strings.TrimSpace(code)
	if len(cleanCode) != 6 {
		return "", nil, ErrInvalidOrExpiredCode
	}

	now := s.nowFunc()

	challenge, err := s.store.GetLatestActiveChallenge(ctx, normEmail)
	if err != nil || challenge == nil {
		return "", nil, ErrInvalidOrExpiredCode
	}

	if challenge.VerifiedAt != nil || now.After(challenge.ExpiresAt) {
		return "", nil, ErrInvalidOrExpiredCode
	}

	if challenge.Attempts >= challenge.MaxAttempts {
		return "", nil, ErrMaxAttemptsExceeded
	}

	// Komparasi waktu konstan (OWASP API Top 10)
	inputHash := HashString(cleanCode)
	match := subtle.ConstantTimeCompare([]byte(challenge.CodeHash), []byte(inputHash)) == 1

	if !match {
		newAttempts := challenge.Attempts + 1
		_ = s.store.UpdateChallengeAttempts(ctx, challenge.ID, newAttempts)
		if newAttempts >= challenge.MaxAttempts {
			return "", nil, ErrMaxAttemptsExceeded
		}
		return "", nil, ErrInvalidOrExpiredCode
	}

	// Tandai challenge sudah terverifikasi
	_ = s.store.MarkChallengeVerified(ctx, challenge.ID, now)

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

	if err := s.store.CreateSession(ctx, session); err != nil {
		return "", nil, fmt.Errorf("guest: failed to create session: %w", err)
	}

	return rawToken, session, nil
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

	_ = s.store.TouchSession(ctx, session.ID, now, newExpiresAt)
	session.LastActiveAt = now
	session.ExpiresAt = newExpiresAt

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
