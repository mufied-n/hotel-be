package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
)

type testDecoder struct {
	r io.Reader
}

func newTestDecoder(r io.Reader) *testDecoder {
	return &testDecoder{r: r}
}

func (d *testDecoder) Decode(v any) error {
	b, err := io.ReadAll(d.r)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

type mockCatalogStore struct {
	variants []catalog.RoomVariant
	err      error
}

func (m *mockCatalogStore) ListVariants(_ context.Context) ([]catalog.RoomVariant, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.variants, nil
}

func (m *mockCatalogStore) GetVariant(_ context.Context, idOrCode string) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	for _, v := range m.variants {
		if v.ID == idOrCode || v.Code == idOrCode {
			return v, nil
		}
	}
	return catalog.RoomVariant{}, catalog.ErrVariantNotFound
}

func (m *mockCatalogStore) CreateVariant(_ context.Context, v catalog.RoomVariant) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	if v.Code == "dup" {
		return catalog.RoomVariant{}, catalog.ErrDuplicateCode
	}
	if v.Code == "" {
		return catalog.RoomVariant{}, catalog.ErrInvalidVariant
	}
	v.ID = "generated-variant-id"
	m.variants = append(m.variants, v)
	return v, nil
}

func (m *mockCatalogStore) UpdateVariant(_ context.Context, id string, v catalog.RoomVariant) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	if id == "not-found" {
		return catalog.RoomVariant{}, catalog.ErrVariantNotFound
	}
	if v.Code == "dup" {
		return catalog.RoomVariant{}, catalog.ErrDuplicateCode
	}
	if v.Code == "" {
		return catalog.RoomVariant{}, catalog.ErrInvalidVariant
	}
	v.ID = id
	return v, nil
}

func (m *mockCatalogStore) DeleteVariant(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if id == "not-found" {
		return catalog.ErrVariantNotFound
	}
	if id == "in-use" {
		return catalog.ErrCannotDelete
	}
	return nil
}

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
	booking   booking.Booking
	rooms     []string
	insertErr error
}

func (m *mockTx) LockAndDecrement(_ context.Context, _ string, _, _ time.Time, _ int) error { return nil }
func (m *mockTx) Increment(_ context.Context, _ string, _, _ time.Time, _ int) error        { return nil }
func (m *mockTx) InsertBookingWithHold(_ context.Context, b *booking.Booking, _ []rates.Quote, holdExpiresAt time.Time) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	b.ID = "bk-123"
	b.ExpiresAt = &holdExpiresAt
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
	tx      *mockTx
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
	if m.tx != nil {
		return m.tx.booking, nil
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
	return setupTestRouterWithStore(nil)
}

// setupTestRouterWithStore sama seperti setupTestRouter tetapi menyuntikkan IdempotencyStore (nil = default memori).
func setupTestRouterWithStore(store IdempotencyStore) (http.Handler, *mockTx) {
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
	reader := &mockReader{tx: txMock}
	bkSvc := booking.NewService(runner, inv, ratesSvc, &mockPayment{}, &mockNotifier{}, reader, 30*time.Minute, nil)

	rateEngine := rates.NewEngine(map[string]int64{
		"std":                                  500_000,
		"01900000-0000-7000-8000-000000000001": 550_000,
	}, 1.25)
	quoteStore := rateEngine.QuoteStore()
	bkSvc.SetQuoteStore(quoteStore)

	handler := NewRouter(Deps{
		BookingSvc:       bkSvc,
		IdempotencyStore: store,
		InvStore:         inv,
		RateSvc:          ratesSvc,
		RateEngine:       rateEngine,
		QuoteStore:       quoteStore,
		Enqueuer:         &workers.Enqueuer{}, // won't panic if client is nil unless called, or mock client
		Enforcer:         auth.DefaultTestEnforcer(),
		IsDevelopment:    true,
		ReadyCheck:       func(_ context.Context) error { return nil },
		FakePay: func(c *gin.Context) {
			c.Status(http.StatusOK)
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
	t.Run("success on check-in date", func(t *testing.T) {
		h, tx := setupTestRouter()
		tx.booking.Status = booking.StatusConfirmed
		tx.booking.CheckIn = time.Now()

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
	})

	t.Run("rejected before check-in date", func(t *testing.T) {
		h, tx := setupTestRouter()
		tx.booking.Status = booking.StatusConfirmed
		tx.booking.CheckIn = time.Now().Add(48 * time.Hour)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/no-show", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("no-show status = %d, want 400; body: %s", w.Code, w.Body.String())
		}
		var pd ProblemDetails
		_ = newTestDecoder(w.Body).Decode(&pd)
		if pd.Code != "NO_SHOW_TOO_EARLY" {
			t.Errorf("expected code NO_SHOW_TOO_EARLY, got %s", pd.Code)
		}
		if tx.booking.Status != booking.StatusConfirmed {
			t.Errorf("expected status confirmed, got %s", tx.booking.Status)
		}
	})
}

func TestDevRouteGating(t *testing.T) {
	// Dev mode: FakePay mounted (BE-G10)
	devRouter := NewRouter(Deps{
		Enforcer:      auth.DefaultTestEnforcer(),
		IsDevelopment: true,
		FakePay: func(c *gin.Context) {
			c.Status(http.StatusOK)
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
		FakePay: func(c *gin.Context) {
			c.Status(http.StatusOK)
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

func TestGetCatalogRooms(t *testing.T) {
	h, _ := setupTestRouter()

	// Happy path: returns 7 variants
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/rooms", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var res struct {
		Total int                   `json:"total"`
		Rooms []catalog.RoomVariant `json:"rooms"`
	}
	if err := newTestDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if res.Total != 7 || len(res.Rooms) != 7 {
		t.Errorf("total = %d, rooms len = %d, want 7", res.Total, len(res.Rooms))
	}

	// Error path: catalog store failure (500)
	hFail := NewRouter(Deps{
		Enforcer:     auth.DefaultTestEnforcer(),
		CatalogStore: &mockCatalogStore{err: errors.New("db disk failure")},
	})
	recFail := httptest.NewRecorder()
	hFail.ServeHTTP(recFail, req)
	if recFail.Code != http.StatusInternalServerError {
		t.Errorf("catalog error status = %d, want 500", recFail.Code)
	}
}

func TestSearchRooms_Validation(t *testing.T) {
	h, _ := setupTestRouter()
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	in10d := today.AddDate(0, 0, 10).Format("2006-01-02")
	in12d := today.AddDate(0, 0, 12).Format("2006-01-02")
	in45d := today.AddDate(0, 0, 45).Format("2006-01-02")
	past := today.AddDate(0, 0, -5).Format("2006-01-02")
	horizonExceedIn := today.AddDate(1, 0, 10).Format("2006-01-02")
	horizonExceedOut := today.AddDate(1, 0, 12).Format("2006-01-02")

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing dates",
			query:      "",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name:       "malformed check_in",
			query:      "check_in=invalid&check_out=" + in12d,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name:       "check_out before check_in",
			query:      fmt.Sprintf("check_in=%s&check_out=%s", in12d, in10d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_RANGE",
		},
		{
			name:       "stay exceeds 30 nights",
			query:      fmt.Sprintf("check_in=%s&check_out=%s", in10d, in45d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "EXCEEDS_MAX_LOS",
		},
		{
			name:       "check_in in the past",
			query:      fmt.Sprintf("check_in=%s&check_out=%s", past, in10d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "PAST_DATE",
		},
		{
			name:       "check_out exceeds 365 days horizon",
			query:      fmt.Sprintf("check_in=%s&check_out=%s", horizonExceedIn, horizonExceedOut),
			wantStatus: http.StatusBadRequest,
			wantCode:   "EXCEEDS_HORIZON",
		},
		{
			name:       "invalid adults count zero",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&adults=0", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "non-numeric adults",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&adults=two", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "invalid rooms count zero",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&rooms=0", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_COUNT",
		},
		{
			name:       "rooms count exceeds 8",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&rooms=9", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_COUNT",
		},
		{
			name:       "negative children count",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&children=-1", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "child age exceeds 17",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&child_ages=5,18", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_CHILD_AGE",
		},
		{
			name:       "negative child age",
			query:      fmt.Sprintf("check_in=%s&check_out=%s&child_ages=-1", in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_CHILD_AGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?"+tt.query, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			var prob ProblemDetails
			_ = newTestDecoder(rec.Body).Decode(&prob)
			if prob.Code != tt.wantCode {
				t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
			}
		})
	}
}

func TestSearchRooms_ContinuityAndStock(t *testing.T) {
	// Case 1: Happy path with standard availability
	h, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var res struct {
		TotalVariants  int                `json:"total_variants"`
		AvailableCount int                `json:"available_count"`
		Results        []SearchResultItem `json:"results"`
	}
	if err := newTestDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if res.TotalVariants != 7 {
		t.Errorf("total_variants = %d, want 7", res.TotalVariants)
	}
	if res.AvailableCount == 0 {
		t.Errorf("available_count = 0, want > 0")
	}

	// Case 2: Exceeds capacity
	reqCap := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=5&rooms=1", nil)
	recCap := httptest.NewRecorder()
	h.ServeHTTP(recCap, reqCap)
	var resCap struct {
		Results []SearchResultItem `json:"results"`
	}
	_ = newTestDecoder(recCap.Body).Decode(&resCap)
	for _, item := range resCap.Results {
		if item.RoomVariant.MaxAdults < 5 {
			if item.Available {
				t.Errorf("room %s with max_adults=%d should not be available for 5 adults", item.RoomVariant.Code, item.RoomVariant.MaxAdults)
			}
			if item.UnavailableReason != "EXCEEDS_CAPACITY" {
				t.Errorf("room %s reason = %s, want EXCEEDS_CAPACITY", item.RoomVariant.Code, item.UnavailableReason)
			}
		}
	}

	// Case 3: Missing inventory (middle night missing or len(avail) < nights)
	hMissing := NewRouter(Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		InvStore: &mockInvStore{
			avail: []inventory.Availability{
				{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), AvailableRooms: 5},
				// 2026-10-11 is missing!
			},
		},
		RateSvc: &mockRates{
			quotes: []rates.Quote{{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), RateMinor: 500_000}},
		},
	})
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1", nil)
	recMissing := httptest.NewRecorder()
	hMissing.ServeHTTP(recMissing, reqMissing)
	var resMissing struct {
		AvailableCount int                `json:"available_count"`
		Results        []SearchResultItem `json:"results"`
	}
	_ = newTestDecoder(recMissing.Body).Decode(&resMissing)
	if resMissing.AvailableCount != 0 {
		t.Errorf("available_count = %d, want 0 on missing inventory", resMissing.AvailableCount)
	}
	if len(resMissing.Results) > 0 && resMissing.Results[0].UnavailableReason != "MISSING_INVENTORY" {
		t.Errorf("reason = %s, want MISSING_INVENTORY", resMissing.Results[0].UnavailableReason)
	}

	// Case 4: Sold out (minAvail == 0)
	hSoldOut := NewRouter(Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		InvStore: &mockInvStore{
			avail: []inventory.Availability{
				{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), AvailableRooms: 0},
				{Date: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), AvailableRooms: 5},
			},
		},
		RateSvc: &mockRates{
			quotes: []rates.Quote{
				{Date: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), RateMinor: 500_000},
				{Date: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), RateMinor: 500_000},
			},
		},
	})
	reqSold := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1", nil)
	recSold := httptest.NewRecorder()
	hSoldOut.ServeHTTP(recSold, reqSold)
	var resSold struct {
		Results []SearchResultItem `json:"results"`
	}
	_ = newTestDecoder(recSold.Body).Decode(&resSold)
	if len(resSold.Results) > 0 && resSold.Results[0].UnavailableReason != "SOLD_OUT" {
		t.Errorf("reason = %s, want SOLD_OUT", resSold.Results[0].UnavailableReason)
	}

	// Case 5: Insufficient rooms (minAvail < requested rooms)
	reqInsuff := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=6", nil)
	recInsuff := httptest.NewRecorder()
	h.ServeHTTP(recInsuff, reqInsuff)
	var resInsuff struct {
		Results []SearchResultItem `json:"results"`
	}
	_ = newTestDecoder(recInsuff.Body).Decode(&resInsuff)
	// We have 5 rooms available, but requested 6
	for _, item := range resInsuff.Results {
		if !item.Available && item.UnavailableReason != "EXCEEDS_CAPACITY" && item.UnavailableReason != "INSUFFICIENT_ROOMS" {
			t.Errorf("unexpected reason %s for room %s", item.UnavailableReason, item.RoomVariant.Code)
		}
	}

	// Case 6: Inventory store internal error (500)
	hInvErr := NewRouter(Deps{
		Enforcer: auth.DefaultTestEnforcer(),
		InvStore: &mockInvStore{err: errors.New("connection reset by peer")},
		RateSvc:  &mockRates{},
	})
	reqInvErr := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1", nil)
	recInvErr := httptest.NewRecorder()
	hInvErr.ServeHTTP(recInvErr, reqInvErr)
	if recInvErr.Code != http.StatusInternalServerError {
		t.Errorf("inv error status = %d, want 500", recInvErr.Code)
	}
}

func TestCreateBooking_NewValidationErrors(t *testing.T) {
	h, _ := setupTestRouter()
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	in10d := today.AddDate(0, 0, 10).Format("2006-01-02")
	in12d := today.AddDate(0, 0, 12).Format("2006-01-02")
	in45d := today.AddDate(0, 0, 45).Format("2006-01-02")
	past := today.AddDate(0, 0, -5).Format("2006-01-02")
	horizonExceedIn := today.AddDate(1, 0, 10).Format("2006-01-02")
	horizonExceedOut := today.AddDate(1, 0, 12).Format("2006-01-02")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "exceeds max stay 30 nights",
			body:       fmt.Sprintf(`{"room_type_id":"std","check_in":"%s","check_out":"%s","num_rooms":1,"num_guests":1,"guest_name":"Budi","guest_email":"budi@example.com"}`, in10d, in45d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "EXCEEDS_MAX_LOS",
		},
		{
			name:       "check_in date in past",
			body:       fmt.Sprintf(`{"room_type_id":"std","check_in":"%s","check_out":"%s","num_rooms":1,"num_guests":1,"guest_name":"Budi","guest_email":"budi@example.com"}`, past, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "PAST_DATE",
		},
		{
			name:       "check_out exceeds 365 days horizon",
			body:       fmt.Sprintf(`{"room_type_id":"std","check_in":"%s","check_out":"%s","num_rooms":1,"num_guests":1,"guest_name":"Budi","guest_email":"budi@example.com"}`, horizonExceedIn, horizonExceedOut),
			wantStatus: http.StatusBadRequest,
			wantCode:   "EXCEEDS_HORIZON",
		},
		{
			name:       "invalid guest info empty name",
			body:       fmt.Sprintf(`{"room_type_id":"std","check_in":"%s","check_out":"%s","num_rooms":1,"num_guests":1,"guest_name":"","guest_email":"budi@example.com"}`, in10d, in12d),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			var prob ProblemDetails
			_ = newTestDecoder(rec.Body).Decode(&prob)
			if prob.Code != tt.wantCode {
				t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
			}
		})
	}
}

func TestCatalogRoomCRUD(t *testing.T) {
	h, _ := setupTestRouter()

	// 1. GET /api/v1/catalog/rooms/{id}
	t.Run("GET by ID happy path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/rooms/sup-king", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var v catalog.RoomVariant
		if err := newTestDecoder(rec.Body).Decode(&v); err != nil {
			t.Fatalf("decode err = %v", err)
		}
		if v.Code != "sup-king" {
			t.Errorf("got code = %s, want sup-king", v.Code)
		}
	})

	t.Run("GET by ID not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/rooms/non-existent", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("GET by ID store error", func(t *testing.T) {
		hErr := NewRouter(Deps{
			Enforcer:     auth.DefaultTestEnforcer(),
			CatalogStore: &mockCatalogStore{err: errors.New("db error")},
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/rooms/sup-king", nil)
		rec := httptest.NewRecorder()
		hErr.ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})

	// 2. POST /api/v1/catalog/rooms
	postCases := []struct {
		name       string
		role       string
		body       string
		storeErr   error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "guest forbidden",
			role:       "guest",
			body:       `{"code":"test","name":"Test","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "receptionist forbidden",
			role:       "receptionist",
			body:       `{"code":"test","name":"Test","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "bad json payload",
			role:       "revenue_mgr",
			body:       `{bad-json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_PAYLOAD",
		},
		{
			name:       "invalid room data empty code",
			role:       "revenue_mgr",
			body:       `{"code":"","name":"Test","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_DATA",
		},
		{
			name:       "conflict room code",
			role:       "revenue_mgr",
			body:       `{"code":"sup-king","name":"Test","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT_ROOM_CODE",
		},
		{
			name:       "store internal error",
			role:       "revenue_mgr",
			body:       `{"code":"test","name":"Test","max_capacity":2,"base_price_minor":100}`,
			storeErr:   errors.New("db insert fail"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "CATALOG_ERROR",
		},
		{
			name:       "happy path create",
			role:       "revenue_mgr",
			body:       `{"code":"villa-pool","name":"Villa Pool Suite","max_capacity":4,"base_price_minor":2500000}`,
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range postCases {
		t.Run("POST "+tt.name, func(t *testing.T) {
			routerToUse, _ := setupTestRouter()
			if tt.storeErr != nil {
				routerToUse = NewRouter(Deps{
					Enforcer:     auth.DefaultTestEnforcer(),
					CatalogStore: &mockCatalogStore{err: tt.storeErr},
				})
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/rooms", bytes.NewBufferString(tt.body))
			if tt.role != "" && tt.role != "guest" {
				req.Header.Set("Authorization", "Bearer "+tt.role)
			}
			rec := httptest.NewRecorder()
			routerToUse.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var prob ProblemDetails
				_ = newTestDecoder(rec.Body).Decode(&prob)
				if prob.Code != tt.wantCode {
					t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
				}
			}
		})
	}

	// 3. PUT /api/v1/catalog/rooms/{id}
	putCases := []struct {
		name       string
		id         string
		role       string
		body       string
		storeErr   error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "guest forbidden",
			id:         "sup-king",
			role:       "guest",
			body:       `{"code":"sup-king","name":"Updated","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "bad json",
			id:         "sup-king",
			role:       "revenue_mgr",
			body:       `{bad-json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_PAYLOAD",
		},
		{
			name:       "not found",
			id:         "not-found",
			role:       "revenue_mgr",
			body:       `{"code":"code","name":"Name","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "ROOM_VARIANT_NOT_FOUND",
		},
		{
			name:       "invalid data empty code",
			id:         "sup-king",
			role:       "revenue_mgr",
			body:       `{"code":"","name":"Name","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_DATA",
		},
		{
			name:       "conflict code",
			id:         "sup-king",
			role:       "revenue_mgr",
			body:       `{"code":"dlx-king","name":"Name","max_capacity":2,"base_price_minor":100}`,
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT_ROOM_CODE",
		},
		{
			name:       "store error",
			id:         "sup-king",
			role:       "revenue_mgr",
			body:       `{"code":"code","name":"Name","max_capacity":2,"base_price_minor":100}`,
			storeErr:   errors.New("db error"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "CATALOG_ERROR",
		},
		{
			name:       "happy path update",
			id:         "sup-king",
			role:       "revenue_mgr",
			body:       `{"code":"sup-king","name":"Superior King Renovated","max_capacity":3,"base_price_minor":650000}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range putCases {
		t.Run("PUT "+tt.name, func(t *testing.T) {
			routerToUse, _ := setupTestRouter()
			if tt.storeErr != nil {
				routerToUse = NewRouter(Deps{
					Enforcer:     auth.DefaultTestEnforcer(),
					CatalogStore: &mockCatalogStore{err: tt.storeErr},
				})
			}
			req := httptest.NewRequest(http.MethodPut, "/api/v1/catalog/rooms/"+tt.id, bytes.NewBufferString(tt.body))
			if tt.role != "" && tt.role != "guest" {
				req.Header.Set("Authorization", "Bearer "+tt.role)
			}
			rec := httptest.NewRecorder()
			routerToUse.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var prob ProblemDetails
				_ = newTestDecoder(rec.Body).Decode(&prob)
				if prob.Code != tt.wantCode {
					t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
				}
			}
		})
	}

	// 4. DELETE /api/v1/catalog/rooms/{id}
	delCases := []struct {
		name        string
		id          string
		role        string
		storeErr    error
		customStore catalog.Store
		wantStatus  int
		wantCode    string
	}{
		{
			name:       "guest forbidden",
			id:         "sup-king",
			role:       "guest",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "revenue_mgr forbidden to delete (only gm_admin)",
			id:         "sup-king",
			role:       "revenue_mgr",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "gm_admin not found",
			id:         "not-found",
			role:       "gm_admin",
			wantStatus: http.StatusNotFound,
			wantCode:   "ROOM_VARIANT_NOT_FOUND",
		},
		{
			name:        "gm_admin in-use variant conflict",
			id:          "in-use",
			role:        "gm_admin",
			customStore: &mockCatalogStore{},
			wantStatus:  http.StatusConflict,
			wantCode:    "CANNOT_DELETE_ACTIVE_VARIANT",
		},
		{
			name:       "gm_admin store error",
			id:         "sup-king",
			role:       "gm_admin",
			storeErr:   errors.New("db error"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "CATALOG_ERROR",
		},
		{
			name:       "gm_admin happy path delete",
			id:         "sup-king",
			role:       "gm_admin",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range delCases {
		t.Run("DELETE "+tt.name, func(t *testing.T) {
			routerToUse, _ := setupTestRouter()
			if tt.customStore != nil {
				routerToUse = NewRouter(Deps{
					Enforcer:     auth.DefaultTestEnforcer(),
					CatalogStore: tt.customStore,
				})
			} else if tt.storeErr != nil {
				routerToUse = NewRouter(Deps{
					Enforcer:     auth.DefaultTestEnforcer(),
					CatalogStore: &mockCatalogStore{err: tt.storeErr},
				})
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/rooms/"+tt.id, nil)
			if tt.role != "" && tt.role != "guest" {
				req.Header.Set("Authorization", "Bearer "+tt.role)
			}
			rec := httptest.NewRecorder()
			routerToUse.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var prob ProblemDetails
				_ = newTestDecoder(rec.Body).Decode(&prob)
				if prob.Code != tt.wantCode {
					t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestQuotes_TableDriven(t *testing.T) {
	router, _ := setupTestRouter()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		checkQuote func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "happy path room only",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"rate_plan_code": "room_only",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 1,
				"num_guests": 2
			}`,
			wantStatus: http.StatusOK,
			checkQuote: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var q rates.LockedQuote
				if err := newTestDecoder(rec.Body).Decode(&q); err != nil {
					t.Fatalf("failed to decode quote: %v", err)
				}
				if q.ID == "" {
					t.Errorf("expected non-empty quote ID")
				}
				if q.RatePlanCode != rates.RatePlanRoomOnly {
					t.Errorf("expected room_only, got %s", q.RatePlanCode)
				}
				if q.Pricing.RoomSubtotalMinor <= 0 {
					t.Errorf("expected room subtotal > 0, got %d", q.Pricing.RoomSubtotalMinor)
				}
				if q.Pricing.TaxMinor != (q.Pricing.RoomSubtotalMinor*10)/100 {
					t.Errorf("expected tax = 10%% of subtotal, got %d", q.Pricing.TaxMinor)
				}
			},
		},
		{
			name: "happy path bed and breakfast with OCTOBREAK promo",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"rate_plan_code": "bed_and_breakfast",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 1,
				"num_guests": 2,
				"promo_code": "OCTOBREAK"
			}`,
			wantStatus: http.StatusOK,
			checkQuote: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var q rates.LockedQuote
				if err := newTestDecoder(rec.Body).Decode(&q); err != nil {
					t.Fatalf("failed to decode quote: %v", err)
				}
				if q.CancellationCode != rates.PolicyNonRefundable {
					t.Errorf("promo quote should be non_refundable, got %s", q.CancellationCode)
				}
				if q.Pricing.DiscountMinor <= 0 {
					t.Errorf("expected discount > 0, got %d", q.Pricing.DiscountMinor)
				}
				if q.Pricing.BreakfastChargeMinor != 400_000 {
					t.Errorf("expected breakfast charge 400_000, got %d", q.Pricing.BreakfastChargeMinor)
				}
			},
		},
		{
			name: "invalid rate plan",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"rate_plan_code": "all_inclusive_vip",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12"
			}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_RATE_PLAN",
		},
		{
			name: "invalid promo code",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"promo_code": "INVALID_PROMO_XYZ"
			}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_PROMO_CODE",
		},
		{
			name: "unknown room type",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-999999999999",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12"
			}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "ROOM_NOT_FOUND",
		},
		{
			name: "invalid date format",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "invalid-date",
				"check_out": "2026-10-12"
			}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name: "check out before check in",
			body: `{
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-12",
				"check_out": "2026-10-10"
			}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name:       "bad json body",
			body:       `{ invalid json `,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var prob ProblemDetails
				_ = newTestDecoder(rec.Body).Decode(&prob)
				if prob.Code != tt.wantCode {
					t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
				}
			}
			if tt.checkQuote != nil {
				tt.checkQuote(t, rec)
			}
		})
	}
}

func TestCreateBooking_WithQuoteAndConsent(t *testing.T) {
	router, _ := setupTestRouter()

	// Buat quote valid untuk pengujian
	qReq := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString(`{
		"room_type_id": "01900000-0000-7000-8000-000000000001",
		"rate_plan_code": "bed_and_breakfast",
		"check_in": "2026-10-10",
		"check_out": "2026-10-12",
		"num_rooms": 1,
		"num_guests": 2
	}`))
	qReq.Header.Set("Content-Type", "application/json")
	qRec := httptest.NewRecorder()
	router.ServeHTTP(qRec, qReq)
	if qRec.Code != http.StatusOK {
		t.Fatalf("setup quote failed: %s", qRec.Body.String())
	}
	var validQuote rates.LockedQuote
	_ = newTestDecoder(qRec.Body).Decode(&validQuote)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name: "fails when consent not provided",
			body: fmt.Sprintf(`{
				"quote_id": "%s",
				"terms_accepted": false,
				"privacy_accepted": true,
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 1,
				"num_guests": 2,
				"guest_name": "Budi Santoso",
				"guest_email": "budi@example.com"
			}`, validQuote.ID),
			wantStatus: http.StatusBadRequest,
			wantCode:   "CONSENT_REQUIRED",
		},
		{
			name: "fails when quote expired or not found",
			body: `{
				"quote_id": "01900000-0000-7000-8000-nonexistent",
				"terms_accepted": true,
				"privacy_accepted": true,
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 1,
				"num_guests": 2,
				"guest_name": "Budi Santoso",
				"guest_email": "budi@example.com"
			}`,
			wantStatus: http.StatusGone,
			wantCode:   "QUOTE_EXPIRED",
		},
		{
			name: "fails when booking params mismatch quote",
			body: fmt.Sprintf(`{
				"quote_id": "%s",
				"terms_accepted": true,
				"privacy_accepted": true,
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 2,
				"num_guests": 2,
				"guest_name": "Budi Santoso",
				"guest_email": "budi@example.com"
			}`, validQuote.ID),
			wantStatus: http.StatusBadRequest,
			wantCode:   "QUOTE_MISMATCH",
		},
		{
			name: "happy path booking with quote and consent",
			body: fmt.Sprintf(`{
				"quote_id": "%s",
				"terms_accepted": true,
				"privacy_accepted": true,
				"room_type_id": "01900000-0000-7000-8000-000000000001",
				"check_in": "2026-10-10",
				"check_out": "2026-10-12",
				"num_rooms": 1,
				"num_guests": 2,
				"guest_name": "Budi Santoso",
				"guest_email": "budi@example.com"
			}`, validQuote.ID),
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("%s: status = %d, want %d (body: %s)", tt.name, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var prob ProblemDetails
				_ = newTestDecoder(rec.Body).Decode(&prob)
				if prob.Code != tt.wantCode {
					t.Errorf("%s: code = %s, want %s", tt.name, prob.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestCancelBooking_PolicyEnforcement(t *testing.T) {
	router, txMock := setupTestRouter()
	now := time.Now()

	// Skenario 1: Confirmed booking non_refundable
	txMock.booking = booking.Booking{
		ID:                 "bk-non-ref",
		Status:             booking.StatusConfirmed,
		RoomTypeID:         "std",
		CheckIn:            now.Add(72 * time.Hour),
		CheckOut:           now.Add(96 * time.Hour),
		NumRooms:           1,
		GuestToken:         "gst_non_ref",
		CancellationPolicy: rates.PolicyNonRefundable,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-non-ref/cancel", nil)
	req.Header.Set("X-Guest-Token", "gst_non_ref")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for non_refundable cancel, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var prob ProblemDetails
	_ = newTestDecoder(rec.Body).Decode(&prob)
	if prob.Code != "NON_REFUNDABLE_BOOKING" {
		t.Errorf("expected NON_REFUNDABLE_BOOKING, got %s", prob.Code)
	}

	// Skenario 2: Confirmed booking flexible_48h tapi sudah melewati deadline
	txMock.booking = booking.Booking{
		ID:                 "bk-past-deadline",
		Status:             booking.StatusConfirmed,
		RoomTypeID:         "std",
		CheckIn:            now.Add(24 * time.Hour), // Kurang dari 48 jam
		CheckOut:           now.Add(48 * time.Hour),
		NumRooms:           1,
		GuestToken:         "gst_past_deadline",
		CancellationPolicy: rates.PolicyFlexible48h,
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-past-deadline/cancel", nil)
	req2.Header.Set("X-Guest-Token", "gst_past_deadline")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for deadline exceeded cancel, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}
	var prob2 ProblemDetails
	_ = newTestDecoder(rec2.Body).Decode(&prob2)
	if prob2.Code != "CANCELLATION_DEADLINE_EXCEEDED" {
		t.Errorf("expected CANCELLATION_DEADLINE_EXCEEDED, got %s", prob2.Code)
	}

	// Skenario 3: Pending booking dengan non_refundable policy -> hold release diperbolehkan
	txMock.booking = booking.Booking{
		ID:                 "bk-pending-non-ref",
		Status:             booking.StatusPending,
		RoomTypeID:         "std",
		CheckIn:            now.Add(72 * time.Hour),
		CheckOut:           now.Add(96 * time.Hour),
		NumRooms:           1,
		GuestToken:         "gst_pending_release",
		CancellationPolicy: rates.PolicyNonRefundable,
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-pending-non-ref/cancel", nil)
	req3.Header.Set("X-Guest-Token", "gst_pending_release")
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 for pending hold release, got %d (body: %s)", rec3.Code, rec3.Body.String())
	}
}

// mustQuoteID membuat quote terkunci via API sungguhan; create booking wajib quote (BE-R06).
func mustQuoteID(t *testing.T, router http.Handler, roomTypeID string, rooms, guests int) string {
	t.Helper()
	body := fmt.Sprintf(`{"room_type_id":%q,"check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":%d,"num_guests":%d,"rate_plan":"room_only"}`, roomTypeID, rooms, guests)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup quote failed (%d): %s", rec.Code, rec.Body.String())
	}
	var q rates.LockedQuote
	_ = newTestDecoder(rec.Body).Decode(&q)
	return q.ID
}

func TestBatchD_IdempotencyAndGuestProfile(t *testing.T) {
	router, txMock := setupTestRouter()

	t.Run("Idempotency-Key IETF flow", func(t *testing.T) {
		validPayload := fmt.Sprintf(`{
			"quote_id": %q,
			"terms_accepted": true,
			"privacy_accepted": true,
			"room_type_id": "std",
			"check_in": "2026-10-10",
			"check_out": "2026-10-12",
			"num_rooms": 1,
			"num_guests": 1,
			"guest_name": "Budi Santoso",
			"guest_email": "budi@example.com",
			"guest_phone": "+6281234567890",
			"estimated_arrival_time": "14:00",
			"special_requests": "Quiet room"
		}`, mustQuoteID(t, router, "std", 1, 1))

		// 1. Initial request with Idempotency-Key
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(validPayload))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Idempotency-Key", "ik-sample-12345")
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created on first call, got %d (body: %s)", rec1.Code, rec1.Body.String())
		}
		var resp1 map[string]any
		_ = newTestDecoder(rec1.Body).Decode(&resp1)
		if resp1["expires_at"] == nil {
			t.Errorf("expected expires_at in response, got nil")
		}
		if resp1["server_time"] == nil {
			t.Errorf("expected server_time in response, got nil")
		}

		// 2. Replayed request with SAME key and SAME payload
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(validPayload))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", "ik-sample-12345")
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created on replay, got %d", rec2.Code)
		}
		if rec2.Header().Get("Idempotency-Replayed") != "true" {
			t.Errorf("expected Idempotency-Replayed header = true, got %q", rec2.Header().Get("Idempotency-Replayed"))
		}

		// 3. Conflicting request with SAME key but DIFFERENT payload
		diffPayload := `{
			"room_type_id": "std",
			"check_in": "2026-10-10",
			"check_out": "2026-10-12",
			"num_rooms": 2,
			"num_guests": 2,
			"guest_name": "Different Guest",
			"guest_email": "diff@example.com"
		}`
		req3 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(diffPayload))
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("Idempotency-Key", "ik-sample-12345")
		rec3 := httptest.NewRecorder()
		router.ServeHTTP(rec3, req3)

		if rec3.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for mismatched payload, got %d", rec3.Code)
		}
		var prob ProblemDetails
		_ = newTestDecoder(rec3.Body).Decode(&prob)
		if prob.Code != "IDEMPOTENCY_CONFLICT" {
			t.Errorf("expected IDEMPOTENCY_CONFLICT, got %s", prob.Code)
		}
	})

	t.Run("Guest profile validations table test", func(t *testing.T) {
		tests := []struct {
			name       string
			phone      string
			arrival    string
			requests   string
			wantStatus int
			wantCode   string
		}{
			{
				name:       "invalid phone domestic",
				phone:      "081234567890",
				arrival:    "14:00",
				requests:   "None",
				wantStatus: http.StatusBadRequest,
				wantCode:   "INVALID_PHONE",
			},
			{
				name:       "invalid arrival time hour",
				phone:      "+6281234567890",
				arrival:    "25:00",
				requests:   "None",
				wantStatus: http.StatusBadRequest,
				wantCode:   "INVALID_ARRIVAL_TIME",
			},
			{
				name:       "special requests exceeds 500 chars",
				phone:      "+6281234567890",
				arrival:    "14:00",
				requests:   strings.Repeat("X", 501),
				wantStatus: http.StatusBadRequest,
				wantCode:   "SPECIAL_REQUEST_TOO_LONG",
			},
			{
				name:       "valid profile values",
				phone:      "+6281234567890",
				arrival:    "14:00",
				requests:   "Late check-in requested",
				wantStatus: http.StatusCreated,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				body, _ := json.Marshal(map[string]any{
					"quote_id":               mustQuoteID(t, router, "std", 1, 1),
					"terms_accepted":         true,
					"privacy_accepted":       true,
					"room_type_id":           "std",
					"check_in":               "2026-10-10",
					"check_out":              "2026-10-12",
					"num_rooms":              1,
					"num_guests":             1,
					"guest_name":             "Budi Santoso",
					"guest_email":            "budi@example.com",
					"guest_phone":            tc.phone,
					"estimated_arrival_time": tc.arrival,
					"special_requests":       tc.requests,
				})
				req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(string(body)))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Fatalf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
				}
				if tc.wantCode != "" {
					var prob ProblemDetails
					_ = newTestDecoder(rec.Body).Decode(&prob)
					if prob.Code != tc.wantCode {
						t.Errorf("expected code %s, got %s", tc.wantCode, prob.Code)
					}
				}
			})
		}
	})

	t.Run("UU PDP Privacy on GET booking", func(t *testing.T) {
		txMock.booking = booking.Booking{
			ID:                   "bk-privacy-test",
			Status:               booking.StatusConfirmed,
			RoomTypeID:           "std",
			CheckIn:              time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
			CheckOut:             time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
			NumRooms:             1,
			GuestName:            "Siti Rahma",
			GuestEmail:           "siti@example.com",
			GuestPhone:           "+6281987654321",
			EstimatedArrivalTime: "15:00",
			SpecialRequests:      "Quiet corner",
			GuestToken:           "gst_siti_secure",
		}

		// 1. Unauthenticated / guest without token -> PublicDTO returned
		req1 := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-privacy-test", nil)
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec1.Code)
		}
		var publicDTO map[string]any
		_ = newTestDecoder(rec1.Body).Decode(&publicDTO)

		// Sensitive PII must NOT be present
		if _, exists := publicDTO["guest_phone"]; exists {
			t.Errorf("guest_phone MUST NOT be returned in PublicDTO")
		}
		if _, exists := publicDTO["guest_email"]; exists {
			t.Errorf("guest_email MUST NOT be returned in PublicDTO")
		}
		if _, exists := publicDTO["guest_name"]; exists {
			t.Errorf("guest_name MUST NOT be returned in PublicDTO")
		}
		if _, exists := publicDTO["guest_token"]; exists {
			t.Errorf("guest_token MUST NOT be returned in PublicDTO")
		}
		// BE-R02: Free-text dan detail kedatangan disamarkan pada PublicDTO
		if val, exists := publicDTO["estimated_arrival_time"]; exists && val != "" {
			t.Errorf("estimated_arrival_time MUST NOT be leaked in PublicDTO (BE-R02), got %v", val)
		}
		if val, exists := publicDTO["special_requests"]; exists && val != "" {
			t.Errorf("special_requests MUST NOT be leaked in PublicDTO (BE-R02), got %v", val)
		}

		// 2. Guest with valid X-Guest-Token -> full Booking returned
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-privacy-test", nil)
		req2.Header.Set("X-Guest-Token", "gst_siti_secure")
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200 with token, got %d", rec2.Code)
		}
		var fullDTO map[string]any
		_ = newTestDecoder(rec2.Body).Decode(&fullDTO)
		if fullDTO["guest_phone"] != "+6281987654321" {
			t.Errorf("expected guest_phone = +6281987654321, got %v", fullDTO["guest_phone"])
		}
		if fullDTO["guest_email"] != "siti@example.com" {
			t.Errorf("expected guest_email = siti@example.com, got %v", fullDTO["guest_email"])
		}
		if fullDTO["estimated_arrival_time"] != "15:00" {
			t.Errorf("expected estimated_arrival_time = 15:00 for authenticated guest, got %v", fullDTO["estimated_arrival_time"])
		}
		if fullDTO["special_requests"] != "Quiet corner" {
			t.Errorf("expected special_requests = Quiet corner for authenticated guest, got %v", fullDTO["special_requests"])
		}
	})
}

func TestTransportModernization_JSONv2_And_Validator(t *testing.T) {
	router, _ := setupTestRouter()

	t.Run("JSON v2 rejects malformed JSON with 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString(`{"invalid-json`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("JSON v2 rejects duplicate keys with 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString(`{"room_type_id":"01900000-0000-7000-8000-000000000001","room_type_id":"01900000-0000-7000-8000-000000000002"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for duplicate keys, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Validator v10 table tests across DTOs", func(t *testing.T) {
		tests := []struct {
			name        string
			method      string
			path        string
			body        string
			authBearer  string
			wantCode    string
			wantStatus  int
		}{
			{
				name:       "search rejects rooms > 8 with INVALID_ROOM_COUNT",
				method:     http.MethodGet,
				path:       "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&rooms=9",
				wantCode:   "INVALID_ROOM_COUNT",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "search rejects adults < 1 with INVALID_GUEST_COUNT",
				method:     http.MethodGet,
				path:       "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=0",
				wantCode:   "INVALID_GUEST_COUNT",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "search rejects children < 0 with INVALID_GUEST_COUNT",
				method:     http.MethodGet,
				path:       "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&children=-2",
				wantCode:   "INVALID_GUEST_COUNT",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "search rejects child age > 17 with INVALID_CHILD_AGE",
				method:     http.MethodGet,
				path:       "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&child_ages=18",
				wantCode:   "INVALID_CHILD_AGE",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "quote rejects missing room_type_id with INVALID_DATE_FORMAT",
				method:     http.MethodPost,
				path:       "/api/v1/quotes",
				body:       `{"check_in":"2026-10-10","check_out":"2026-10-12"}`,
				wantCode:   "INVALID_DATE_FORMAT",
				wantStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
				if tc.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				if tc.authBearer != "" {
					req.Header.Set("Authorization", "Bearer "+tc.authBearer)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Fatalf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
				}
				if tc.wantCode != "" {
					var prob ProblemDetails
					_ = newTestDecoder(rec.Body).Decode(&prob)
					if prob.Code != tc.wantCode {
						t.Errorf("expected code %s, got %s", tc.wantCode, prob.Code)
					}
				}
			})
		}
	})
}

