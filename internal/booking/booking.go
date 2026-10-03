// Package booking adalah domain core sistem (desain §12): state machine
// booking, port-nya, dan use case. Modul ini TIDAK mengenal vendor mana pun —
// semua dependensi eksternal lewat interface yang didefinisikan di sini
// (hexagonal, desain §8).
package booking

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status adalah state machine booking (desain §12.2).
type Status string

const (
	StatusPending    Status = "pending"
	StatusConfirmed  Status = "confirmed"
	StatusCheckedIn  Status = "checked_in"
	StatusCheckedOut Status = "checked_out"
	StatusCancelled  Status = "cancelled"
	StatusExpired    Status = "expired"
	StatusFailed     Status = "failed"
	StatusNoShow     Status = "no_show"
)

// legalTransitions memetakan status → himpunan status tujuan yang sah.
// Transisi hanya boleh lewat fungsi Transition terpusat (bukan ad-hoc UPDATE).
var legalTransitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusConfirmed: true, // webhook pembayaran sukses
		StatusExpired:   true, // hold timeout (release-hold worker)
		StatusFailed:    true, // pembayaran gagal
		StatusCancelled: true, // tamu batal sebelum bayar
	},
	StatusConfirmed: {
		StatusCheckedIn: true,
		StatusCancelled: true,
		StatusNoShow:    true,
	},
	StatusCheckedIn: {
		StatusCheckedOut: true,
	},
	StatusCheckedOut: {},
	StatusCancelled:  {},
	StatusExpired:    {},
	StatusFailed:     {},
	StatusNoShow:     {},
}

// ErrIllegalTransition diembalikan bila transisi status tidak sah.
var ErrIllegalTransition = errors.New("booking: illegal status transition")

// Transition memvalidasi transisi status. Fungsi murni — di-unit-test penuh.
func Transition(from, to Status) error {
	if !legalTransitions[from][to] {
		return fmt.Errorf("%w: %s → %s", ErrIllegalTransition, from, to)
	}
	return nil
}

// Booking adalah agregat booking.
type Booking struct {
	ID                   string     `json:"id"`
	RoomTypeID           string     `json:"room_type_id"`
	CheckIn              time.Time  `json:"check_in"`
	CheckOut             time.Time  `json:"check_out"`
	NumRooms             int        `json:"num_rooms"`
	NumGuests            int        `json:"num_guests"`
	Status               Status     `json:"status"`
	QuoteID              string     `json:"quote_id,omitempty"`
	RatePlanCode         string     `json:"rate_plan_code"`
	CancellationPolicy   string     `json:"cancellation_policy"`
	CancellationDesc     string     `json:"cancellation_description"`
	RoomSubtotalMinor    int64      `json:"room_subtotal_minor"`
	BreakfastChargeMinor int64      `json:"breakfast_charge_minor"`
	DiscountMinor        int64      `json:"discount_minor"`
	TaxMinor             int64      `json:"tax_minor"`
	TotalPriceMinor      int64      `json:"total_price_minor"`
	Currency             string     `json:"currency"`
	GuestName            string     `json:"guest_name"`
	GuestEmail           string     `json:"guest_email"`
	GuestPhone           string     `json:"guest_phone,omitempty"`
	EstimatedArrivalTime string     `json:"estimated_arrival_time,omitempty"`
	SpecialRequests      string     `json:"special_requests,omitempty"`
	GuestToken           string     `json:"guest_token,omitempty"`
	TermsAccepted        bool       `json:"terms_accepted"`
	TermsAcceptedAt      *time.Time `json:"terms_accepted_at,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// PublicDTO adalah representasi publik minimal tanpa kebocoran PII (BE-G13).
// guest_phone dan guest_email disembunyikan untuk memenuhi UU PDP No. 27/2022.
type PublicDTO struct {
	ID                   string     `json:"id"`
	RoomTypeID           string     `json:"room_type_id"`
	CheckIn              time.Time  `json:"check_in"`
	CheckOut             time.Time  `json:"check_out"`
	NumRooms             int        `json:"num_rooms"`
	Status               Status     `json:"status"`
	EstimatedArrivalTime string     `json:"estimated_arrival_time,omitempty"`
	SpecialRequests      string     `json:"special_requests,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// ToPublicDTO menyaring field sensitif (nama, email, telepon, guest_token) untuk akses unauthenticated.
func (b Booking) ToPublicDTO() PublicDTO {
	return PublicDTO{
		ID:                   b.ID,
		RoomTypeID:           b.RoomTypeID,
		CheckIn:              b.CheckIn,
		CheckOut:             b.CheckOut,
		NumRooms:             b.NumRooms,
		Status:               b.Status,
		EstimatedArrivalTime: b.EstimatedArrivalTime,
		SpecialRequests:      b.SpecialRequests,
		ExpiresAt:            b.ExpiresAt,
		CreatedAt:            b.CreatedAt,
	}
}

// Nights mengembalikan jumlah malam (konvensi half-open [check_in, check_out)).
func (b Booking) Nights() int {
	fromUTC := time.Date(b.CheckIn.Year(), b.CheckIn.Month(), b.CheckIn.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(b.CheckOut.Year(), b.CheckOut.Month(), b.CheckOut.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC) / (24 * time.Hour))
}

// EventPublisher adalah port outbox (desain §5.3, §8.3): event domain
// ditulis dalam transaksi yang sama dengan perubahan state.
type EventPublisher interface {
	PublishTx(ctx context.Context, topic string, payload []byte) error
}

// PaymentEvent adalah hasil verifikasi webhook pembayaran (bahasa domain).
type PaymentEvent struct {
	BookingID string
	Succeeded bool
	Reference string
}

// ErrNotFound diembalikan bila booking tidak ditemukan.
var ErrNotFound = errors.New("booking: not found")

// ErrInsufficient diembalikan bila kamar tidak cukup pada transaksi kritis
// (termasuk saat kalah race FOR UPDATE → dikonversi HTTP 409 oleh API).
var ErrInsufficient = errors.New("booking: insufficient rooms")

// ErrNoRoomAvailable diembalikan saat check-in tidak menemukan kamar fisik
// bebas untuk rentang menginap — termasuk ketika GiST exclusion constraint
// (desain §13) menolak assignment yang tumpang tindih.
var ErrNoRoomAvailable = errors.New("booking: no physical room available")

// ErrRoomNotReady dikembalikan bila kamar fisik belum siap huni (status belum inspected).
var ErrRoomNotReady = errors.New("booking: room is not ready for check-in (not inspected)")

// ErrTransientConflict menandakan tabrakan konkuren sementara (SQLSTATE 23P01/40001)
// yang dapat di-retry secara aman dengan snapshot transaksi baru.
var ErrTransientConflict = errors.New("booking: transient concurrency conflict")

// ErrInvalidDateRange diembalikan jika check_out <= check_in.
var ErrInvalidDateRange = errors.New("booking: check_out must be after check_in")

// ErrInvalidCapacity diembalikan bila num_rooms atau num_guests kurang dari 1 atau melebihi batas.
var ErrInvalidCapacity = errors.New("booking: num_rooms (1-8) and num_guests must be positive")

// ErrExceedsMaxStay diembalikan bila durasi menginap melebihi batas maksimal 30 malam (BE-G03).
var ErrExceedsMaxStay = errors.New("booking: stay duration exceeds maximum 30 nights")

// ErrPastDate diembalikan bila tanggal check_in berada di masa lampau (BE-G03).
var ErrPastDate = errors.New("booking: check_in date cannot be in the past")

// ErrExceedsHorizon diembalikan bila tanggal reservasi melebihi horizon 365 hari (BE-G03).
var ErrExceedsHorizon = errors.New("booking: check_out date exceeds 365 days booking horizon")

// ErrInvalidGuestInfo diembalikan bila guest_name kosong atau guest_email tidak valid (BE-G03).
var ErrInvalidGuestInfo = errors.New("booking: guest_name and valid guest_email are required")

// ErrConsentRequired diembalikan bila tamu belum menyetujui syarat & ketentuan dan privasi (BE-G08).
var ErrConsentRequired = errors.New("booking: terms and privacy consent are required")

// ErrQuoteRequired diembalikan bila pembuatan booking tidak menyertakan quote_id (BE-G06).
var ErrQuoteRequired = errors.New("booking: quote_id is required")

// ErrQuoteExpired diembalikan bila quote sudah melewati batas TTL 15 menit (BE-G06).
var ErrQuoteExpired = errors.New("booking: quote has expired (>15m)")

// ErrQuoteMismatch diembalikan bila parameter booking berbeda dengan quote terkunci (BE-G06).
var ErrQuoteMismatch = errors.New("booking: reservation parameters do not match quote")

// ErrNonRefundable diembalikan saat tamu mencoba membatalkan booking bertarif non-refundable (BE-G08).
var ErrNonRefundable = errors.New("booking: non-refundable reservation cannot be cancelled by guest")

// ErrCancellationDeadlineExceeded diembalikan saat pembatalan melewati batas waktu H-2 (BE-G08).
var ErrCancellationDeadlineExceeded = errors.New("booking: free cancellation deadline has passed (48h before check-in)")

// ErrInvalidPhone dikembalikan jika nomor telepon bukan format E.164 (BE-G07).
var ErrInvalidPhone = errors.New("booking: invalid phone number, must be in E.164 format (e.g. +6281234567890)")

// ErrInvalidArrivalTime dikembalikan jika jam kedatangan bukan format HH:MM 24-jam (BE-G07).
var ErrInvalidArrivalTime = errors.New("booking: invalid estimated arrival time, must be HH:MM format (e.g. 14:00)")

// ErrSpecialRequestTooLong dikembalikan jika special requests melebihi 500 karakter (BE-G07).
var ErrSpecialRequestTooLong = errors.New("booking: special requests exceeds 500 characters limit")

// ErrHoldExpired dikembalikan jika pembayaran tiba setelah batas waktu hold kamar kedaluwarsa (BE-G12).
var ErrHoldExpired = errors.New("booking: hold has expired, room availability was released")

// ErrNoShowTooEarly dikembalikan saat no-show dipicu sebelum tanggal check-in tiba (BE-G22).
var ErrNoShowTooEarly = errors.New("booking: cannot mark no-show before check-in date")
