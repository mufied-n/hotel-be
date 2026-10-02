package booking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
)

var (
	phoneRegex   = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	arrivalRegex = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
)

func generateGuestToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return "gst_" + hex.EncodeToString(buf)
}

// Ports (driven) yang dikonsumsi domain — didefinisikan di sisi consumer (§8.2).
type (
	// InventoryTx digunakan dalam SATU transaksi yang sama dengan insert booking:
	// lock baris inventory FOR UPDATE + decrement available (desain §12.3).
	InventoryTx interface {
		// LockAndDecrement mengunci baris inventory [from, to) terurut ASC
		// (anti-deadlock), memverifikasi ketersediaan, lalu decrement.
		LockAndDecrement(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error
		// Increment mengembalikan ketersediaan (cancel / release-hold / no-show).
		Increment(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error
		// InsertBookingWithHold menyimpan booking status pending + hold TTL + detail per malam.
		InsertBookingWithHold(ctx context.Context, b *Booking, quotes []rates.Quote, holdExpiresAt time.Time) error
		// GetForUpdate membaca booking dengan FOR UPDATE (anti-race webhook).
		GetForUpdate(ctx context.Context, id string) (Booking, error)
		// UpdateStatus mentransisikan status tanpa validasi (validasi di state machine).
		UpdateStatus(ctx context.Context, id string, to Status) error
		// PickAndAssignRooms memilih sejumlah kamar fisik bebas (bukan overlap) dari tipe
		// terkait dan memasang assignment — DB menolak via GiST exclusion bila
		// bentrok (desain §13). Return ErrNoRoomAvailable bila tak ada.
		PickAndAssignRooms(ctx context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error)
		// GetRoomAssignments membaca seluruh nomor kamar yang sudah ter-assign (idempotency check-in).
		GetRoomAssignments(ctx context.Context, bookingID string) ([]string, error)
	}
	// Reader adalah port baca tanpa transaksi (query biasa).
	Reader interface {
		Get(ctx context.Context, id string) (Booking, error)
	}
	// PaymentGateway adalah port pembayaran (§8.2). Implementasi vendor
	// (Midtrans/Stripe/Xendit) ditulis di internal/adapter.
	PaymentGateway interface {
		CreateCharge(ctx context.Context, b Booking, amountMinor int64, currency string) (ChargeResult, error)
	}
	// Notifier adalah port notifikasi (email konfirmasi, dsb.).
	Notifier interface {
		SendBookingConfirmed(ctx context.Context, b Booking) error
	}
)

// ChargeResult adalah hasil permintaan pembayaran — bahasa domain, bukan
// bentuk API vendor (anti-corruption layer, §8.4).
type ChargeResult struct {
	PaymentURL string `json:"payment_url"`
	Reference  string `json:"reference"`
}

// CreateInput adalah parameter use case Create.
type CreateInput struct {
	QuoteID              string    `json:"quote_id,omitempty"`
	RoomTypeID           string    `json:"room_type_id"`
	CheckIn              time.Time `json:"check_in"`  // YYYY-MM-DD
	CheckOut             time.Time `json:"check_out"` // YYYY-MM-DD
	NumRooms             int       `json:"num_rooms"`
	NumGuests            int       `json:"num_guests"`
	GuestName            string    `json:"guest_name"`
	GuestEmail           string    `json:"guest_email"`
	GuestPhone           string    `json:"guest_phone,omitempty"`
	EstimatedArrivalTime string    `json:"estimated_arrival_time,omitempty"`
	SpecialRequests      string    `json:"special_requests,omitempty"`
	TermsAccepted        bool      `json:"terms_accepted"`
	PrivacyAccepted      bool      `json:"privacy_accepted"`
}

// Service adalah use case inti booking.
type Service struct {
	tx          TxRunner // transaksi lintas modul (booking + inventory + outbox)
	inv         inventory.AvailabilityStore
	rates       rates.RateProvider
	quoteStore  rates.QuoteStore
	payment     PaymentGateway
	attempts    PaymentAttemptStore
	notify      Notifier
	reader      Reader
	holdTimeout time.Duration
	log         *slog.Logger
}

// TxRunner menjalankan fn dalam satu transaksi Postgres.
type TxRunner interface {
	// InTx mengeksekusi fn; InventoryTx yang diterima fn terikat pada tx yang sama.
	InTx(ctx context.Context, fn func(tx InventoryTx, events EventPublisher) error) error
}

func NewService(tx TxRunner, inv inventory.AvailabilityStore, r rates.RateProvider, p PaymentGateway, n Notifier, rd Reader, holdTimeout time.Duration, log *slog.Logger) *Service {
	if holdTimeout <= 0 {
		holdTimeout = 30 * time.Minute
	}
	return &Service{
		tx:          tx,
		inv:         inv,
		rates:       r,
		payment:     p,
		notify:      n,
		reader:      rd,
		holdTimeout: holdTimeout,
		log:         log,
	}
}

// SetQuoteStore menyematkan quote repository untuk penguncian harga 15 menit (BE-G06).
func (s *Service) SetQuoteStore(qs rates.QuoteStore) {
	s.quoteStore = qs
}

// SetPaymentAttemptStore menyematkan store buku besar percobaan pembayaran (BE-G11).
func (s *Service) SetPaymentAttemptStore(pas PaymentAttemptStore) {
	s.attempts = pas
}

func (s *Service) HoldTimeout() time.Duration { return s.holdTimeout }
func (s *Service) holdExpiry() time.Time       { return time.Now().UTC().Add(s.holdTimeout) }

// Create menjalankan critical path (desain §12.3):
//
//	BEGIN
//	  lock baris inventory [check_in, check_out) ORDER BY date ASC FOR UPDATE
//	  verifikasi availability
//	  decrement available_rooms
//	  INSERT booking (pending) + hold (expires_at) + reservation_room_nights
//	  INSERT outbox (booking.created)
//	COMMIT
//	CreateCharge via gateway (DI LUAR transaksi DB agar tidak menahan lock baris)
func (s *Service) Create(ctx context.Context, in CreateInput) (Booking, ChargeResult, error) {
	if !in.CheckIn.Before(in.CheckOut) {
		return Booking{}, ChargeResult{}, ErrInvalidDateRange
	}
	nights := int(in.CheckOut.Sub(in.CheckIn).Hours() / 24)
	if nights > 30 {
		return Booking{}, ChargeResult{}, ErrExceedsMaxStay
	}
	if in.NumRooms < 1 || in.NumRooms > 8 || in.NumGuests < 1 {
		return Booking{}, ChargeResult{}, ErrInvalidCapacity
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	checkInDate := time.Date(in.CheckIn.Year(), in.CheckIn.Month(), in.CheckIn.Day(), 0, 0, 0, 0, time.UTC)
	if checkInDate.Before(today.Add(-24 * time.Hour)) {
		return Booking{}, ChargeResult{}, ErrPastDate
	}
	if in.CheckOut.After(now.AddDate(0, 0, 366)) {
		return Booking{}, ChargeResult{}, ErrExceedsHorizon
	}
	if strings.TrimSpace(in.GuestName) == "" || strings.TrimSpace(in.GuestEmail) == "" || !strings.Contains(in.GuestEmail, "@") {
		return Booking{}, ChargeResult{}, ErrInvalidGuestInfo
	}
	if in.GuestPhone != "" && !phoneRegex.MatchString(in.GuestPhone) {
		return Booking{}, ChargeResult{}, ErrInvalidPhone
	}
	if in.EstimatedArrivalTime != "" && !arrivalRegex.MatchString(in.EstimatedArrivalTime) {
		return Booking{}, ChargeResult{}, ErrInvalidArrivalTime
	}
	if len([]rune(in.SpecialRequests)) > 500 {
		return Booking{}, ChargeResult{}, ErrSpecialRequestTooLong
	}

	// 1. Quote locking & Terms Consent validation (BE-G04, BE-G05, BE-G06)
	var quotes []rates.Quote
	var total int64
	var lockedQuote rates.LockedQuote

	if in.QuoteID != "" {
		if !in.TermsAccepted || !in.PrivacyAccepted {
			return Booking{}, ChargeResult{}, ErrConsentRequired
		}
		if s.quoteStore == nil {
			return Booking{}, ChargeResult{}, errors.New("booking: quote store not configured")
		}
		var err error
		lockedQuote, err = s.quoteStore.GetQuote(ctx, in.QuoteID)
		if err != nil {
			if errors.Is(err, rates.ErrQuoteNotFound) || errors.Is(err, rates.ErrQuoteExpired) {
				return Booking{}, ChargeResult{}, ErrQuoteExpired
			}
			return Booking{}, ChargeResult{}, fmt.Errorf("booking: quote lookup: %w", err)
		}
		// Match parameters
		if lockedQuote.RoomTypeID != in.RoomTypeID ||
			!lockedQuote.CheckIn.Equal(in.CheckIn) ||
			!lockedQuote.CheckOut.Equal(in.CheckOut) ||
			lockedQuote.NumRooms != in.NumRooms ||
			lockedQuote.NumGuests != in.NumGuests {
			return Booking{}, ChargeResult{}, ErrQuoteMismatch
		}
		quotes = lockedQuote.NightlyRates
		total = lockedQuote.Pricing.TotalPriceMinor
	}

	// 2. Pre-flight check (tanpa lock) untuk fail-fast
	if s.inv != nil {
		avail, err := s.inv.GetByDate(ctx, in.RoomTypeID, in.CheckIn, in.CheckOut)
		if err != nil {
			return Booking{}, ChargeResult{}, fmt.Errorf("booking: preflight: %w", err)
		}
		if err := inventory.Check(avail, in.CheckIn, in.CheckOut, in.NumRooms); err != nil {
			return Booking{}, ChargeResult{}, err
		}
	}

	// 3. Rate fallback jika tanpa quote ID
	if in.QuoteID == "" {
		if s.rates != nil {
			var err error
			quotes, err = s.rates.Quote(ctx, in.RoomTypeID, in.CheckIn, in.CheckOut)
			if err != nil {
				return Booking{}, ChargeResult{}, fmt.Errorf("booking: quote: %w", err)
			}
			for _, q := range quotes {
				total += q.RateMinor * int64(in.NumRooms)
			}
		}
	}

	b := Booking{
		RoomTypeID:           in.RoomTypeID,
		CheckIn:              in.CheckIn,
		CheckOut:             in.CheckOut,
		NumRooms:             in.NumRooms,
		NumGuests:            in.NumGuests,
		Status:               StatusPending,
		Currency:             "IDR",
		GuestName:            in.GuestName,
		GuestEmail:           in.GuestEmail,
		GuestPhone:           in.GuestPhone,
		EstimatedArrivalTime: in.EstimatedArrivalTime,
		SpecialRequests:      in.SpecialRequests,
		GuestToken:           generateGuestToken(),
		CreatedAt:            time.Now().UTC(),
	}

	if in.QuoteID != "" {
		b.QuoteID = lockedQuote.ID
		b.RatePlanCode = lockedQuote.RatePlanCode
		b.CancellationPolicy = lockedQuote.CancellationCode
		b.CancellationDesc = lockedQuote.CancellationDesc
		b.RoomSubtotalMinor = lockedQuote.Pricing.RoomSubtotalMinor
		b.BreakfastChargeMinor = lockedQuote.Pricing.BreakfastChargeMinor
		b.DiscountMinor = lockedQuote.Pricing.DiscountMinor
		b.TaxMinor = lockedQuote.Pricing.TaxMinor
		b.TotalPriceMinor = lockedQuote.Pricing.TotalPriceMinor
		b.Currency = lockedQuote.Pricing.Currency
	} else {
		b.RatePlanCode = rates.RatePlanRoomOnly
		b.CancellationPolicy = rates.PolicyFlexible48h
		b.CancellationDesc = "Pembatalan gratis hingga 48 jam sebelum check-in"
		b.RoomSubtotalMinor = total
		b.TotalPriceMinor = total
	}

	if in.TermsAccepted {
		nowConsent := time.Now().UTC()
		b.TermsAccepted = true
		b.TermsAcceptedAt = &nowConsent
	}

	// 2. Transaksi kritis lokal: lock + decrement + insert + outbox.
	txErr := s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
		if err := tx.LockAndDecrement(ctx, b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms); err != nil {
			return err
		}
		if err := tx.InsertBookingWithHold(ctx, &b, quotes, s.holdExpiry()); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"event":       "booking.created",
			"booking_id":  b.ID,
			"room_type":   b.RoomTypeID,
			"check_in":    b.CheckIn.Format("2006-01-02"),
			"check_out":   b.CheckOut.Format("2006-01-02"),
			"num_rooms":   b.NumRooms,
			"total_minor": b.TotalPriceMinor,
		})
		return events.PublishTx(ctx, "booking.created", payload)
	})
	if txErr != nil {
		return Booking{}, ChargeResult{}, txErr
	}

	// 3. Pemanggilan gateway eksternal di luar transaksi agar tidak menahan lock database.
	charge, err := s.payment.CreateCharge(ctx, b, b.TotalPriceMinor, b.Currency)
	if err != nil {
		if s.attempts != nil {
			_ = s.attempts.RecordAttempt(ctx, PaymentAttempt{
				BookingID:         b.ID,
				Provider:          "gateway",
				ProviderReference: "",
				AmountMinor:       b.TotalPriceMinor,
				Currency:          b.Currency,
				Status:            "failed",
				CreatedAt:         time.Now().UTC(),
				UpdatedAt:         time.Now().UTC(),
			})
		}
		// Kompensasi: batalkan booking dan rilis inventory jika charge token gagal
		_ = s.Cancel(ctx, b.ID)
		return Booking{}, ChargeResult{}, fmt.Errorf("booking: payment charge: %w", err)
	}

	if s.attempts != nil {
		_ = s.attempts.RecordAttempt(ctx, PaymentAttempt{
			BookingID:         b.ID,
			Provider:          "gateway",
			ProviderReference: charge.Reference,
			AmountMinor:       b.TotalPriceMinor,
			Currency:          b.Currency,
			Status:            "initiated",
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		})
	}

	return b, charge, nil
}

// Confirm mentransisikan pending → confirmed secara IDEMPOTEN (desain §12.4):
// webhook dapat datang dua kali; transisi bersyarat membuat duplikat harmless.
func (s *Service) Confirm(ctx context.Context, bookingID string) error {
	return s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
		b, err := tx.GetForUpdate(ctx, bookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusConfirmed {
			return nil // idempotent — webhook duplikat
		}
		if b.Status == StatusPending && b.ExpiresAt != nil && time.Now().UTC().After(*b.ExpiresAt) {
			if s.attempts != nil {
				_ = s.attempts.UpdateAttemptStatus(ctx, bookingID, "received_after_expiry")
			}
			return ErrHoldExpired
		}
		if err := Transition(b.Status, StatusConfirmed); err != nil {
			return err
		}
		if err := tx.UpdateStatus(ctx, bookingID, StatusConfirmed); err != nil {
			return err
		}
		if s.attempts != nil {
			_ = s.attempts.UpdateAttemptStatus(ctx, bookingID, "success")
		}
		payload, _ := json.Marshal(map[string]any{"event": "booking.confirmed", "booking_id": bookingID})
		return events.PublishTx(ctx, "booking.confirmed", payload)
	})
}

// Cancel mentransisikan ke cancelled dan mengembalikan inventory.
func (s *Service) Cancel(ctx context.Context, bookingID string) error {
	return s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
		b, err := tx.GetForUpdate(ctx, bookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusCancelled {
			return nil // idempotent
		}

		// Penegakan kebijakan pembatalan untuk pesanan confirmed (BE-G08)
		if b.Status == StatusConfirmed {
			if b.CancellationPolicy == rates.PolicyNonRefundable {
				return ErrNonRefundable
			}
			if b.CancellationPolicy == rates.PolicyFlexible48h {
				// Deadline 48 jam sebelum jam 14:00 WIB pada tanggal check-in
				checkInTime := time.Date(b.CheckIn.Year(), b.CheckIn.Month(), b.CheckIn.Day(), 14, 0, 0, 0, time.UTC)
				deadline := checkInTime.Add(-48 * time.Hour)
				if time.Now().After(deadline) {
					return ErrCancellationDeadlineExceeded
				}
			}
		}

		if err := Transition(b.Status, StatusCancelled); err != nil {
			return err
		}
		if err := tx.UpdateStatus(ctx, bookingID, StatusCancelled); err != nil {
			return err
		}
		// Kembalikan ketersediaan untuk status yang sempat memegang inventory.
		if b.Status == StatusPending || b.Status == StatusConfirmed {
			if err := tx.Increment(ctx, b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms); err != nil {
				return err
			}
		}
		payload, _ := json.Marshal(map[string]any{"event": "booking.cancelled", "booking_id": bookingID})
		return events.PublishTx(ctx, "booking.cancelled", payload)
	})
}

// CheckOut mentransisikan checked_in → checked_out (BE-G22).
// Bila tamu check-out lebih awal daripada tanggal yang dijadwalkan (early check-out),
// sisa kamar [today, scheduled_checkout) dikembalikan ke inventaris agar dapat dijual kembali.
func (s *Service) CheckOut(ctx context.Context, bookingID string) error {
	return s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
		b, err := tx.GetForUpdate(ctx, bookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusCheckedOut {
			return nil // idempotent
		}
		if err := Transition(b.Status, StatusCheckedOut); err != nil {
			return err
		}
		if err := tx.UpdateStatus(ctx, bookingID, StatusCheckedOut); err != nil {
			return err
		}

		// Restitusi inventaris jika early check-out (BE-G22)
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		checkOutUTC := b.CheckOut.UTC()
		scheduledOut := time.Date(checkOutUTC.Year(), checkOutUTC.Month(), checkOutUTC.Day(), 0, 0, 0, 0, time.UTC)
		isEarly := false
		if today.Before(scheduledOut) {
			releaseFrom := today
			checkInUTC := b.CheckIn.UTC()
			scheduledIn := time.Date(checkInUTC.Year(), checkInUTC.Month(), checkInUTC.Day(), 0, 0, 0, 0, time.UTC)
			if releaseFrom.Before(scheduledIn) {
				releaseFrom = scheduledIn
			}
			if releaseFrom.Before(scheduledOut) {
				if err := tx.Increment(ctx, b.RoomTypeID, releaseFrom, scheduledOut, b.NumRooms); err != nil {
					return err
				}
				isEarly = true
			}
		}

		payload, _ := json.Marshal(map[string]any{
			"event":          "booking.checked_out",
			"booking_id":     bookingID,
			"early_checkout": isEarly,
		})
		return events.PublishTx(ctx, "booking.checked_out", payload)
	})
}

// MarkNoShow mentransisikan confirmed → no_show dan mengembalikan inventory (BE-G22).
// Penandaan no-show hanya dapat dilakukan terhitung sejak tanggal check-in tiba.
// Upaya menandai no-show sebelum tanggal check-in ditolak dengan ErrNoShowTooEarly.
func (s *Service) MarkNoShow(ctx context.Context, bookingID string) error {
	return s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
		b, err := tx.GetForUpdate(ctx, bookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusNoShow {
			return nil // idempotent
		}

		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		checkInUTC := b.CheckIn.UTC()
		checkInDate := time.Date(checkInUTC.Year(), checkInUTC.Month(), checkInUTC.Day(), 0, 0, 0, 0, time.UTC)
		if today.Before(checkInDate) {
			return ErrNoShowTooEarly
		}

		if err := Transition(b.Status, StatusNoShow); err != nil {
			return err
		}
		if err := tx.UpdateStatus(ctx, bookingID, StatusNoShow); err != nil {
			return err
		}

		// Kembalikan sisa inventaris dari tanggal hari ini hingga check-out
		releaseFrom := today
		if releaseFrom.Before(checkInDate) {
			releaseFrom = checkInDate
		}
		checkOutUTC := b.CheckOut.UTC()
		scheduledOut := time.Date(checkOutUTC.Year(), checkOutUTC.Month(), checkOutUTC.Day(), 0, 0, 0, 0, time.UTC)
		if releaseFrom.Before(scheduledOut) {
			if err := tx.Increment(ctx, b.RoomTypeID, releaseFrom, scheduledOut, b.NumRooms); err != nil {
				return err
			}
		}

		payload, _ := json.Marshal(map[string]any{"event": "booking.no_show", "booking_id": bookingID})
		return events.PublishTx(ctx, "booking.no_show", payload)
	})
}

// Get membaca satu booking (tanpa transaksi).
func (s *Service) Get(ctx context.Context, id string) (Booking, error) {
	return s.reader.Get(ctx, id)
}

// CheckInResult hasil use case check-in.
type CheckInResult struct {
	BookingID   string   `json:"id"`
	RoomNumbers []string `json:"room_numbers"`
	Already     bool     `json:"already_checked_in,omitempty"` // true saat pemanggilan ulang
}

// CheckIn mentransisikan confirmed → checked_in DAN memasangkan kamar fisik
// dalam SATU transaksi. Keunikan nomor kamar per rentang tanggal dijamin
// database via EXCLUDE USING GIST (desain §13) — bukan oleh kode aplikasi.
// Mendukung multi-kamar (num_rooms >= 1). Idempotent: booking yang sudah
// checked_in mengembalikan daftar kamar yang sama.
// Otomatis melakukan retry hingga 3 kali jika terjadi transient concurrency conflict.
func (s *Service) CheckIn(ctx context.Context, bookingID string) (CheckInResult, error) {
	var res CheckInResult
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		err := s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
			b, err := tx.GetForUpdate(ctx, bookingID)
			if err != nil {
				return err
			}
			if b.Status == StatusCheckedIn {
				rooms, err := tx.GetRoomAssignments(ctx, bookingID)
				if err != nil {
					return err
				}
				res = CheckInResult{BookingID: bookingID, RoomNumbers: rooms, Already: true}
				return nil
			}
			if err := Transition(b.Status, StatusCheckedIn); err != nil {
				return err
			}
			rooms, err := tx.PickAndAssignRooms(ctx, bookingID, b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms)
			if err != nil {
				return err
			}
			if err := tx.UpdateStatus(ctx, bookingID, StatusCheckedIn); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"event": "booking.checked_in", "booking_id": bookingID, "room_numbers": rooms,
			})
			if err := events.PublishTx(ctx, "booking.checked_in", payload); err != nil {
				return err
			}
			res = CheckInResult{BookingID: bookingID, RoomNumbers: rooms}
			return nil
		})
		if err == nil {
			return res, nil
		}
		if errors.Is(err, ErrTransientConflict) {
			lastErr = ErrNoRoomAvailable
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
			}
			continue
		}
		return res, err
	}
	return res, lastErr
}

