// Package payment menyediakan implementasi port booking.PaymentGateway.
// FakeGateway dipakai untuk dev/CI; adapter vendor nyata (Midtrans/Stripe/
// Xendit) ditambahkan di package ini tanpa menyentuh domain (§8.3).
package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/example/hotel-booking/internal/booking"
)

// FakeGateway membuat payment URL dummy — untuk dev/test.
// BaseURL bisa dioverride (mis. "http://localhost:18080" saat port di-remap).
type FakeGateway struct{ BaseURL string }

func NewFake() *FakeGateway {
	base := os.Getenv("FAKE_PAY_BASE_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &FakeGateway{BaseURL: base}
}

func (g *FakeGateway) CreateCharge(_ context.Context, b booking.Booking, amountMinor int64, currency string) (booking.ChargeResult, error) {
	ref, err := randomRef()
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("payment: ref: %w", err)
	}
	return booking.ChargeResult{
		PaymentURL: fmt.Sprintf("%s/fake-pay/%s?amount=%d&currency=%s", g.BaseURL, ref, amountMinor, currency),
		Reference:  ref,
	}, nil
}

// VerifyWebhook pada fake selalu menerima.
func (g *FakeGateway) VerifyWebhook(_ []byte, _ string) (booking.PaymentEvent, error) {
	return booking.PaymentEvent{BookingID: "", Succeeded: true}, nil
}

func randomRef() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

var _ booking.PaymentGateway = (*FakeGateway)(nil)
