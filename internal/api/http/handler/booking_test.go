package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

type mockBookingTx struct {
	booking   booking.Booking
	getErr    error
	statusErr error
}

func (m *mockBookingTx) LockAndDecrement(context.Context, string, time.Time, time.Time, int) error {
	return nil
}
func (m *mockBookingTx) Increment(context.Context, string, time.Time, time.Time, int) error {
	return nil
}
func (m *mockBookingTx) InsertBookingWithHold(context.Context, *booking.Booking, []rates.Quote, time.Time) error {
	return nil
}
func (m *mockBookingTx) GetForUpdate(context.Context, string) (booking.Booking, error) {
	if m.getErr != nil {
		return booking.Booking{}, m.getErr
	}
	return m.booking, nil
}
func (m *mockBookingTx) UpdateStatus(_ context.Context, _ string, to booking.Status) error {
	if m.statusErr != nil {
		return m.statusErr
	}
	m.booking.Status = to
	return nil
}
func (m *mockBookingTx) PickAndAssignRooms(context.Context, string, string, time.Time, time.Time, int) ([]string, error) {
	return []string{"101"}, nil
}
func (m *mockBookingTx) GetRoomAssignments(context.Context, string) ([]string, error) {
	return []string{"101"}, nil
}
func (m *mockBookingTx) PublishTx(context.Context, string, []byte) error { return nil }

type mockTxRunner struct{ tx *mockBookingTx }

func (r *mockTxRunner) InTx(_ context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	return fn(r.tx, r.tx)
}

type mockBookingReader struct{ tx *mockBookingTx }

func (r *mockBookingReader) Get(_ context.Context, _ string) (booking.Booking, error) {
	if r.tx.getErr != nil {
		return booking.Booking{}, r.tx.getErr
	}
	return r.tx.booking, nil
}

type mockPaymentGateway struct{}

func (mockPaymentGateway) CreateCharge(context.Context, booking.Booking, int64, string) (booking.ChargeResult, error) {
	return booking.ChargeResult{PaymentURL: "http://payment.url", Reference: "ref-test"}, nil
}

type mockNotifier struct{}

func (mockNotifier) SendBookingConfirmed(context.Context, booking.Booking) error { return nil }

func TestBookingHandlers_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	txMock := &mockBookingTx{
		booking: booking.Booking{
			ID:         "b1",
			RoomTypeID: "var-1",
			Status:     booking.StatusConfirmed,
			GuestName:  "Test Guest",
			GuestEmail: "guest@example.com",
			GuestToken: "gst_token",
			CheckIn:         time.Now().Add(-24 * time.Hour),
			CheckOut:        time.Now().Add(24 * time.Hour),
			NumRooms:        1,
			TotalPriceMinor: 500000,
			Currency:        "IDR",
		},
	}
	runner := &mockTxRunner{tx: txMock}
	reader := &mockBookingReader{tx: txMock}
	bkSvc := booking.NewService(runner, &mockInvStore{}, &mockRates{}, mockPaymentGateway{}, mockNotifier{}, reader, 30*time.Minute, nil)

	qs := rates.NewMemoryQuoteStore(15 * time.Minute)
	checkIn := time.Now().Add(24 * time.Hour).Truncate(24 * time.Hour)
	checkOut := time.Now().Add(48 * time.Hour).Truncate(24 * time.Hour)
	_ = qs.SaveQuote(context.Background(), rates.LockedQuote{
		ID:         "quote-123",
		RoomTypeID: "var-1",
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		NumRooms:   1,
		NumGuests:  2,
		NightlyRates: []rates.Quote{
			{Date: checkIn, RateMinor: 500000},
		},
		Pricing: rates.PricingBreakdown{
			TotalPriceMinor: 500000,
			Currency:        "IDR",
		},
		ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	bkSvc.SetQuoteStore(qs)

	xendit := payment.NewXendit("https://api.xendit.co", "test-sec", "valid-token", "http://localhost", nil)

	deps := Deps{
		BookingSvc:    bkSvc,
		XenditGateway: xendit,
		CatalogStore: &mockCatalogStore{
			variants: []catalog.RoomVariant{
				{ID: "var-1", MaxCapacity: 2},
			},
		},
	}

	r := gin.New()
	r.Use(middleware.IdentifySubject(nil))
	r.POST("/api/v1/bookings", CreateBooking(deps))
	r.GET("/api/v1/bookings/:id", GetBooking(deps))
	r.GET("/api/v1/bookings/:id/payment", GetBookingPayment(deps))
	r.POST("/api/v1/bookings/:id/cancel", CancelBooking(deps))
	r.POST("/api/v1/bookings/:id/check-in", CheckIn(deps))
	r.POST("/api/v1/bookings/:id/check-out", CheckOut(deps))
	r.POST("/api/v1/bookings/:id/no-show", NoShow(deps))
	r.POST("/api/v1/webhooks/xendit", XenditWebhook(deps))

	createBody := `{"quote_id":"quote-123","terms_accepted":true,"privacy_accepted":true,"room_type_id":"var-1","check_in":"` + checkIn.Format("2006-01-02") + `","check_out":"` + checkOut.Format("2006-01-02") + `","num_rooms":1,"num_guests":2,"guest_name":"Budi","guest_email":"budi@example.com"}`

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		headers    map[string]string
		status     booking.Status
		pending    bool
		statusErr  error
		wantStatus int
	}{
		{
			name:       "get booking found returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/bookings/b1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get booking payment guest returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/bookings/b1/payment",
			headers:    map[string]string{"X-Guest-Token": "gst_token"},
			pending:    true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "get booking payment unauthorized returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/bookings/b1/payment",
			headers:    map[string]string{"X-Guest-Token": "wrong_token"},
			pending:    true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "cancel booking returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/cancel",
			body:       `{"reason":"plans changed"}`,
			headers:    map[string]string{"X-Guest-Token": "gst_token"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "cancel booking guest wrong token returns 403",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/cancel",
			headers:    map[string]string{"X-Guest-Token": "wrong_token"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cancel booking illegal transition returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/cancel",
			headers:    map[string]string{"X-Guest-Token": "gst_token"},
			statusErr:  booking.ErrIllegalTransition,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "check-in returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/check-in",
			wantStatus: http.StatusOK,
		},
		{
			name:       "check-in illegal transition returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/check-in",
			statusErr:  booking.ErrIllegalTransition,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "check-out returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/check-out",
			status:     booking.StatusCheckedIn,
			wantStatus: http.StatusOK,
		},
		{
			name:       "check-out illegal transition returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/check-out",
			statusErr:  booking.ErrIllegalTransition,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "no-show returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/no-show",
			wantStatus: http.StatusOK,
		},
		{
			name:       "no-show illegal transition returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/no-show",
			statusErr:  booking.ErrIllegalTransition,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "create booking success returns 201",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       createBody,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "create booking room type not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{"quote_id":"quote-123","terms_accepted":true,"privacy_accepted":true,"room_type_id":"nonexistent","check_in":"` + checkIn.Format("2006-01-02") + `","check_out":"` + checkOut.Format("2006-01-02") + `","num_rooms":1,"num_guests":2,"guest_name":"Budi","guest_email":"budi@example.com"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "create booking exceeds variant capacity returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{"quote_id":"quote-123","terms_accepted":true,"privacy_accepted":true,"room_type_id":"var-1","check_in":"` + checkIn.Format("2006-01-02") + `","check_out":"` + checkOut.Format("2006-01-02") + `","num_rooms":1,"num_guests":5,"guest_name":"Budi","guest_email":"budi@example.com"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create booking invalid date returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{"room_type_id":"var-1","check_in":"invalid","check_out":"2026-10-17","num_rooms":1,"num_guests":2,"guest_name":"Budi","guest_email":"budi@example.com"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create booking guests less than rooms returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-15","check_out":"2026-10-17","num_rooms":2,"num_guests":1,"guest_name":"Budi","guest_email":"budi@example.com"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create booking invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create booking idempotency key too long returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings",
			body:       `{}`,
			headers:    map[string]string{"Idempotency-Key": "this-key-is-way-too-long-because-it-exceeds-sixty-four-characters-limit-1234567890"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "webhook invalid token returns 401",
			method:     http.MethodPost,
			url:        "/api/v1/webhooks/xendit",
			body:       `{"id":"inv-1","status":"PAID"}`,
			headers:    map[string]string{"x-callback-token": "wrong-token"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "webhook valid token and payload returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/webhooks/xendit",
			body:       `{"id":"inv-1","external_id":"b1","status":"PAID","amount":500000,"currency":"IDR"}`,
			headers:    map[string]string{"x-callback-token": "valid-token"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "webhook expired returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/webhooks/xendit",
			body:       `{"id":"inv-1","external_id":"b1","status":"EXPIRED"}`,
			headers:    map[string]string{"x-callback-token": "valid-token"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "webhook unhandled status returns 200 ignored",
			method:     http.MethodPost,
			url:        "/api/v1/webhooks/xendit",
			body:       `{"id":"inv-1","external_id":"b1","status":"PENDING"}`,
			headers:    map[string]string{"x-callback-token": "valid-token"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			txMock.statusErr = tc.statusErr
			if tc.pending {
				txMock.booking.Status = booking.StatusPending
				fut := time.Now().Add(time.Hour)
				txMock.booking.ExpiresAt = &fut
			} else if tc.status != "" {
				txMock.booking.Status = tc.status
			} else {
				txMock.booking.Status = booking.StatusConfirmed
			}
			var bodyReader *bytes.Buffer
			if tc.body != "" {
				bodyReader = bytes.NewBufferString(tc.body)
			} else {
				bodyReader = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.url, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}

	// Test not found cases
	txMock.getErr = booking.ErrNotFound
	recNotFound := httptest.NewRecorder()
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/nonexistent", nil)
	r.ServeHTTP(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not found booking, got %d", recNotFound.Code)
	}
}

func TestWebhook_NilGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/webhooks/xendit", XenditWebhook(Deps{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/xendit", bytes.NewBufferString("{}"))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestBookingHandlers_NilService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	deps := Deps{}
	r.GET("/api/v1/bookings/:id/payment", GetBookingPayment(deps))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/b1/payment", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}
