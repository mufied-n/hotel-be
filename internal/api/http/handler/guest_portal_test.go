package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

type mockFullGuestService struct {
	guest.Service
}

func (m *mockFullGuestService) RequestChallenge(_ context.Context, email string) (int, error) {
	if email == "bad-email" {
		return 0, guest.ErrInvalidEmail
	}
	if email == "ratelimit@example.com" {
		return 0, guest.ErrRateLimited
	}
	return 60, nil
}

func (m *mockFullGuestService) VerifyChallenge(_ context.Context, email, code string) (string, *guest.GuestSession, error) {
	if email == "" || code == "" || code == "bad-code" {
		return "", nil, guest.ErrInvalidOrExpiredCode
	}
	if code == "max-attempts" {
		return "", nil, guest.ErrMaxAttemptsExceeded
	}
	return "gst_sess_12345", &guest.GuestSession{
		GuestEmail: email,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

func (m *mockFullGuestService) ValidateSession(_ context.Context, _ string) (*guest.GuestSession, error) {
	return &guest.GuestSession{
		GuestEmail: "guest@example.com",
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

func (m *mockFullGuestService) RevokeSession(context.Context, string) error { return nil }

func (m *mockFullGuestService) GetSessionProfile(_ context.Context, _ *guest.GuestSession) (*guest.ProfileView, error) {
	return &guest.ProfileView{Email: "guest@example.com", ActiveBookingsCount: 1}, nil
}

func (m *mockFullGuestService) ListBookings(_ context.Context, _, _ string, _ int) ([]guest.BookingSummary, error) {
	return []guest.BookingSummary{
		{ID: "b1", RoomTypeName: "Standard King"},
	}, nil
}

func (m *mockFullGuestService) GetBookingDetail(_ context.Context, _, id string) (*guest.BookingDetail, error) {
	if id == "not-found" {
		return nil, guest.ErrBookingNotFound
	}
	return &guest.BookingDetail{
		ID:        "b1",
		GuestName: "Test Guest",
	}, nil
}

func (m *mockFullGuestService) GetBookingReceipt(_ context.Context, _, id string) (*guest.ReceiptDTO, error) {
	if id == "not-found" {
		return nil, guest.ErrBookingNotFound
	}
	return &guest.ReceiptDTO{
		BookingID: "b1",
	}, nil
}

func (m *mockFullGuestService) GenerateCalendarICS(_ *guest.ReceiptDTO) ([]byte, error) {
	return []byte("BEGIN:VCALENDAR\nEND:VCALENDAR"), nil
}

func TestGuestPortalHandlers_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	future := time.Now().Add(time.Hour)
	txMock := &mockBookingTx{
		booking: booking.Booking{
			ID:              "b1",
			RoomTypeID:      "var-1",
			Status:          booking.StatusPending,
			GuestName:       "Test Guest",
			GuestEmail:      "guest@example.com",
			GuestToken:      "gst_token",
			CheckIn:         time.Now().Add(24 * time.Hour),
			CheckOut:        time.Now().Add(48 * time.Hour),
			NumRooms:        1,
			TotalPriceMinor: 500000,
			Currency:        "IDR",
			ExpiresAt:       &future,
		},
	}
	runner := &mockTxRunner{tx: txMock}
	reader := &mockBookingReader{tx: txMock}
	bkSvc := booking.NewService(runner, &mockInvStore{}, &mockRates{}, mockPaymentGateway{}, mockNotifier{}, reader, 30*time.Minute, nil)

	guestSvc := &mockFullGuestService{}
	deps := Deps{GuestSvc: guestSvc, BookingSvc: bkSvc}

	r := gin.New()
	r.Use(middleware.RequireGuestSession(guestSvc))
	r.POST("/api/v1/auth/guest/challenge", GuestChallenge(deps))
	r.POST("/api/v1/auth/guest/verify", GuestVerify(deps))
	r.GET("/api/v1/auth/guest/me", GuestMe(deps))
	r.POST("/api/v1/auth/guest/logout", GuestLogout(deps))
	r.GET("/api/v1/guest/bookings", GuestBookings(deps))
	r.GET("/api/v1/guest/bookings/:id", GuestBookingDetail(deps))
	r.GET("/api/v1/guest/bookings/:id/payment", GuestBookingPayment(deps))
	r.GET("/api/v1/guest/bookings/:id/receipt", GuestBookingReceipt(deps))
	r.GET("/api/v1/guest/bookings/:id/calendar.ics", GuestBookingCalendar(deps))

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		status     booking.Status
		expired    bool
		getErr     error
		wantStatus int
	}{
		{
			name:       "challenge invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/challenge",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "challenge invalid email returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/challenge",
			body:       `{"email":"bad-email"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "challenge valid email returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/challenge",
			body:       `{"email":"guest@example.com"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "challenge rate limited returns 429",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/challenge",
			body:       `{"email":"ratelimit@example.com"}`,
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "verify invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/verify",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "verify missing fields returns 401",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/verify",
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "verify invalid code returns 401",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/verify",
			body:       `{"email":"guest@example.com","code":"bad-code"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "verify max attempts returns 403",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/verify",
			body:       `{"email":"guest@example.com","code":"max-attempts"}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "verify valid code returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/verify",
			body:       `{"email":"guest@example.com","code":"123456"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest me returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/auth/guest/me",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest logout returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/auth/guest/logout",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest bookings list returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest booking detail found returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest booking detail not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/not-found",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "guest payment recovery success returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/payment",
			status:     booking.StatusPending,
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest payment recovery hold expired returns 410",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/payment",
			status:     booking.StatusPending,
			expired:    true,
			wantStatus: http.StatusGone,
		},
		{
			name:       "guest payment recovery not pending returns 409",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/payment",
			status:     booking.StatusConfirmed,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "guest payment recovery not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/payment",
			getErr:     booking.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "guest receipt returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/receipt",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest receipt not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/not-found/receipt",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "guest calendar returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/calendar.ics",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest calendar not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/not-found/calendar.ics",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status != "" {
				txMock.booking.Status = tc.status
			} else {
				txMock.booking.Status = booking.StatusPending
			}
			if tc.expired {
				past := time.Now().Add(-time.Hour)
				txMock.booking.ExpiresAt = &past
			} else {
				fut := time.Now().Add(time.Hour)
				txMock.booking.ExpiresAt = &fut
			}
			txMock.getErr = tc.getErr

			var bodyReader *bytes.Buffer
			if tc.body != "" {
				bodyReader = bytes.NewBufferString(tc.body)
			} else {
				bodyReader = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.url, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Guest-Session", "gst_sess_12345")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGuestPortal_NilService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := Deps{}
	r := gin.New()
	r.POST("/api/v1/auth/guest/challenge", GuestChallenge(deps))
	r.POST("/api/v1/auth/guest/verify", GuestVerify(deps))
	r.GET("/api/v1/auth/guest/me", GuestMe(deps))
	r.GET("/api/v1/guest/bookings", GuestBookings(deps))
	r.GET("/api/v1/guest/bookings/:id", GuestBookingDetail(deps))
	r.GET("/api/v1/guest/bookings/:id/payment", GuestBookingPayment(deps))
	r.GET("/api/v1/guest/bookings/:id/receipt", GuestBookingReceipt(deps))
	r.GET("/api/v1/guest/bookings/:id/calendar.ics", GuestBookingCalendar(deps))

	routes := []struct {
		method     string
		url        string
		wantStatus int
	}{
		{http.MethodPost, "/api/v1/auth/guest/challenge", http.StatusNotImplemented},
		{http.MethodPost, "/api/v1/auth/guest/verify", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/auth/guest/me", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/guest/bookings", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/guest/bookings/b1", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/guest/bookings/b1/payment", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/guest/bookings/b1/receipt", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/guest/bookings/b1/calendar.ics", http.StatusNotImplemented},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.url, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(rt.method, rt.url, bytes.NewBufferString(`{"email":"guest@example.com","code":"123456"}`))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.GuestSessionContextKey, &guest.GuestSession{GuestEmail: "guest@example.com"})
			req = req.WithContext(ctx)
			r.ServeHTTP(rec, req)
			if rec.Code != rt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, rt.wantStatus)
			}
		})
	}
}

func TestIsSecureCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	if !isSecureCookie(c, true) {
		t.Errorf("expected secure with X-Forwarded-Proto https")
	}

	c.Request.Header.Del("X-Forwarded-Proto")
	if isSecureCookie(c, true) {
		t.Errorf("expected not secure in dev without tls")
	}
	if !isSecureCookie(c, false) {
		t.Errorf("expected secure in prod")
	}
}
