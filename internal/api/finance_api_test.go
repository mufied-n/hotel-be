package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/platform/auth"
)

type mockFinanceAPIService struct {
	processRefundFunc            func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error)
	createLatePaymentCaseFunc    func(ctx context.Context, bookingID, providerRef string, amountMinor int64, notes string) (*finance.PaymentCase, error)
	listCasesFunc                func(ctx context.Context, status string, limit int) ([]finance.PaymentCase, error)
	resolveCaseFunc              func(ctx context.Context, input finance.ResolveCaseInput) error
	getBookingRefundStatusFunc   func(ctx context.Context, email, bookingID string) (*finance.RefundStatusView, error)
	getReconciliationSummaryFunc func(ctx context.Context) (*finance.ReconciliationSummary, error)
}

func (m *mockFinanceAPIService) ProcessRefund(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
	if m.processRefundFunc != nil {
		return m.processRefundFunc(ctx, input)
	}
	return &finance.PaymentRefund{
		ID:               "rfnd_mock_123",
		BookingID:        input.BookingID,
		ReferenceID:      "rfnd-REF123-1",
		AmountMinor:      input.AmountMinor,
		Currency:         "IDR",
		Reason:           input.Reason,
		Status:           "succeeded",
		Provider:         "xendit",
		ProviderRefundID: "rfd_xen_123",
		ActorID:          input.ActorID,
		ActorRole:        input.ActorRole,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *mockFinanceAPIService) CreateLatePaymentCase(ctx context.Context, bookingID, providerRef string, amountMinor int64, notes string) (*finance.PaymentCase, error) {
	if m.createLatePaymentCaseFunc != nil {
		return m.createLatePaymentCaseFunc(ctx, bookingID, providerRef, amountMinor, notes)
	}
	return &finance.PaymentCase{
		ID:                "case_mock_123",
		BookingID:         bookingID,
		CaseType:          "late_payment",
		Status:            "open",
		AmountMinor:       amountMinor,
		Currency:          "IDR",
		ProviderReference: providerRef,
		Notes:             notes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}, nil
}

func (m *mockFinanceAPIService) ListCases(ctx context.Context, status string, limit int) ([]finance.PaymentCase, error) {
	if m.listCasesFunc != nil {
		return m.listCasesFunc(ctx, status, limit)
	}
	return []finance.PaymentCase{
		{
			ID:                "case_01",
			BookingID:         "bk_late_01",
			CaseType:          "late_payment",
			Status:            "open",
			AmountMinor:       750000,
			Currency:          "IDR",
			ProviderReference: "inv_late_01",
			CreatedAt:         time.Now(),
		},
	}, nil
}

func (m *mockFinanceAPIService) ResolveCase(ctx context.Context, input finance.ResolveCaseInput) error {
	if m.resolveCaseFunc != nil {
		return m.resolveCaseFunc(ctx, input)
	}
	return nil
}

func (m *mockFinanceAPIService) GetBookingRefundStatus(ctx context.Context, email, bookingID string) (*finance.RefundStatusView, error) {
	if m.getBookingRefundStatusFunc != nil {
		return m.getBookingRefundStatusFunc(ctx, email, bookingID)
	}
	return &finance.RefundStatusView{
		BookingID: bookingID,
		HasRefund: true,
		Refunds: []finance.PaymentRefund{
			{
				ID:          "rfnd_view_01",
				BookingID:   bookingID,
				AmountMinor: 500000,
				Status:      "succeeded",
			},
		},
	}, nil
}

func (m *mockFinanceAPIService) GetReconciliationSummary(ctx context.Context) (*finance.ReconciliationSummary, error) {
	if m.getReconciliationSummaryFunc != nil {
		return m.getReconciliationSummaryFunc(ctx)
	}
	return &finance.ReconciliationSummary{
		TotalSettledMinor:  100000000,
		TotalRefundedMinor: 10000000,
		NetCapturedMinor:   90000000,
		OpenCasesCount:     2,
		TotalRefundsCount:  3,
	}, nil
}

var _ finance.Service = (*mockFinanceAPIService)(nil)

func setupFinanceTestRouter(t *testing.T, finSvc finance.Service, guestSvc guest.Service) http.Handler {
	t.Helper()

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "finance", "/api/v1/finance/refunds", "POST"},
		{"p", "finance", "/api/v1/finance/cases", "GET"},
		{"p", "finance", "/api/v1/finance/cases/:id/resolve", "POST"},
		{"p", "finance", "/api/v1/finance/reconciliations", "GET"},
		{"p", "gm_admin", "/api/v1/finance/*", "*"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create in-memory enforcer: %v", err)
	}

	return NewRouter(Deps{
		StaffAuth:  TestStaffVerifier(),
		Enforcer:   enforcer,
		FinanceSvc: finSvc,
		GuestSvc:   guestSvc,
	})
}

func TestFinanceAPI_ProcessRefund_TableTest(t *testing.T) {
	tests := []struct {
		name         string
		authHeader   string
		roleHeader   string
		body         string
		setupMock    func(m *mockFinanceAPIService)
		expectCode   int
		expectStatus string
	}{
		{
			name:       "Finance role with valid request creates refund (201 Created)",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-001","amount_minor":1000000,"reason":"Flexible cancellation"}`,
			setupMock:  nil,
			expectCode: http.StatusCreated,
		},
		{
			name:       "GM Admin role can also initiate refund (201 Created)",
			authHeader: "Bearer gm_admin",
			body:       `{"booking_id":"bk-001","amount_minor":1000000,"reason":"Executive cancellation"}`,
			setupMock:  nil,
			expectCode: http.StatusCreated,
		},
		{
			name:       "Receptionist role is forbidden from refunds (403 Forbidden)",
			authHeader: "Bearer receptionist",
			body:       `{"booking_id":"bk-001","amount_minor":1000000,"reason":"Customer cancellation"}`,
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Guest / unauthenticated is forbidden (403 Forbidden)",
			authHeader: "",
			body:       `{"booking_id":"bk-001","amount_minor":1000000,"reason":"Customer cancellation"}`,
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Malformed JSON payload returns 400 Bad Request",
			authHeader: "Bearer finance",
			body:       `{not-valid-json`,
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Booking not found returns 404 Not Found",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-fake","amount_minor":500000,"reason":"Room cancelled"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.processRefundFunc = func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
					return nil, finance.ErrBookingNotFound
				}
			},
			expectCode: http.StatusNotFound,
		},
		{
			name:       "Short or empty reason returns 400 Bad Request",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-001","amount_minor":500000,"reason":"no"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.processRefundFunc = func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
					return nil, finance.ErrReasonRequired
				}
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Over-refund attempt exceeding remaining balance returns 409 Conflict",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-001","amount_minor":99999999,"reason":"Exceeding refund balance"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.processRefundFunc = func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
					return nil, finance.ErrOverRefund
				}
			},
			expectCode: http.StatusConflict,
		},
		{
			name:       "Refund on unpaid booking returns 409 Conflict",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-001","amount_minor":500000,"reason":"Unpaid booking refund"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.processRefundFunc = func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
					return nil, finance.ErrBookingNotPaid
				}
			},
			expectCode: http.StatusConflict,
		},
		{
			name:       "Gateway connection failure returns 502 Bad Gateway",
			authHeader: "Bearer finance",
			body:       `{"booking_id":"bk-001","amount_minor":500000,"reason":"Gateway timeout test"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.processRefundFunc = func(ctx context.Context, input finance.CreateRefundInput) (*finance.PaymentRefund, error) {
					return nil, fmt.Errorf("%w: bank connection lost", finance.ErrGatewayFailed)
				}
			},
			expectCode: http.StatusBadGateway,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finMock := &mockFinanceAPIService{}
			if tc.setupMock != nil {
				tc.setupMock(finMock)
			}
			router := setupFinanceTestRouter(t, finMock, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/refunds", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("status code = %d, want %d; body: %s", w.Code, tc.expectCode, w.Body.String())
			}
		})
	}
}

func TestFinanceAPI_PaymentCases_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		authHeader string
		body       string
		setupMock  func(m *mockFinanceAPIService)
		expectCode int
	}{
		{
			name:       "Finance role can list payment cases (200 OK)",
			method:     http.MethodGet,
			path:       "/api/v1/finance/cases?status=open&limit=10",
			authHeader: "Bearer finance",
			setupMock:  nil,
			expectCode: http.StatusOK,
		},
		{
			name:       "Receptionist role forbidden from listing cases (403 Forbidden)",
			method:     http.MethodGet,
			path:       "/api/v1/finance/cases",
			authHeader: "Bearer receptionist",
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Finance role can resolve payment case (200 OK)",
			method:     http.MethodPost,
			path:       "/api/v1/finance/cases/case_01/resolve",
			authHeader: "Bearer finance",
			body:       `{"action":"refund","notes":"Manual refund issued"}`,
			setupMock:  nil,
			expectCode: http.StatusOK,
		},
		{
			name:       "Resolve without action returns 400 Bad Request",
			method:     http.MethodPost,
			path:       "/api/v1/finance/cases/case_01/resolve",
			authHeader: "Bearer finance",
			body:       `{"action":"","notes":"no action"}`,
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Resolve already resolved case returns 409 Conflict",
			method:     http.MethodPost,
			path:       "/api/v1/finance/cases/case_01/resolve",
			authHeader: "Bearer finance",
			body:       `{"action":"refund","notes":"re-resolve"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.resolveCaseFunc = func(ctx context.Context, input finance.ResolveCaseInput) error {
					return finance.ErrCaseAlreadyResolved
				}
			},
			expectCode: http.StatusConflict,
		},
		{
			name:       "Resolve non-existent case returns 404 Not Found",
			method:     http.MethodPost,
			path:       "/api/v1/finance/cases/case_fake/resolve",
			authHeader: "Bearer finance",
			body:       `{"action":"refund","notes":"ghost"}`,
			setupMock: func(m *mockFinanceAPIService) {
				m.resolveCaseFunc = func(ctx context.Context, input finance.ResolveCaseInput) error {
					return finance.ErrCaseNotFound
				}
			},
			expectCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finMock := &mockFinanceAPIService{}
			if tc.setupMock != nil {
				tc.setupMock(finMock)
			}
			router := setupFinanceTestRouter(t, finMock, nil)

			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("status code = %d, want %d; body: %s", w.Code, tc.expectCode, w.Body.String())
			}
		})
	}
}

func TestFinanceAPI_ReconciliationsSummary_TableTest(t *testing.T) {
	finMock := &mockFinanceAPIService{}
	router := setupFinanceTestRouter(t, finMock, nil)

	// 1. Finance role access summary -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/reconciliations", nil)
	req.Header.Set("Authorization", "Bearer finance")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("finance summary status = %d, want %d", w.Code, http.StatusOK)
	}

	// 2. Receptionist access summary -> 403 Forbidden
	reqForbidden := httptest.NewRequest(http.MethodGet, "/api/v1/finance/reconciliations", nil)
	reqForbidden.Header.Set("Authorization", "Bearer receptionist")
	wForbidden := httptest.NewRecorder()
	router.ServeHTTP(wForbidden, reqForbidden)

	if wForbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden summary status = %d, want %d", wForbidden.Code, http.StatusForbidden)
	}
}

func TestFinanceAPI_GuestRefundStatus_TableTest(t *testing.T) {
	guestSvcMock := &mockGuestService{
		validateSessionFunc: func(ctx context.Context, rawToken string) (*guest.GuestSession, error) {
			if rawToken == "gst_sess_valid" {
				return &guest.GuestSession{
					ID:         "sess_01",
					GuestEmail: "guest@example.com",
					ExpiresAt:  time.Now().Add(24 * time.Hour),
				}, nil
			}
			return nil, guest.ErrSessionNotFound
		},
	}

	finMock := &mockFinanceAPIService{
		getBookingRefundStatusFunc: func(ctx context.Context, email, bookingID string) (*finance.RefundStatusView, error) {
			if email != "guest@example.com" || bookingID == "bk_idor_target" {
				return nil, finance.ErrBookingNotFound // Anti-IDOR
			}
			return &finance.RefundStatusView{
				BookingID: bookingID,
				HasRefund: true,
				Refunds: []finance.PaymentRefund{
					{
						ID:          "rfnd_001",
						BookingID:   bookingID,
						AmountMinor: 450000,
						Status:      "succeeded",
					},
				},
			}, nil
		},
	}

	router := setupFinanceTestRouter(t, finMock, guestSvcMock)

	// 1. Valid guest session retrieving their own refund status -> 200 OK
	reqValid := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/bk_my_01/refund-status", nil)
	reqValid.Header.Set("Authorization", "Bearer gst_sess_valid")
	wValid := httptest.NewRecorder()
	router.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Fatalf("valid guest refund status code = %d, want 200; body: %s", wValid.Code, wValid.Body.String())
	}

	// 2. Anti-IDOR: Guest attempts to access someone else's booking -> 404 Not Found
	reqIDOR := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/bk_idor_target/refund-status", nil)
	reqIDOR.Header.Set("Authorization", "Bearer gst_sess_valid")
	wIDOR := httptest.NewRecorder()
	router.ServeHTTP(wIDOR, reqIDOR)

	if wIDOR.Code != http.StatusNotFound {
		t.Fatalf("idor attack status code = %d, want 404; body: %s", wIDOR.Code, wIDOR.Body.String())
	}

	// 3. Unauthenticated guest -> 401 Unauthorized
	reqAnon := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/bk_my_01/refund-status", nil)
	wAnon := httptest.NewRecorder()
	router.ServeHTTP(wAnon, reqAnon)

	if wAnon.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status code = %d, want 401; body: %s", wAnon.Code, wAnon.Body.String())
	}
}
