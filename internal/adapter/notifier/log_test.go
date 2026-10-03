package notifier

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

func TestLogNotifier_TableTest(t *testing.T) {
	n := NewLog(slog.Default())
	b := booking.Booking{
		ID:              "01900000-0000-7000-8000-000000000001",
		GuestName:       "Siti Rahma",
		GuestEmail:      "siti@example.com",
		CheckIn:         time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		CheckOut:        time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		NumRooms:        1,
		TotalPriceMinor: 750000,
		Currency:        "IDR",
	}

	err := n.SendBookingConfirmed(context.Background(), b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = n.SendGuestOTP(context.Background(), "siti@example.com", "123456")
	if err != nil {
		t.Fatalf("unexpected error on SendGuestOTP: %v", err)
	}
}

