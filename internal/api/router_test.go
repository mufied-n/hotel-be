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
		BookingSvc: bkSvc,
		InvStore:   inv,
		RateSvc:    ratesSvc,
		Enqueuer:   &workers.Enqueuer{}, // won't panic if client is nil unless called, or mock client
		ReadyCheck: func(_ context.Context) error { return nil },
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

	// Found
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/bk-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Not Found
	reqNF := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/not-found", nil)
	wNF := httptest.NewRecorder()
	h.ServeHTTP(wNF, reqNF)

	if wNF.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", wNF.Code)
	}
}

func TestCancelBooking(t *testing.T) {
	h, tx := setupTestRouter()
	tx.booking.Status = booking.StatusPending

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/cancel", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("cancel status = %d, want 200", w.Code)
	}
	if tx.booking.Status != booking.StatusCancelled {
		t.Errorf("status = %s, want cancelled", tx.booking.Status)
	}
}

func TestCheckIn(t *testing.T) {
	h, tx := setupTestRouter()
	tx.booking.Status = booking.StatusConfirmed

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-123/check-in", nil)
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
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("no-show status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if tx.booking.Status != booking.StatusNoShow {
		t.Errorf("status = %s, want no_show", tx.booking.Status)
	}
}
