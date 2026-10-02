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
	ID              string    `json:"id"`
	RoomTypeID      string    `json:"room_type_id"`
	CheckIn         time.Time `json:"check_in"`
	CheckOut        time.Time `json:"check_out"`
	NumRooms        int       `json:"num_rooms"`
	NumGuests       int       `json:"num_guests"`
	Status          Status    `json:"status"`
	TotalPriceMinor int64     `json:"total_price_minor"`
	Currency        string    `json:"currency"`
	GuestName       string    `json:"guest_name"`
	GuestEmail      string    `json:"guest_email"`
	GuestToken      string    `json:"guest_token,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PublicDTO adalah representasi publik minimal tanpa kebocoran PII (BE-G13).
type PublicDTO struct {
	ID         string    `json:"id"`
	RoomTypeID string    `json:"room_type_id"`
	CheckIn    time.Time `json:"check_in"`
	CheckOut   time.Time `json:"check_out"`
	NumRooms   int       `json:"num_rooms"`
	Status     Status    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ToPublicDTO menyaring field sensitif (nama, email, guest_token) untuk akses unauthenticated.
func (b Booking) ToPublicDTO() PublicDTO {
	return PublicDTO{
		ID:         b.ID,
		RoomTypeID: b.RoomTypeID,
		CheckIn:    b.CheckIn,
		CheckOut:   b.CheckOut,
		NumRooms:   b.NumRooms,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt,
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
