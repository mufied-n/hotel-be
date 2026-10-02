// Package notifier menyediakan implementasi port booking.Notifier.
// Saat ini: stub berbasis log. Untuk produksi, ganti dengan adapter
// SMTP/Resend/SES — domain tidak perlu diubah (hexagonal §8.3).
package notifier

import (
	"context"
	"log/slog"

	"github.com/example/hotel-booking/internal/booking"
)

// LogNotifier mengirim "email" ke structured log.
type LogNotifier struct{ Log *slog.Logger }

func NewLog(l *slog.Logger) *LogNotifier { return &LogNotifier{Log: l} }

func (n *LogNotifier) SendBookingConfirmed(ctx context.Context, b booking.Booking) error {
	n.Log.InfoContext(ctx, "email.booking_confirmed",
		"booking_id", b.ID,
		"to", b.GuestEmail,
		"guest", b.GuestName,
		"check_in", b.CheckIn.Format("2006-01-02"),
		"check_out", b.CheckOut.Format("2006-01-02"),
		"num_rooms", b.NumRooms,
		"total_minor", b.TotalPriceMinor,
		"currency", b.Currency,
	)
	return nil
}

var _ booking.Notifier = (*LogNotifier)(nil)
