package script

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
)

type e2eTxMock struct {
	booking booking.Booking
	rooms   []string
}

func (m *e2eTxMock) LockAndDecrement(_ context.Context, _ string, _, _ time.Time, _ int) error {
	return nil
}
func (m *e2eTxMock) Increment(_ context.Context, _ string, _, _ time.Time, _ int) error {
	return nil
}
func (m *e2eTxMock) InsertBookingWithHold(_ context.Context, b *booking.Booking, _ []rates.Quote, _ time.Time) error {
	b.ID = "bk-e2e-001"
	b.Status = booking.StatusPending
	m.booking = *b
	return nil
}
func (m *e2eTxMock) GetForUpdate(_ context.Context, id string) (booking.Booking, error) {
	return m.booking, nil
}
func (m *e2eTxMock) UpdateStatus(_ context.Context, _ string, to booking.Status) error {
	m.booking.Status = to
	return nil
}
func (m *e2eTxMock) PickAndAssignRooms(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	return m.rooms, nil
}
func (m *e2eTxMock) GetRoomAssignments(_ context.Context, _ string) ([]string, error) {
	return m.rooms, nil
}
func (m *e2eTxMock) PublishTx(_ context.Context, _ string, _ []byte) error { return nil }

type e2eTxRunner struct{ tx *e2eTxMock }

func (r *e2eTxRunner) InTx(_ context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	return fn(r.tx, r.tx)
}

type e2eReaderMock struct{ tx *e2eTxMock }

func (m *e2eReaderMock) Get(_ context.Context, _ string) (booking.Booking, error) {
	return m.tx.booking, nil
}

type e2ePayMock struct{}

func (m *e2ePayMock) CreateCharge(_ context.Context, _ booking.Booking, _ int64, _ string) (booking.ChargeResult, error) {
	return booking.ChargeResult{PaymentURL: "http://pay.hotel.test/charge/123", Reference: "ref-e2e-001"}, nil
}

type e2eNotifierMock struct{}

func (m *e2eNotifierMock) SendBookingConfirmed(_ context.Context, _ booking.Booking) error {
	return nil
}

func setupE2ETestServer(t *testing.T) (*httptest.Server, *e2eTxMock) {
	t.Helper()

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "guest", "/api/v1/bookings", "POST"},
		{"p", "guest", "/api/v1/bookings/:id", "GET"},
		{"p", "guest", "/api/v1/bookings/:id/cancel", "POST"},
		{"p", "guest", "/fake-pay/:ref", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/no-show", "POST"},
		{"p", "housekeeping", "/api/v1/rooms/housekeeping", "GET"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	now := time.Now()
	tx := &e2eTxMock{
		booking: booking.Booking{
			ID:              "bk-e2e-001",
			Status:          booking.StatusPending,
			RoomTypeID:      "01900000-0000-7000-8000-000000000001",
			CheckIn:         now,
			CheckOut:        now.Add(48 * time.Hour),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Budi Santoso",
			GuestEmail:      "budi@example.com",
			GuestToken:      "gst_e2e_secret_token_123",
			TotalPriceMinor: 1_100_000,
		},
		rooms: []string{"301"},
	}

	runner := &e2eTxRunner{tx: tx}
	inv := &mockInventoryStore{
		avail: []inventory.Availability{
			{Date: now, TotalRooms: 20, AvailableRooms: 10},
			{Date: now.Add(24 * time.Hour), TotalRooms: 20, AvailableRooms: 10},
		},
	}
	ratesSvc := &mockRateProvider{
		quotes: []rates.Quote{
			{Date: now, RateMinor: 550_000},
			{Date: now.Add(24 * time.Hour), RateMinor: 550_000},
		},
	}

	bkSvc := booking.NewService(
		runner,
		inv,
		ratesSvc,
		&e2ePayMock{},
		&e2eNotifierMock{},
		&e2eReaderMock{tx: tx},
		30*time.Minute,
		nil,
	)

	handler := api.NewRouter(api.Deps{
		BookingSvc:    bkSvc,
		InvStore:      inv,
		RateSvc:       ratesSvc,
		Enforcer:      enforcer,
		IsDevelopment: true,
		ReadyCheck:    func(ctx context.Context) error { return nil },
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			_ = bkSvc.Confirm(r.Context(), "bk-e2e-001")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"confirmed"}`))
		},
	})

	srv := httptest.NewServer(handler)
	return srv, tx
}

type mockInventoryStore struct {
	avail []inventory.Availability
}

func (m *mockInventoryStore) GetByDate(_ context.Context, _ string, _, _ time.Time) ([]inventory.Availability, error) {
	return m.avail, nil
}

type mockRateProvider struct {
	quotes []rates.Quote
}

func (m *mockRateProvider) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	return m.quotes, nil
}

func TestEndToEndHotelBookingRBACLifecycle(t *testing.T) {
	srv, tx := setupE2ETestServer(t)
	defer srv.Close()

	client := srv.Client()
	var createdGuestToken string

	// 1. Healthz probe
	t.Run("E2E-01: Health check", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("healthz status = %d, want 200", res.StatusCode)
		}
	})

	// 2. Public Availability Search
	t.Run("E2E-02: Public search availability", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/availability?room_type_id=01900000-0000-7000-8000-000000000001&check_in=2026-10-10&check_out=2026-10-12")
		if err != nil {
			t.Fatalf("availability request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("availability status = %d, want 200", res.StatusCode)
		}
	})

	// 3. Public Create Booking
	t.Run("E2E-03: Public create booking hold", func(t *testing.T) {
		payload := []byte(`{
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "2026-10-10",
			"check_out": "2026-10-12",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Budi Santoso",
			"guest_email": "budi@example.com"
		}`)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("booking request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Errorf("create booking status = %d, want 201", res.StatusCode)
		}
		var resp struct {
			GuestAccessToken string `json:"guest_access_token"`
		}
		_ = json.NewDecoder(res.Body).Decode(&resp)
		createdGuestToken = resp.GuestAccessToken
	})

	// 4. RBAC Negative Test: Public Guest CANNOT check-in
	t.Run("E2E-04: Guest cannot check-in (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("check-in request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest check-in status = %d, want 403 Forbidden", res.StatusCode)
		}
	})

	// 5. Payment Confirmation Simulation
	t.Run("E2E-05: Payment confirmation", func(t *testing.T) {
		res, err := client.Post(srv.URL+"/fake-pay/ref-e2e", "application/json", nil)
		if err != nil {
			t.Fatalf("payment request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("payment status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusConfirmed {
			t.Errorf("booking status after payment = %s, want confirmed", tx.booking.Status)
		}
	})

	// 6. Receptionist Check-In
	t.Run("E2E-06: Receptionist check-in (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		req.Header.Set("X-User-Role", "receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("receptionist check-in request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("receptionist check-in status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusCheckedIn {
			t.Errorf("booking status = %s, want checked_in", tx.booking.Status)
		}
	})

	// 7. Housekeeping cannot check-out
	t.Run("E2E-07: Housekeeping cannot check-out (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		req.Header.Set("X-User-Role", "housekeeping")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("housekeeping check-out request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("housekeeping check-out status = %d, want 403 Forbidden", res.StatusCode)
		}
	})

	// 8. Receptionist Check-Out via Bearer Token
	t.Run("E2E-08: Receptionist check-out via Bearer token (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("receptionist check-out request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("receptionist check-out status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusCheckedOut {
			t.Errorf("booking status = %s, want checked_out", tx.booking.Status)
		}
	})

	// 9. GM Admin Wildcard Inspection
	t.Run("E2E-09: GM Admin inspect booking (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		req.Header.Set("X-User-Role", "gm_admin")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("gm_admin request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("gm_admin status = %d, want 200", res.StatusCode)
		}

		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["id"] != "bk-e2e-001" {
			t.Errorf("expected booking ID bk-e2e-001, got %v", body["id"])
		}
	})

	// 10. BE-G13: Public guest receives masked PublicDTO (no PII leakage)
	t.Run("E2E-10: Public guest receives masked PublicDTO (no PII)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if _, exists := body["guest_name"]; exists {
			t.Errorf("PII leaked! guest_name found: %v", body["guest_name"])
		}
		if _, exists := body["guest_email"]; exists {
			t.Errorf("PII leaked! guest_email found: %v", body["guest_email"])
		}
		if _, exists := body["guest_token"]; exists {
			t.Errorf("PII leaked! guest_token found: %v", body["guest_token"])
		}
		if body["id"] != "bk-e2e-001" {
			t.Errorf("expected id bk-e2e-001, got %v", body["id"])
		}
	})

	// 11. BE-G13: Guest with valid X-Guest-Token receives full PII
	t.Run("E2E-11: Guest with X-Guest-Token receives full PII", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		req.Header.Set("X-Guest-Token", createdGuestToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["guest_name"] != "Budi Santoso" {
			t.Errorf("expected guest_name 'Budi Santoso', got %v", body["guest_name"])
		}
		if body["guest_email"] != "budi@example.com" {
			t.Errorf("expected guest_email 'budi@example.com', got %v", body["guest_email"])
		}
	})

	// 12. BE-G13: Guest with invalid token cannot cancel booking (403 Forbidden)
	t.Run("E2E-12: Guest with invalid token cannot cancel booking (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/cancel", nil)
		req.Header.Set("X-Guest-Token", "invalid_token_xyz")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("cancel status = %d, want 403 Forbidden", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "FORBIDDEN_OWNERSHIP" {
			t.Errorf("expected error code FORBIDDEN_OWNERSHIP, got %s", pd.Code)
		}
	})

	// 13. BE-G10: Production mode gates /fake-pay (404 Not Found)
	t.Run("E2E-13: Production mode gates /fake-pay (404 Not Found)", func(t *testing.T) {
		prodHandler := api.NewRouter(api.Deps{
			Enforcer:      auth.DefaultTestEnforcer(),
			IsDevelopment: false,
			FakePay: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		})
		prodSrv := httptest.NewServer(prodHandler)
		defer prodSrv.Close()

		res, err := client.Post(prodSrv.URL+"/fake-pay/ref-e2e", "application/json", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("production fake-pay status = %d, want 404", res.StatusCode)
		}
	})
}
