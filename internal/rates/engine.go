// Package rates adalah rate engine sederhana (desain §3.2): harga per malam
// dari base rate per tipe kamar + multiplier weekend. Kelas pricing produksi
// (promo code, seasonal, LOS pricing) ditambahkan di modul ini tanpa
// menyentuh modul lain.
package rates

import (
	"context"
	"time"
)

// Quote adalah harga untuk satu malam.
type Quote struct {
	Date      time.Time `json:"date"`
	RateMinor int64     `json:"rate_minor"` // satuan minor unit (sen/cent) — NFR §15
}

// RateProvider adalah port yang dikonsumsi booking untuk menghitung harga.
// Definisi interface ada di sisi consumer (modul rates sebagai penyedia
// logika, binding tetap dilakukan di composition root — §8).
type RateProvider interface {
	Quote(ctx context.Context, roomTypeID string, from, to time.Time) ([]Quote, error)
}

// Engine implementasi in-memory: base rate per tipe + multiplier weekend.
type Engine struct {
	base          map[string]int64
	weekendFactor float64
}

func NewEngine(base map[string]int64, weekendFactor float64) *Engine {
	return &Engine{base: base, weekendFactor: weekendFactor}
}

// Quote menghitung harga untuk rentang half-open [from, to).
func (e *Engine) Quote(_ context.Context, roomTypeID string, from, to time.Time) ([]Quote, error) {
	base, ok := e.base[roomTypeID]
	if !ok {
		return nil, ErrUnknownRoomType{RoomTypeID: roomTypeID}
	}
	var out []Quote
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		rate := base
		switch d.Weekday() {
		case time.Friday, time.Saturday:
			rate = int64(float64(base) * e.weekendFactor)
		}
		out = append(out, Quote{Date: d, RateMinor: rate})
	}
	return out, nil
}

// ErrUnknownRoomType diembalikan bila tipe kamar tidak punya base rate.
type ErrUnknownRoomType struct{ RoomTypeID string }

func (e ErrUnknownRoomType) Error() string {
	return "rates: unknown room type " + e.RoomTypeID
}
