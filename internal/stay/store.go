package stay

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrBookingNotFound dikembalikan bila booking tidak ditemukan.
	ErrBookingNotFound = errors.New("stay: booking tidak ditemukan")
	// ErrInvalidBookingStatus dikembalikan bila status booking bukan checked_in saat room move.
	ErrInvalidBookingStatus = errors.New("stay: pemindahan kamar hanya dapat dilakukan untuk tamu yang sedang checked_in")
	// ErrSameRoomMove dikembalikan bila kamar target sama dengan kamar saat ini.
	ErrSameRoomMove = errors.New("stay: kamar tujuan tidak boleh sama dengan kamar saat ini")
	// ErrTargetRoomNotReady dikembalikan bila kamar target belum berstatus inspected.
	ErrTargetRoomNotReady = errors.New("stay: kamar target belum siap huni (wajib berstatus inspected)")
	// ErrRoomPhysicalOverlap dikembalikan bila kamar target atau perpanjangan bertabrakan dengan reservasi lain.
	ErrRoomPhysicalOverlap = errors.New("stay: kamar fisik memiliki jadwal menginap lain yang tumpang tindih")
	// ErrInvalidAdditionalNights dikembalikan bila jumlah malam perpanjangan tidak antara 1 dan 30.
	ErrInvalidAdditionalNights = errors.New("stay: jumlah malam perpanjangan harus antara 1 dan 30 malam")
	// ErrNoAvailabilityForExtension dikembalikan bila inventaris tipe kamar habis pada rentang perpanjangan.
	ErrNoAvailabilityForExtension = errors.New("stay: tidak ada kuota kamar yang tersedia untuk tanggal perpanjangan")
	// ErrInvalidReasonCategory dikembalikan bila kategori alasan pemindahan tidak sah.
	ErrInvalidReasonCategory = errors.New("stay: kategori alasan pemindahan kamar tidak sah")
)

// Store mendefinisikan port driven persistensi dan transaksi masa menginap.
type Store interface {
	GetBooking(ctx context.Context, bookingID string) (*BookingDetails, error)
	MoveRoom(ctx context.Context, input RoomMoveInput, moveDate time.Time) (*RoomMoveResult, error)
	ExtendStay(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*ExtendStayResult, error)
	ListRoomMoves(ctx context.Context, bookingID string) ([]RoomMoveLog, error)
}
