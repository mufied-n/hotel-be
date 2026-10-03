package handler

import (
	"context"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/rates"
)

type mockFinanceLatePayment struct {
	mockFinanceService
	called bool
}

func (m *mockFinanceLatePayment) CreateLatePaymentCase(_ context.Context, _, _ string, _ int64, _ string) (*finance.PaymentCase, error) {
	m.called = true
	return &finance.PaymentCase{ID: "c-1"}, nil
}

func TestDeps_WithDefaults(t *testing.T) {
	rateEngine := rates.NewEngine(map[string]int64{"v1": 100000}, 1.0)
	finMock := &mockFinanceLatePayment{}
	bkSvc := &booking.Service{}

	d := Deps{
		RateEngine: rateEngine,
		BookingSvc: bkSvc,
		FinanceSvc: finMock,
	}

	res := d.WithDefaults()

	if res.CatalogStore == nil {
		t.Errorf("expected CatalogStore to be initialized")
	}
	if res.RateSvc == nil {
		t.Errorf("expected RateSvc to be initialized from RateEngine")
	}
	if res.QuoteStore == nil {
		t.Errorf("expected QuoteStore to be initialized from RateEngine")
	}
	if res.RequestTimeout != 30*time.Second {
		t.Errorf("expected default RequestTimeout to be 30s, got %v", res.RequestTimeout)
	}

	// Test late payment adapter call
	adapter := latePaymentAdapter{svc: finMock}
	err := adapter.CreateLatePaymentCase(context.Background(), "b1", "ref1", 500000, "late")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !finMock.called {
		t.Errorf("expected CreateLatePaymentCase to be called on service")
	}
}
