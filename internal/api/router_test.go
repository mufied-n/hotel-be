package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
)

type mockInvStore struct {
	avail []inventory.Availability
	err   error
}

func (m *mockInvStore) GetByDate(_ context.Context, _ string, _, _ time.Time) ([]inventory.Availability, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.avail, nil
}

type mockRates struct {
	quotes []rates.Quote
	err    error
}

func (m *mockRates) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quotes, nil
}

type mockTxRunner struct {
	fn func(booking.InventoryTx, booking.EventPublisher) error
}

func (m *mockTxRunner) InTx(ctx context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	if m.fn != nil {
		return m.fn(nil, nil)
	}
	return nil
}

type mockTx struct {
	booking booking.Booking
	rooms   []string
}

func (m *mockTx) LockAndDecrement(_ context.Context, _ string, _, _ time.Time, _ int) error { return nil }
func (m *mockTx) Increment(_ context.Context, _ string, _, _ time.Time, _ int) error        { return nil }
func (m *mockTx) InsertBookingWithHold(_ context.Context, b *booking.Booking, _ []rates.Quote, _ time.Time) error {
	b.ID = "bk-123"
	m.booking = *b
	return nil
}
func (m *mockTx) GetForUpdate(_ context.Context, id string) (booking.Booking, error) {
	if id == "not-found" {
		return booking.Booking{}, booking.ErrNotFound
	}
	return m.booking, nil
}
func (m *mockTx) UpdateStatus(_ context.Context, _ string, to booking.Status) error {
	m.booking.Status = to
	return nil
}
func (m *mockTx) PickAndAssignRooms(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	return m.rooms, nil
}
func (m *mockTx) GetRoomAssignments(_ context.Context, _ string) ([]string, error) {
	return m.rooms, nil
}
func (m *mockTx) PublishTx(_ context.Context, _ string, _ []byte) error { return nil }

type mockReader struct {
	booking booking.Booking
	err     error
}

func (m *mockReader) Get(_ context.Context, id string) (booking.Booking, error) {
	if m.err != nil {
		return booking.Booking{}, m.err
	}
	if id == "not-found" {
		return booking.Booking{}, booking.ErrNotFound
	}
	return m.booking, nil
}

type mockPayment struct{}

func (m *mockPayment) CreateCharge(_ context.Context, _ booking.Booking, _ int64, _ string) (booking.ChargeResult, error) {
	return booking.ChargeResult{PaymentURL: "http://pay.test", Reference: "ref-test"}, nil
}

type mockNotifier struct{}

func (m *mockNotifier) SendBookingConfirmed(_ context.Context, _ booking.Booking) error { return nil }

func setupTestRouter() (http.Handler, *mockTx) {
	txMock := &mockTx{
		booking: booking.Booking{
			ID:              "bk-123",
			Status:          booking.StatusConfirmed,
			RoomTypeID:      "std",
			NumRooms:        1,
			CheckIn:         time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
			CheckOut:        time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
			TotalPriceMinor: 1_000_000,
			GuestName:       "Budi Santoso",
			GuestEmail:      "budi@example.com",
			GuestToken:      "gst_valid_token_123",
		},
		rooms: []string{"101"},
	}

	runner := &testRunner{tx: txMock}
	inv := &mockInvStore{
		avail: []inventory.Availability{
			{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), TotalRooms: 5, AvailableRooms: 5},
			{Date: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), TotalRooms: 5, AvailableRooms: 5},
		},
	}
	ratesSvc := &mockRates{
		quotes: []rates.Quote{
			{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), RateMinor: 500_000},
			{Date: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), RateMinor: 500_000},
		},
	}
	reader := &mockReader{booking: txMock.booking}
	bkSvc := booking.NewService(runner, inv, ratesSvc, &mockPayment{}, &mockNotifier{}, reader, 30*time.Minute, nil)

	handler := NewRouter(Deps{
		BookingSvc:    bkSvc,
		InvStore:      inv,
		RateSvc:       ratesSvc,
		Enqueuer:      &workers.Enqueuer{}, // won't panic if client is nil unless called, or mock client
		Enforcer:      auth.DefaultTestEnforcer(),
		IsDevelopment: true,
		ReadyCheck:    func(_ context.Context) error { return nil },
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	return handler, txMock
}

type testRunner struct{ tx *mockTx }

func (r *testRunner) InTx(_ context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	return fn(r.tx, r.tx)
}

func TestHealthz(t *testing.T) {
	h, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestReady(t *testing.T) {
	// Case 1: Healthy
	h, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ready status = %d, want 200", w.Code)
	}

	// Case 2: Unhealthy
	hUnhealthy := NewRouter(Deps{
		ReadyCheck: func(_ context.Context) error { return errors.New("valkey ping failed") },
	})
	req2 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w2 := httptest.NewRecorder()
	hUnhealthy.ServeHTTP(w2, req2)

	if w2.Code != http.StatusServiceUnavailable {
		t.Errorf("unhealthy status = %d, want 503", w2.Code)
	}
}

func TestGetAvailability(t *testing.T) {
	h, _ := setupTestRouter()

	// Happy path
	req := httptest.NewRequest(http.MethodGet, "/api/v1/availability?room_type_id=std&check_in=2026-10-10&check_out=2026-10-12", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// Bad input (check_out before check_in)
	reqBad := httptest.NewRequest(http.MethodGet, "/api/v1/availability?room_type_id=std&check_in=2026-10-12&check_out=2026-10-10", nil)
	wBad := httptest.NewRecorder()
	h.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", wBad.Code)
	}
}

func TestCreateBooking_Validation400(t *testing.T) {
	h, _ := setupTestRouter()

	// check_out before check_in
	body := map[string]any{
		"room_type_id": "std",
		"check_in":     "2026-10-15",
		"check_out":    "2026-10-10",
		"num_rooms":    1,
		"num_guests":   1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 Bad Request, got %s", w.Code, w.Body.String())
	}

	// 0 rooms
	body["check_in"] = "2026-10-10"
	body["check_out"] = "2026-10-12"
	body["num_rooms"] = 0
	b, _ = json.Marshal(body)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewReader(b))
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 Bad Request", w2.Code)
	}
}

func TestGetBooking(t *testing.T) {
	h, _ := setupTestRouter()

	t.Run("public guest without token receives masked PublicDTO", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-123", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, exists := resp["guest_name"]; exists {
			t.Errorf("guest_name leaked in public DTO: %v", resp["guest_name"])
		}
		if _, exists := resp["guest_email"]; exists {
			t.Errorf("guest_email leaked in public DTO: %v", resp["guest_email"])
		}
		if _, exists := resp["guest_token"]; exists {
			t.Errorf("guest_token leaked in public DTO: %v", resp["guest_token"])
		}
		if resp["id"] != "bk-123" {
			t.Errorf("expected id bk-123, got %v", resp["id"])
		}
	})

	t.Run("guest with valid X-Guest-Token receives full booking with PII", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-123", nil)
		req.Header.Set("X-Guest-Token", "gst_valid_token_123")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var b booking.Booking
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if b.GuestName != "Budi Santoso" || b.GuestEmail != "budi@example.com" {
			t.Errorf("expected full PII, got name=%q email=%q", b.GuestName, b.GuestEmail)
		}
	})

	t.Run("staff role receives full booking with PII", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-123", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var b booking.Booking
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if b.GuestName != "Budi Santoso" {
			t.Errorf("expected full PII for staff, got name=%q", b.GuestName)
		}
	})

	t.Run("not found", func(t *testing.T) {
		reqNF := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/not-found", nil)
		wNF := httptest.NewRecorder()
		h.ServeHTTP(wNF, reqNF)

		if wNF.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", wNF.Code)
		}
	})
}

func TestCancelBooking(t *testing.T) {
	h, tx := setupTestRouter()

	t.Run("guest with invalid or missing token is rejected 403", func(t *testing.T) {
		tx.booking.Status = booking.StatusPending
		req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/cancel", nil)
		req.Header.Set("X-Guest-Token", "wrong_token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("cancel status = %d, want 403", w.Code)
		}
		if tx.booking.Status != booking.StatusPending {
			t.Errorf("status should not change, got %s", tx.booking.Status)
		}
	})

	t.Run("guest with valid X-Guest-Token can cancel", func(t *testing.T) {
		tx.booking.Status = booking.StatusPending
		req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/cancel", nil)
		req.Header.Set("X-Guest-Token", "gst_valid_token_123")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("cancel status = %d, want 200", w.Code)
		}
		if tx.booking.Status != booking.StatusCancelled {
			t.Errorf("status = %s, want cancelled", tx.booking.Status)
		}
	})

	t.Run("staff role can cancel without guest token", func(t *testing.T) {
		tx.booking.Status = booking.StatusPending
		req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/cancel", nil)
		req.Header.Set("Authorization", "Bearer gm_admin")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("cancel status = %d, want 200", w.Code)
		}
		if tx.booking.Status != booking.StatusCancelled {
			t.Errorf("status = %s, want cancelled", tx.booking.Status)
		}
	})
}

func TestCheckIn(t *testing.T) {
	h, tx := setupTestRouter()
	tx.booking.Status = booking.StatusConfirmed

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/check-in", nil)
	req.Header.Set("Authorization", "Bearer receptionist")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("check-in status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if tx.booking.Status != booking.StatusCheckedIn {
		t.Errorf("status = %s, want checked_in", tx.booking.Status)
	}
}

func TestCheckOut(t *testing.T) {
	h, tx := setupTestRouter()
	tx.booking.Status = booking.StatusCheckedIn

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/check-out", nil)
	req.Header.Set("Authorization", "Bearer receptionist")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("check-out status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if tx.booking.Status != booking.StatusCheckedOut {
		t.Errorf("status = %s, want checked_out", tx.booking.Status)
	}
}

func TestNoShow(t *testing.T) {
	h, tx := setupTestRouter()
	tx.booking.Status = booking.StatusConfirmed

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/no-show", nil)
	req.Header.Set("Authorization", "Bearer receptionist")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("no-show status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if tx.booking.Status != booking.StatusNoShow {
		t.Errorf("status = %s, want no_show", tx.booking.Status)
	}
}

func TestDevRouteGating(t *testing.T) {
	// Dev mode: FakePay mounted (BE-G10)
	devRouter := NewRouter(Deps{
		Enforcer:      auth.DefaultTestEnforcer(),
		IsDevelopment: true,
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	reqDev := httptest.NewRequest(http.MethodPost, "/fake-pay/ref-123", nil)
	wDev := httptest.NewRecorder()
	devRouter.ServeHTTP(wDev, reqDev)
	if wDev.Code != http.StatusOK {
		t.Errorf("dev mode fake-pay status = %d, want 200", wDev.Code)
	}

	// Prod mode: FakePay NOT mounted -> 404 Not Found (BE-G10)
	prodRouter := NewRouter(Deps{
		Enforcer:      auth.DefaultTestEnforcer(),
		IsDevelopment: false,
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	reqProd := httptest.NewRequest(http.MethodPost, "/fake-pay/ref-123", nil)
	wProd := httptest.NewRecorder()
	prodRouter.ServeHTTP(wProd, reqProd)
	if wProd.Code != http.StatusNotFound {
		t.Errorf("prod mode fake-pay status = %d, want 404", wProd.Code)
	}
}

func TestFailClosedEnforcer(t *testing.T) {
	// Fail-closed: enforcer is nil -> 503 Service Unavailable (BE-G14)
	router := NewRouter(Deps{
		Enforcer: nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/availability?room_type_id=std&check_in=2026-10-10&check_out=2026-10-12", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("nil enforcer status = %d, want 503", w.Code)
	}
	var pd ProblemDetails
	if err := json.Unmarshal(w.Body.Bytes(), &pd); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if pd.Code != "AUTH_SERVICE_UNAVAILABLE" {
		t.Errorf("expected code AUTH_SERVICE_UNAVAILABLE, got %s", pd.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(1, 2) // 1 token/sec, capacity 2
	router := NewRouter(Deps{
		Enforcer:    auth.DefaultTestEnforcer(),
		RateLimiter: rl,
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "192.168.1.50:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, w.Code)
		}
	}

	// 3rd request exceeds capacity
	req3 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req3.RemoteAddr = "192.168.1.50:12345"
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("request 3 status = %d, want 429", w3.Code)
	}
}

func TestRouterErrorBranches(t *testing.T) {
	// Ready check failure (503)
	failingReadyDeps := Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		ReadyCheck: func(ctx context.Context) error {
			return errors.New("db down")
		},
	}
	rReadyFail := NewRouter(failingReadyDeps)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	rReadyFail.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("ready fail status = %d, want 503", w.Code)
	}

	h, _ := setupTestRouter()

	errorPaths := []struct {
		name       string
		method     string
		path       string
		body       string
		role       string
		guestToken string
		wantStatus int
	}{
		{"check-in not found", http.MethodPost, "/api/v1/bookings/not-found/check-in", "", "receptionist", "", http.StatusNotFound},
		{"check-out not found", http.MethodPost, "/api/v1/bookings/not-found/check-out", "", "receptionist", "", http.StatusNotFound},
		{"no-show not found", http.MethodPost, "/api/v1/bookings/not-found/no-show", "", "receptionist", "", http.StatusNotFound},
		{"cancel not found", http.MethodPost, "/api/v1/bookings/not-found/cancel", "", "guest", "some-token", http.StatusNotFound},
		{"availability invalid query", http.MethodGet, "/api/v1/availability?room_type_id=", "", "guest", "", http.StatusBadRequest},
		{"availability invalid dates", http.MethodGet, "/api/v1/availability?room_type_id=1&check_in=2026-10-15&check_out=2026-10-10", "", "guest", "", http.StatusBadRequest},
		{"create booking bad json", http.MethodPost, "/api/v1/bookings", "{invalid-json", "guest", "", http.StatusBadRequest},
		{"create booking invalid date format", http.MethodPost, "/api/v1/bookings", `{"room_type_id":"std","check_in":"bad","check_out":"2026-10-12"}`, "guest", "", http.StatusBadRequest},
	}

	for _, tt := range errorPaths {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Buffer
			if tt.body != "" {
				bodyReader = bytes.NewBufferString(tt.body)
			} else {
				bodyReader = bytes.NewBuffer(nil)
			}
			r := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.role != "" && tt.role != "guest" {
				r.Header.Set("Authorization", "Bearer "+tt.role)
			}
			if tt.guestToken != "" {
				r.Header.Set("X-Guest-Token", tt.guestToken)
			}
			rw := httptest.NewRecorder()
			h.ServeHTTP(rw, r)
			if rw.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rw.Code, tt.wantStatus, rw.Body.String())
			}
		})
	}

	// Test availability store not found (404)
	rAvailNotFound := NewRouter(Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		InvStore: &mockInvStore{err: inventory.ErrNotFound},
		RateSvc:  &mockRates{},
	})
	rReq := httptest.NewRequest(http.MethodGet, "/api/v1/availability?room_type_id=std&check_in=2026-10-10&check_out=2026-10-12", nil)
	rRec := httptest.NewRecorder()
	rAvailNotFound.ServeHTTP(rRec, rReq)
	if rRec.Code != http.StatusNotFound {
		t.Errorf("avail not found status = %d, want 404", rRec.Code)
	}

	// Test availability rate provider error (404)
	now := time.Now()
	rRateErr := NewRouter(Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		InvStore: &mockInvStore{avail: []inventory.Availability{{Date: now, AvailableRooms: 5}}},
		RateSvc:  &mockRates{err: errors.New("rate error")},
	})
	rateReq := httptest.NewRequest(http.MethodGet, "/api/v1/availability?room_type_id=std&check_in=2026-10-10&check_out=2026-10-12", nil)
	rateRec := httptest.NewRecorder()
	rRateErr.ServeHTTP(rateRec, rateReq)
	if rateRec.Code != http.StatusNotFound {
		t.Errorf("rate err status = %d, want 404", rateRec.Code)
	}
}
