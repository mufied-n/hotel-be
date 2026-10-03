package payment

import (
	"context"
	"testing"

	"github.com/example/hotel-booking/internal/booking"
)

func TestFakeGateway_TableTest(t *testing.T) {
	gw := NewFake()

	t.Run("CreateCharge generates valid fake pay URL and reference", func(t *testing.T) {
		res, err := gw.CreateCharge(context.Background(), booking.Booking{ID: "b1"}, 1000000, "IDR")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.PaymentURL == "" || res.Reference == "" {
			t.Errorf("expected non-empty URL and ref, got %v", res)
		}
	})

	t.Run("VerifyWebhook returns succeeded true", func(t *testing.T) {
		ev, err := gw.VerifyWebhook([]byte("{}"), "dummy-sig")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ev.Succeeded {
			t.Errorf("expected succeeded true")
		}
	})
}
