package script

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		{"p", "guest", "/api/v1/catalog/rooms", "GET"},
		{"p", "guest", "/api/v1/catalog/rooms/:id", "GET"},
		{"p", "guest", "/api/v1/search", "GET"},
		{"p", "guest", "/api/v1/quotes", "POST"},
		{"p", "guest", "/api/v1/bookings", "POST"},
		{"p", "guest", "/api/v1/bookings/:id", "GET"},
		{"p", "guest", "/api/v1/bookings/:id/cancel", "POST"},
		{"p", "guest", "/fake-pay/:ref", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/no-show", "POST"},
		{"p", "housekeeping", "/api/v1/rooms/housekeeping", "GET"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms", "POST"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms/:id", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
		{"g", "revenue_mgr", "guest"},
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

	rateEngine := rates.NewEngine(map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000,
	}, 1.25)
	quoteStore := rateEngine.QuoteStore()
	bkSvc.SetQuoteStore(quoteStore)

	handler := api.NewRouter(api.Deps{
		BookingSvc:    bkSvc,
		InvStore:      inv,
		RateSvc:       ratesSvc,
		RateEngine:    rateEngine,
		QuoteStore:    quoteStore,
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

	// 2A. Public Catalog Room Discovery (BE-G01)
	t.Run("E2E-02A: Public catalog room discovery (7 sellable variants, 95 rooms)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/catalog/rooms")
		if err != nil {
			t.Fatalf("catalog request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("catalog status = %d, want 200", res.StatusCode)
		}
		var catalogResp struct {
			Total int `json:"total"`
			Rooms []struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"rooms"`
		}
		if err := json.NewDecoder(res.Body).Decode(&catalogResp); err != nil {
			t.Fatalf("catalog decode failed: %v", err)
		}
		if catalogResp.Total != 7 || len(catalogResp.Rooms) != 7 {
			t.Errorf("catalog total = %d, want 7 sellable variants", catalogResp.Total)
		}
	})

	// 2B. Multi-night Cross-Variant Search Engine (BE-G02, BE-G03)
	t.Run("E2E-02B: Multi-night cross-variant search", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("search status = %d, want 200", res.StatusCode)
		}
		var searchResp struct {
			TotalVariants  int `json:"total_variants"`
			AvailableCount int `json:"available_count"`
			Results        []struct {
				Available       bool  `json:"available"`
				AvailableRooms  int   `json:"available_rooms"`
				TotalPriceMinor int64 `json:"total_price_minor"`
			} `json:"results"`
		}
		if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil {
			t.Fatalf("search decode failed: %v", err)
		}
		if searchResp.TotalVariants != 7 {
			t.Errorf("total_variants = %d, want 7", searchResp.TotalVariants)
		}
		if searchResp.AvailableCount == 0 {
			t.Errorf("available_count = 0, want > 0")
		}
	})

	// 2C. Search Validation Reject Exceeding Stay (BE-G03: LOS > 30 nights)
	t.Run("E2E-02C: Search validation reject stay > 30 nights (400 Bad Request)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-11-20&adults=2&rooms=1")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("search validation status = %d, want 400", res.StatusCode)
		}
	})

	// 2D. Search Validation Reject Invalid Child Age (BE-G03: Age > 17)
	t.Run("E2E-02D: Search validation reject child age > 17 (400 Bad Request)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&child_ages=19")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("search child age status = %d, want 400", res.StatusCode)
		}
	})

	var createdVariantID string

	// 2E. Revenue Manager creates a new room variant (POST /api/v1/catalog/rooms)
	t.Run("E2E-02E: Revenue Manager creates room variant (201 Created)", func(t *testing.T) {
		payload := []byte(`{
			"code": "villa-garden",
			"name": "Garden Villa",
			"family_name": "Villa",
			"bed_type": "1 King Bed",
			"room_size_sqm": 85,
			"max_capacity": 4,
			"max_adults": 2,
			"max_children": 2,
			"base_price_minor": 2500000,
			"description": "Private villa with lush tropical garden view.",
			"amenities": ["Private Pool", "Free Wi-Fi"],
			"photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
		}`)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/catalog/rooms", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("create variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create variant status = %d, want 201", res.StatusCode)
		}
		var created struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		}
		if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
			t.Fatalf("decode created variant failed: %v", err)
		}
		if created.ID == "" || created.Code != "villa-garden" {
			t.Fatalf("unexpected created variant: %+v", created)
		}
		createdVariantID = created.ID
	})

	// 2F. Revenue Manager updates room variant (PUT /api/v1/catalog/rooms/:id)
	t.Run("E2E-02F: Revenue Manager updates room variant (200 OK)", func(t *testing.T) {
		payload := []byte(`{
			"code": "villa-garden",
			"name": "Garden Villa Deluxe",
			"family_name": "Villa",
			"bed_type": "1 King Bed",
			"room_size_sqm": 85,
			"max_capacity": 4,
			"max_adults": 2,
			"max_children": 2,
			"base_price_minor": 2750000,
			"description": "Private villa with lush tropical garden view and floating breakfast.",
			"amenities": ["Private Pool", "Free Wi-Fi", "Floating Breakfast"],
			"photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
		}`)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("update variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("update variant status = %d, want 200", res.StatusCode)
		}
		var updated struct {
			Name           string `json:"name"`
			BasePriceMinor int64  `json:"base_price_minor"`
		}
		if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
			t.Fatalf("decode updated variant failed: %v", err)
		}
		if updated.Name != "Garden Villa Deluxe" || updated.BasePriceMinor != 2750000 {
			t.Errorf("unexpected updated variant: %+v", updated)
		}
	})

	// 2G. Public Guest views single room variant (GET /api/v1/catalog/rooms/:id)
	t.Run("E2E-02G: Public Guest views single room variant (200 OK)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/catalog/rooms/" + createdVariantID)
		if err != nil {
			t.Fatalf("get single variant failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("get single variant status = %d, want 200", res.StatusCode)
		}
		var v struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			BasePriceMinor int64  `json:"base_price_minor"`
		}
		if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
			t.Fatalf("decode single variant failed: %v", err)
		}
		if v.ID != createdVariantID || v.BasePriceMinor != 2750000 {
			t.Errorf("expected updated price 2750000, got %+v", v)
		}
	})

	// 2H. Catalog RBAC Negative Tests (Guest cannot POST/PUT/DELETE, Revenue Mgr cannot DELETE)
	t.Run("E2E-02H: Catalog RBAC Negative Tests (403 Forbidden)", func(t *testing.T) {
		// Guest cannot POST
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/catalog/rooms", bytes.NewReader([]byte(`{"code":"hack"}`)))
		req.Header.Set("Content-Type", "application/json")
		res, _ := client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest post catalog status = %d, want 403", res.StatusCode)
		}

		// Guest cannot PUT
		req, _ = http.NewRequest(http.MethodPut, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, bytes.NewReader([]byte(`{"name":"hack"}`)))
		req.Header.Set("Content-Type", "application/json")
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest put catalog status = %d, want 403", res.StatusCode)
		}

		// Guest cannot DELETE
		req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest delete catalog status = %d, want 403", res.StatusCode)
		}

		// Revenue Manager cannot DELETE
		req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("revenue_mgr delete catalog status = %d, want 403", res.StatusCode)
		}
	})

	// 2I. GM Admin deletes room variant (DELETE /api/v1/catalog/rooms/:id)
	t.Run("E2E-02I: GM Admin deletes room variant (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		req.Header.Set("Authorization", "Bearer gm_admin")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("gm_admin delete variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("delete variant status = %d, want 200", res.StatusCode)
		}

		// Verify subsequent GET returns 404 Not Found
		res, err = client.Get(srv.URL + "/api/v1/catalog/rooms/" + createdVariantID)
		if err != nil {
			t.Fatalf("subsequent get variant failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("deleted variant status = %d, want 404", res.StatusCode)
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

	checkIn := "2026-10-10"
	checkOut := "2026-10-12"
	var lockedPromoQuote rates.LockedQuote

	// 14. BE-G04, BE-G05, BE-G06: Guest requests locked quote with BB and OCTOBREAK promo
	t.Run("E2E-14: Guest requests locked quote with BB and OCTOBREAK promo", func(t *testing.T) {
		payload := `{
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"rate_plan_code": "bed_and_breakfast",
			"check_in": "` + checkIn + `",
			"check_out": "` + checkOut + `",
			"num_rooms": 1,
			"num_guests": 2,
			"promo_code": "OCTOBREAK"
		}`
		res, err := client.Post(srv.URL+"/api/v1/quotes", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&lockedPromoQuote); err != nil {
			t.Fatalf("decode quote failed: %v", err)
		}
		if lockedPromoQuote.CancellationCode != rates.PolicyNonRefundable {
			t.Errorf("expected non_refundable policy for promo quote, got %s", lockedPromoQuote.CancellationCode)
		}
		if lockedPromoQuote.Pricing.DiscountMinor <= 0 {
			t.Errorf("expected discount > 0, got %d", lockedPromoQuote.Pricing.DiscountMinor)
		}
		if lockedPromoQuote.Pricing.BreakfastChargeMinor != 400_000 {
			t.Errorf("expected breakfast charge 400_000, got %d", lockedPromoQuote.Pricing.BreakfastChargeMinor)
		}
	})

	// 15. BE-G19: Guest attempts to book with quote but omits consent (400 CONSENT_REQUIRED)
	t.Run("E2E-15: Guest attempts to book without consent (400 CONSENT_REQUIRED)", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"quote_id": "%s",
			"terms_accepted": false,
			"privacy_accepted": true,
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "%s",
			"check_out": "%s",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Siti Rahma",
			"guest_email": "siti@example.com"
		}`, lockedPromoQuote.ID, checkIn, checkOut)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "CONSENT_REQUIRED" {
			t.Errorf("expected code CONSENT_REQUIRED, got %s", pd.Code)
		}
	})

	var promoBookingID string
	var promoGuestToken string

	// 16. BE-G06, BE-G19: Guest creates booking with locked quote snapshot and consent (201 Created)
	t.Run("E2E-16: Guest creates booking with locked quote snapshot and consent", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"quote_id": "%s",
			"terms_accepted": true,
			"privacy_accepted": true,
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "%s",
			"check_out": "%s",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Siti Rahma",
			"guest_email": "siti@example.com"
		}`, lockedPromoQuote.ID, checkIn, checkOut)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		bMap := body["booking"].(map[string]any)
		promoBookingID = bMap["id"].(string)
		promoGuestToken = body["guest_access_token"].(string)
		if int64(bMap["total_price_minor"].(float64)) != lockedPromoQuote.Pricing.TotalPriceMinor {
			t.Errorf("locked price mismatch: got %v, want %d", bMap["total_price_minor"], lockedPromoQuote.Pricing.TotalPriceMinor)
		}
	})

	// 17. BE-G08: Confirmed promo booking cannot be cancelled by guest (409 NON_REFUNDABLE_BOOKING)
	t.Run("E2E-17: Confirmed promo booking cannot be cancelled by guest (409 Conflict)", func(t *testing.T) {
		// Konfirmasi booking promo terlebih dahulu
		payRes, err := client.Post(srv.URL+"/fake-pay/ref-promo?booking_id="+promoBookingID, "application/json", nil)
		if err != nil {
			t.Fatalf("fake-pay failed: %v", err)
		}
		if payRes.StatusCode != http.StatusOK {
			t.Fatalf("fake-pay status = %d, want 200", payRes.StatusCode)
		}

		// Update status mock booking di tx agar confirmed
		tx.booking.Status = booking.StatusConfirmed

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/"+promoBookingID+"/cancel", nil)
		req.Header.Set("X-Guest-Token", promoGuestToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("cancel request failed: %v", err)
		}
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409 Conflict", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "NON_REFUNDABLE_BOOKING" {
			t.Errorf("expected code NON_REFUNDABLE_BOOKING, got %s", pd.Code)
		}
	})
}
