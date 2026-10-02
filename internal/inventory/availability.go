// Package inventory menangani ketersediaan kamar (Schema A row-per-day,
// kamar fungible — desain §13). Modul ini adalah domain core: ia mendefinisikan
// port Repository-nya sendiri; implementasi Postgres ada di adapter.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Availability adalah sisa kamar untuk satu malam.
type Availability struct {
	Date           time.Time `json:"date"`
	TotalRooms     int       `json:"total_rooms"`
	AvailableRooms int       `json:"available_rooms"`
}

var (
	// ErrInsufficient diembalikan bila jumlah kamar tersisa kurang dari yang diminta.
	ErrInsufficient = errors.New("inventory: insufficient rooms")
	// ErrNotFound diembalikan bila baris inventory tidak ada untuk tipe kamar/tanggal tsb.
	ErrNotFound = errors.New("inventory: not found")
)

// Port (driven) milik modul inventory. Implementasi Postgres ada di internal/adapter.
// Sesuai prinsip hexagonal: interface didefinisikan di sisi consumer/domain (§8.2).
type AvailabilityStore interface {
	// GetByDate mengembalikan availability satu tipe kamar untuk rentang
	// half-open [from, to). Baris yang hilang untuk tanggal tertentu dianggap
	// ErrNotFound oleh pemanggil.
	GetByDate(ctx context.Context, roomTypeID string, from, to time.Time) ([]Availability, error)
}

// Check memverifikasi bahwa jumlah kamar tersedia sepanjang rentang tanggal.
// Fungsi murni — mudah di-unit-test tanpa DB.
func Check(avail []Availability, from, to time.Time, numRooms int) error {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	nights := int(toUTC.Sub(fromUTC) / (24 * time.Hour))
	if len(avail) < nights {
		return fmt.Errorf("%w: expected %d nights, got %d rows", ErrNotFound, nights, len(avail))
	}
	for _, a := range avail {
		if a.AvailableRooms < numRooms {
			return fmt.Errorf("%w: %s has %d, need %d",
				ErrInsufficient, a.Date.Format("2006-01-02"), a.AvailableRooms, numRooms)
		}
	}
	return nil
}
