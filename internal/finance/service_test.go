package finance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/payment"
)

type mockFinanceStore struct {
	balances       map[string]struct {
		captured int64
		refunded int64
		status   string
		invID    string
	}
	refunds        map[string][]PaymentRefund
	cases          map[string]*PaymentCase
	ownership      map[string]string // bookingID -> email
	summary        *ReconciliationSummary
	balanceErr     error
	createErr      error
	updateErr      error
	ownershipErr   error
}

func newMockFinanceStore() *mockFinanceStore {
	return &mockFinanceStore{
		balances:  make(map[string]struct{ captured, refunded int64; status, invID string }),
		refunds:   make(map[string][]PaymentRefund),
		cases:     make(map[string]*PaymentCase),
		ownership: make(map[string]string),
	}
}

func (m *mockFinanceStore) GetRefundableBalance(ctx context.Context, bookingID string) (capturedMinor, refundedMinor, remainingMinor int64, bookingStatus string, invoiceID string, err error) {
	if m.balanceErr != nil {
		return 0, 0, 0, "", "", m.balanceErr
	}
	b, ok := m.balances[bookingID]
	if !ok {
		return 0, 0, 0, "", "", ErrBookingNotFound
	}
	remaining := b.captured - b.refunded
	if remaining < 0 {
		remaining = 0
	}
	return b.captured, b.refunded, remaining, b.status, b.invID, nil
}

func (m *mockFinanceStore) CreateRefund(ctx context.Context, refund *PaymentRefund) error {
	if m.createErr != nil {
		return m.createErr
	}
	refund.ID = fmt.Sprintf("rfnd_mock_%d", len(m.refunds[refund.BookingID])+1)
	m.refunds[refund.BookingID] = append(m.refunds[refund.BookingID], *refund)

	// Update refunded balance
	if b, ok := m.balances[refund.BookingID]; ok {
		b.refunded += refund.AmountMinor
		m.balances[refund.BookingID] = b
	}
	return nil
}

func (m *mockFinanceStore) UpdateRefundStatus(ctx context.Context, refundID, status, providerRefundID string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	for bID, list := range m.refunds {
		for i, r := range list {
			if r.ID == refundID {
				list[i].Status = status
				list[i].ProviderRefundID = providerRefundID
				m.refunds[bID] = list
				return nil
			}
		}
	}
	return ErrRefundNotFound
}

func (m *mockFinanceStore) ListRefundsByBookingID(ctx context.Context, bookingID string) ([]PaymentRefund, error) {
	return m.refunds[bookingID], nil
}

func (m *mockFinanceStore) CreatePaymentCase(ctx context.Context, pc *PaymentCase) error {
	pc.ID = fmt.Sprintf("case_mock_%d", len(m.cases)+1)
	m.cases[pc.ID] = pc
	return nil
}

func (m *mockFinanceStore) GetPaymentCaseByID(ctx context.Context, caseID string) (*PaymentCase, error) {
	pc, ok := m.cases[caseID]
	if !ok {
		return nil, ErrCaseNotFound
	}
	pcCopy := *pc
	return &pcCopy, nil
}

func (m *mockFinanceStore) ListPaymentCases(ctx context.Context, status string, limit int) ([]PaymentCase, error) {
	var res []PaymentCase
	for _, pc := range m.cases {
		if status == "" || pc.Status == status {
			res = append(res, *pc)
		}
	}
	if len(res) > limit && limit > 0 {
		res = res[:limit]
	}
	return res, nil
}

func (m *mockFinanceStore) ResolvePaymentCase(ctx context.Context, caseID, action, notes, resolvedBy string) error {
	pc, ok := m.cases[caseID]
	if !ok {
		return ErrCaseNotFound
	}
	if pc.Status == "resolved" {
		return ErrCaseAlreadyResolved
	}
	now := time.Now()
	pc.Status = "resolved"
	pc.ResolutionAction = action
	pc.Notes = pc.Notes + " | " + notes
	pc.ResolvedBy = resolvedBy
	pc.ResolvedAt = &now
	return nil
}

func (m *mockFinanceStore) GetReconciliationSummary(ctx context.Context) (*ReconciliationSummary, error) {
	if m.summary != nil {
		return m.summary, nil
	}
	return &ReconciliationSummary{
		TotalSettledMinor:  100000000,
		TotalRefundedMinor: 10000000,
		NetCapturedMinor:   90000000,
		OpenCasesCount:     2,
		TotalRefundsCount:  3,
	}, nil
}

func (m *mockFinanceStore) VerifyBookingOwnership(ctx context.Context, bookingID, email string) (bool, error) {
	if m.ownershipErr != nil {
		return false, m.ownershipErr
	}
	owner, ok := m.ownership[bookingID]
	return ok && owner == email, nil
}

type mockGateway struct {
	res payment.RefundResult
	err error
}

func (g *mockGateway) CreateRefund(ctx context.Context, req payment.RefundRequest) (payment.RefundResult, error) {
	if g.err != nil {
		return payment.RefundResult{}, g.err
	}
	return g.res, nil
}

func TestFinanceService_ProcessRefund_TableTest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		input      CreateRefundInput
		setupStore func(s *mockFinanceStore)
		setupGW    func() *mockGateway
		expectErr  error
		verifyRes  func(t *testing.T, r *PaymentRefund)
	}{
		{
			name: "Valid full refund succeeds and sets status to succeeded",
			input: CreateRefundInput{
				BookingID:   "bk-001",
				AmountMinor: 1000000,
				Reason:      "Customer requested flexible cancellation",
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-001"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 1000000, refunded: 0, status: "confirmed", invID: "inv_123"}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{
					res: payment.RefundResult{RefundID: "rfd_xen_01", Status: "succeeded", Amount: 1000000},
				}
			},
			expectErr: nil,
			verifyRes: func(t *testing.T, r *PaymentRefund) {
				if r == nil {
					t.Fatalf("expected non-nil refund")
				}
				if r.Status != "succeeded" {
					t.Errorf("status = %v, want succeeded", r.Status)
				}
				if r.ProviderRefundID != "rfd_xen_01" {
					t.Errorf("provider refund id = %v, want rfd_xen_01", r.ProviderRefundID)
				}
			},
		},
		{
			name: "Valid partial refund succeeds and leaves remaining balance",
			input: CreateRefundInput{
				BookingID:   "bk-002",
				AmountMinor: 400000,
				Reason:      "Partial refund for unused breakfast",
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-002"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 1000000, refunded: 0, status: "confirmed", invID: "inv_456"}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{
					res: payment.RefundResult{RefundID: "rfd_xen_part", Status: "succeeded", Amount: 400000},
				}
			},
			expectErr: nil,
			verifyRes: func(t *testing.T, r *PaymentRefund) {
				if r.AmountMinor != 400000 {
					t.Errorf("amount = %d, want 400000", r.AmountMinor)
				}
			},
		},
		{
			name: "Over-refund attempt exceeding remaining balance is strictly rejected",
			input: CreateRefundInput{
				BookingID:   "bk-003",
				AmountMinor: 600000,
				Reason:      "Second partial refund exceeding remaining",
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-003"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 1000000, refunded: 500000, status: "confirmed", invID: "inv_789"}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{}
			},
			expectErr: ErrOverRefund,
			verifyRes: nil,
		},
		{
			name: "Refund on non-paid/pending booking is rejected",
			input: CreateRefundInput{
				BookingID:   "bk-004",
				AmountMinor: 500000,
				Reason:      "Customer changed mind before paying",
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-004"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 0, refunded: 0, status: "pending", invID: ""}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{}
			},
			expectErr: ErrBookingNotPaid,
			verifyRes: nil,
		},
		{
			name: "Short or empty reason is rejected with ErrReasonRequired",
			input: CreateRefundInput{
				BookingID:   "bk-001",
				AmountMinor: 500000,
				Reason:      "no", // less than 5 characters
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-001"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 1000000, refunded: 0, status: "confirmed", invID: "inv_123"}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{}
			},
			expectErr: ErrReasonRequired,
			verifyRes: nil,
		},
		{
			name: "Gateway error marks refund failed and returns ErrGatewayFailed",
			input: CreateRefundInput{
				BookingID:   "bk-005",
				AmountMinor: 500000,
				Reason:      "Customer requested refund",
				ActorID:     "fin_01",
				ActorRole:   "finance",
			},
			setupStore: func(s *mockFinanceStore) {
				s.balances["bk-005"] = struct {
					captured, refunded int64
					status, invID      string
				}{captured: 1000000, refunded: 0, status: "confirmed", invID: "inv_err"}
			},
			setupGW: func() *mockGateway {
				return &mockGateway{err: errors.New("connection timeout to bank")}
			},
			expectErr: ErrGatewayFailed,
			verifyRes: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockFinanceStore()
			tc.setupStore(store)
			gw := tc.setupGW()
			svc := NewService(store, gw, slog.Default())

			res, err := svc.ProcessRefund(ctx, tc.input)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.verifyRes != nil {
				tc.verifyRes(t, res)
			}
		})
	}
}

func TestFinanceService_LatePaymentCase_TableTest(t *testing.T) {
	ctx := context.Background()

	store := newMockFinanceStore()
	svc := NewService(store, nil, slog.Default())

	// 1. Create Late Payment Case
	pc, err := svc.CreateLatePaymentCase(ctx, "bk-late-001", "inv_late_xen_99", 850000, "Paid after hold expired")
	if err != nil {
		t.Fatalf("failed to create late payment case: %v", err)
	}
	if pc.Status != "open" || pc.CaseType != "late_payment" {
		t.Errorf("expected open late_payment case, got %+v", pc)
	}

	// 2. List Cases
	list, err := svc.ListCases(ctx, "open", 10)
	if err != nil {
		t.Fatalf("failed to list cases: %v", err)
	}
	if len(list) != 1 || list[0].BookingID != "bk-late-001" {
		t.Errorf("expected 1 case with booking bk-late-001, got %+v", list)
	}

	// 3. Resolve Case
	err = svc.ResolveCase(ctx, ResolveCaseInput{
		CaseID:  pc.ID,
		Action:  "refund",
		Notes:   "Refund processed manually due to sold out rooms",
		ActorID: "fin_admin",
	})
	if err != nil {
		t.Fatalf("failed to resolve case: %v", err)
	}

	// 4. Resolve again returns ErrCaseAlreadyResolved
	err = svc.ResolveCase(ctx, ResolveCaseInput{
		CaseID:  pc.ID,
		Action:  "refund",
		ActorID: "fin_admin",
	})
	if !errors.Is(err, ErrCaseAlreadyResolved) {
		t.Errorf("expected ErrCaseAlreadyResolved, got %v", err)
	}
}

func TestFinanceService_GuestRefundStatus_TableTest(t *testing.T) {
	ctx := context.Background()

	store := newMockFinanceStore()
	store.ownership["bk-own-01"] = "guest@example.com"
	store.refunds["bk-own-01"] = []PaymentRefund{
		{
			ID:          "rfnd-001",
			BookingID:   "bk-own-01",
			AmountMinor: 500000,
			Status:      "succeeded",
		},
	}

	svc := NewService(store, nil, slog.Default())

	// 1. Owner guest retrieves refund status (200 OK)
	statusView, err := svc.GetBookingRefundStatus(ctx, "guest@example.com", "bk-own-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusView.HasRefund || len(statusView.Refunds) != 1 {
		t.Errorf("expected 1 refund for owner guest, got %+v", statusView)
	}

	// 2. IDOR Attack: Hacker attempts to access another guest's refund status -> ErrBookingNotFound (404)
	_, err = svc.GetBookingRefundStatus(ctx, "hacker@example.com", "bk-own-01")
	if !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("IDOR vulnerability: expected ErrBookingNotFound, got %v", err)
	}
}

func TestFinanceService_ReconciliationSummary_TableTest(t *testing.T) {
	ctx := context.Background()

	store := newMockFinanceStore()
	svc := NewService(store, nil, slog.Default())

	sum, err := svc.GetReconciliationSummary(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.NetCapturedMinor != 90000000 {
		t.Errorf("net captured = %d, want 90000000", sum.NetCapturedMinor)
	}
}
