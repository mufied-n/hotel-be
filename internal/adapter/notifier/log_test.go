package notifier

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

func TestLogNotifier_TableTest(t *testing.T) {
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

	tests := []struct {
		name       string
		masked     bool
		otpCode    string
		wantInLog  string
		wantNotLog string
	}{
		{
			name:       "unmasked log notifier prints actual OTP",
			masked:     false,
			otpCode:    "849201",
			wantInLog:  "849201",
			wantNotLog: "[REDACTED]",
		},
		{
			name:       "masked log notifier redacts OTP",
			masked:     true,
			otpCode:    "849201",
			wantInLog:  "[REDACTED]",
			wantNotLog: "849201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			var n *LogNotifier
			if tt.masked {
				n = NewLogMasked(logger)
			} else {
				n = NewLog(logger)
			}

			err := n.SendBookingConfirmed(context.Background(), b)
			if err != nil {
				t.Fatalf("unexpected error on SendBookingConfirmed: %v", err)
			}
			if !strings.Contains(buf.String(), "email.booking_confirmed") {
				t.Errorf("expected booking confirmation in log, got: %s", buf.String())
			}

			buf.Reset()
			err = n.SendGuestOTP(context.Background(), "siti@example.com", tt.otpCode)
			if err != nil {
				t.Fatalf("unexpected error on SendGuestOTP: %v", err)
			}
			logOutput := buf.String()
			if !strings.Contains(logOutput, tt.wantInLog) {
				t.Errorf("expected log to contain %q, got: %s", tt.wantInLog, logOutput)
			}
			if tt.wantNotLog != "" && strings.Contains(logOutput, "otp="+tt.wantNotLog) {
				t.Errorf("expected log NOT to contain %q, got: %s", tt.wantNotLog, logOutput)
			}
		})
	}
}
