package booking

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockLatePaymentRecorder struct {
	recorded bool
}

func (m *mockLatePaymentRecorder) CreateLatePaymentCase(_ context.Context, _, _ string, _ int64, _ string) error {
	m.recorded = true
	return nil
}

func TestApplyPaymentEvent_TableDriven(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	baseBooking := Booking{
		ID:              "bk-test-1",
		Status:          StatusPending,
		TotalPriceMinor: 1_000_000,
		Currency:        "IDR",
		ExpiresAt:       &exp,
	}

	tests := []struct {
		name          string
		initialStatus Status
		setupSvc      func(s *Service)
		event         PaymentEvent
		wantOutcome   PaymentOutcomeStatus
		wantErr       error
	}{
		{
			name:          "PAID on confirmed booking returns ALREADY_CONFIRMED (idempotent replay)",
			initialStatus: StatusConfirmed,
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				Status:     "PAID",
				Amount:     1_000_000,
				Currency:   "IDR",
			},
			wantOutcome: PaymentOutcomeAlreadyConfirmed,
		},
		{
			name:          "PAID with amount mismatch returns ErrPaymentAmountMismatch",
			initialStatus: StatusPending,
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				Status:     "PAID",
				Amount:     500_000,
				Currency:   "IDR",
			},
			wantErr: ErrPaymentAmountMismatch,
		},
		{
			name:          "PAID with currency mismatch returns ErrPaymentCurrencyMismatch",
			initialStatus: StatusPending,
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				Status:     "PAID",
				Amount:     1_000_000,
				Currency:   "USD",
			},
			wantErr: ErrPaymentCurrencyMismatch,
		},
		{
			name:          "PAID with mismatched invoice in attempts returns ErrInvoiceMismatch",
			initialStatus: StatusPending,
			setupSvc: func(s *Service) {
				s.SetPaymentAttemptStore(&mockPaymentAttemptStore{
					attempts: []PaymentAttempt{
						{ProviderReference: "inv-correct"},
					},
				})
			},
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				ID:         "inv-forged",
				Status:     "PAID",
				Amount:     1_000_000,
				Currency:   "IDR",
			},
			wantErr: ErrInvoiceMismatch,
		},
		{
			name:          "EXPIRED on confirmed booking returns STALE_IGNORED",
			initialStatus: StatusConfirmed,
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				Status:     "EXPIRED",
			},
			wantOutcome: PaymentOutcomeStaleIgnored,
		},
		{
			name:          "Unhandled status returns UNHANDLED_STATUS",
			initialStatus: StatusPending,
			event: PaymentEvent{
				ExternalID: "bk-test-1",
				Status:     "PENDING",
			},
			wantOutcome: PaymentOutcomeUnhandledStatus,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := baseBooking
			b.Status = tc.initialStatus
			reader := &fakeReader{bookings: map[string]*Booking{b.ID: &b}}
			tx := newFakeTx(nil, nil)
			tx.bookings[b.ID] = &b
			svc := newTestService(tx, reader)
			if tc.setupSvc != nil {
				tc.setupSvc(svc)
			}

			outcome, err := svc.ApplyPaymentEvent(context.Background(), tc.event)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if outcome.Status != tc.wantOutcome {
				t.Errorf("outcome.Status = %v, want %v", outcome.Status, tc.wantOutcome)
			}
		})
	}
}

type mockPaymentAttemptStore struct {
	attempts []PaymentAttempt
	err      error
}

func (m *mockPaymentAttemptStore) RecordAttempt(_ context.Context, _ PaymentAttempt) error {
	return nil
}
func (m *mockPaymentAttemptStore) UpdateAttemptStatus(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockPaymentAttemptStore) UpdateAttemptByID(_ context.Context, _, _, _ string, _ map[string]any) error {
	return nil
}
func (m *mockPaymentAttemptStore) GetAttemptsByBookingID(_ context.Context, _ string) ([]PaymentAttempt, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.attempts, nil
}
