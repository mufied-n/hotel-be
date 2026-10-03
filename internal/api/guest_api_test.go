package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/guest"
)

type mockGuestService struct {
	requestChallengeFunc    func(ctx context.Context, email string) (int, error)
	verifyChallengeFunc     func(ctx context.Context, email, code string) (string, *guest.GuestSession, error)
	validateSessionFunc     func(ctx context.Context, rawToken string) (*guest.GuestSession, error)
	revokeSessionFunc       func(ctx context.Context, rawToken string) error
	getProfileFunc          func(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error)
	listBookingsFunc        func(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error)
	getBookingDetailFunc    func(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error)
	getBookingReceiptFunc   func(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error)
	generateCalendarICSFunc func(receipt *guest.ReceiptDTO) ([]byte, error)
}

func (m *mockGuestService) RequestChallenge(ctx context.Context, email string) (int, error) {
	if m.requestChallengeFunc != nil {
		return m.requestChallengeFunc(ctx, email)
	}
	return 60, nil
}

func (m *mockGuestService) VerifyChallenge(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
	if m.verifyChallengeFunc != nil {
		return m.verifyChallengeFunc(ctx, email, code)
	}
	return "gst_sess_mock_token_123", &guest.GuestSession{
		ID:         "sess_01",
		GuestEmail: email,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

func (m *mockGuestService) ValidateSession(ctx context.Context, rawToken string) (*guest.GuestSession, error) {
	if m.validateSessionFunc != nil {
		return m.validateSessionFunc(ctx, rawToken)
	}
	if rawToken == "gst_sess_valid" {
		return &guest.GuestSession{
			ID:         "sess_valid",
			GuestEmail: "tamu@example.com",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}, nil
	}
	return nil, guest.ErrSessionNotFound
}

func (m *mockGuestService) RevokeSession(ctx context.Context, rawToken string) error {
	if m.revokeSessionFunc != nil {
		return m.revokeSessionFunc(ctx, rawToken)
	}
	return nil
}

func (m *mockGuestService) GetSessionProfile(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error) {
	if m.getProfileFunc != nil {
		return m.getProfileFunc(ctx, session)
	}
	return &guest.ProfileView{
		Email:               session.GuestEmail,
		ActiveBookingsCount: 1,
		LastActiveAt:        time.Now(),
		ExpiresAt:           session.ExpiresAt,
	}, nil
}

func (m *mockGuestService) ListBookings(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error) {
	if m.listBookingsFunc != nil {
		return m.listBookingsFunc(ctx, email, status, limit)
	}
	return []guest.BookingSummary{
		{
			ID:              "bk_001",
			RoomTypeName:    "Deluxe Premier",
			Status:          "confirmed",
			TotalPriceMinor: 1500000,
			Currency:        "IDR",
		},
	}, nil
}

func (m *mockGuestService) GetBookingDetail(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error) {
	if m.getBookingDetailFunc != nil {
		return m.getBookingDetailFunc(ctx, email, bookingID)
	}
	if bookingID == "bk_001" && email == "tamu@example.com" {
		return &guest.BookingDetail{
			ID:              "bk_001",
			GuestEmail:      email,
			GuestName:       "Budi Santoso",
			RoomTypeName:    "Deluxe Premier",
			Status:          "confirmed",
			TotalPriceMinor: 1500000,
			Currency:        "IDR",
			AllowedActions: guest.AllowedActions{
				CanCancel:          true,
				CanDownloadReceipt: true,
			},
		}, nil
	}
	return nil, guest.ErrBookingNotFound
}

func (m *mockGuestService) GetBookingReceipt(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error) {
	if m.getBookingReceiptFunc != nil {
		return m.getBookingReceiptFunc(ctx, email, bookingID)
	}
	if bookingID == "bk_001" && email == "tamu@example.com" {
		return &guest.ReceiptDTO{
			InvoiceNumber:    "INV/PKU/202610/BK001",
			BookingID:        "bk_001",
			BookingReference: "PKU-20261003-BK001",
			Status:           "confirmed",
			HotelInfo: guest.HotelInfo{
				Name: "Pulang ke Uttara",
			},
			StayDetails: guest.StayDetails{
				CheckInDate:  "2026-10-10",
				CheckOutDate: "2026-10-12",
				TotalNights:  2,
			},
			GuestDetails: guest.GuestDetails{
				Name:  "Budi Santoso",
				Email: email,
			},
			RoomItem: guest.RoomItemReceipt{
				RoomTypeName: "Deluxe Premier",
				NumRooms:     1,
			},
		}, nil
	}
	if bookingID == "bk_pending" {
		return nil, guest.ErrReceiptNotAvailable
	}
	return nil, guest.ErrBookingNotFound
}

func (m *mockGuestService) GenerateCalendarICS(receipt *guest.ReceiptDTO) ([]byte, error) {
	if m.generateCalendarICSFunc != nil {
		return m.generateCalendarICSFunc(receipt)
	}
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nSUMMARY:Pulang ke Uttara\r\nEND:VCALENDAR\r\n"), nil
}

func TestGuestAuth_HTTP_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		headers        map[string]string
		setupMock      func(m *mockGuestService)
		withGuestSvc   bool
		expectedStatus int
	}{
		{
			name:           "Challenge request with valid email returns 200 OK",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/challenge",
			body:           `{"email": "tamu@example.com"}`,
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Challenge request with invalid email returns 400 Bad Request",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/challenge",
			body:   `{"email": "invalid"}`,
			setupMock: func(m *mockGuestService) {
				m.requestChallengeFunc = func(ctx context.Context, email string) (int, error) {
					return 0, guest.ErrInvalidEmail
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Challenge request with cooldown returns 429 Too Many Requests",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/challenge",
			body:   `{"email": "tamu@example.com"}`,
			setupMock: func(m *mockGuestService) {
				m.requestChallengeFunc = func(ctx context.Context, email string) (int, error) {
					return 0, guest.ErrRateLimited
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "Challenge request when service unconfigured returns 501",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/challenge",
			body:           `{"email": "tamu@example.com"}`,
			withGuestSvc:   false,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "Verify with valid code returns 200 OK and sets cookie",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/verify",
			body:           `{"email": "tamu@example.com", "code": "123456"}`,
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Verify with wrong/expired code returns 401 Unauthorized",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/verify",
			body:   `{"email": "tamu@example.com", "code": "000000"}`,
			setupMock: func(m *mockGuestService) {
				m.verifyChallengeFunc = func(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
					return "", nil, guest.ErrInvalidOrExpiredCode
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Verify with max attempts exceeded returns 403 Forbidden",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/verify",
			body:   `{"email": "tamu@example.com", "code": "000000"}`,
			setupMock: func(m *mockGuestService) {
				m.verifyChallengeFunc = func(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
					return "", nil, guest.ErrMaxAttemptsExceeded
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Get me without session returns 401 Unauthorized",
			method:         http.MethodGet,
			path:           "/api/v1/auth/guest/me",
			withGuestSvc:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Get me with valid Bearer session returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/auth/guest/me",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Get me with X-Guest-Session header returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/auth/guest/me",
			headers: map[string]string{
				"X-Guest-Session": "gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Logout with active session returns 200 OK",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/logout",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Logout when RevokeSession fails returns 503 Service Unavailable",
			method: http.MethodPost,
			path:   "/api/v1/auth/guest/logout",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			setupMock: func(m *mockGuestService) {
				m.revokeSessionFunc = func(ctx context.Context, rawToken string) error {
					return errors.New("db disconnect")
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:   "List bookings with valid session returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings?status=upcoming",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Get booking detail owned by guest returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_001",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "IDOR Defense: Accessing another guest's booking returns 404 Not Found",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_other_person",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Get booking receipt with valid session returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_001/receipt",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Get booking receipt without session returns 401 Unauthorized",
			method:         http.MethodGet,
			path:           "/api/v1/guest/bookings/bk_001/receipt",
			withGuestSvc:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Get booking receipt for pending booking returns 400 Bad Request",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_pending/receipt",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "IDOR Defense: Accessing other guest receipt returns 404 Not Found",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_other_receipt/receipt",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Get booking calendar.ics with valid session returns 200 OK",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_001/calendar.ics",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Get booking calendar.ics without session returns 401 Unauthorized",
			method:         http.MethodGet,
			path:           "/api/v1/guest/bookings/bk_001/calendar.ics",
			withGuestSvc:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Get booking calendar.ics for pending booking returns 400 Bad Request",
			method: http.MethodGet,
			path:   "/api/v1/guest/bookings/bk_pending/calendar.ics",
			headers: map[string]string{
				"Authorization": "Bearer gst_sess_valid",
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockGuestService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}

			deps := Deps{
				IsDevelopment: true,
			}
			if tc.withGuestSvc {
				deps.GuestSvc = mockSvc
			}

			router := NewRouter(deps)

			var reqBody *bytes.Reader
			if tc.body != "" {
				reqBody = bytes.NewReader([]byte(tc.body))
			} else {
				reqBody = bytes.NewReader([]byte{})
			}

			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tc.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGuestAPI_SecurityHeadersAndCookies(t *testing.T) {
	mockSvc := &mockGuestService{}

	// 1. In dev mode without TLS/HTTPS: Secure is false, HttpOnly is true, SameSite is Lax
	depsDev := Deps{
		IsDevelopment: true,
		GuestSvc:      mockSvc,
	}
	routerDev := NewRouter(depsDev)

	reqVerify := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest/verify", strings.NewReader(`{"email":"tamu@example.com","code":"123456"}`))
	reqVerify.Header.Set("Content-Type", "application/json")
	recVerify := httptest.NewRecorder()
	routerDev.ServeHTTP(recVerify, reqVerify)

	if recVerify.Code != http.StatusOK {
		t.Fatalf("verify failed: %d", recVerify.Code)
	}
	if cc := recVerify.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}
	cookieHeader := recVerify.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "HttpOnly") {
		t.Errorf("expected HttpOnly in cookie, got %q", cookieHeader)
	}
	if strings.Contains(cookieHeader, "Secure") {
		t.Errorf("expected non-Secure cookie in plain dev mode, got %q", cookieHeader)
	}

	// 2. With X-Forwarded-Proto: https in dev mode: Secure is true
	reqHttps := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest/verify", strings.NewReader(`{"email":"tamu@example.com","code":"123456"}`))
	reqHttps.Header.Set("Content-Type", "application/json")
	reqHttps.Header.Set("X-Forwarded-Proto", "https")
	recHttps := httptest.NewRecorder()
	routerDev.ServeHTTP(recHttps, reqHttps)

	cookieHttps := recHttps.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHttps, "Secure") {
		t.Errorf("expected Secure flag when X-Forwarded-Proto: https, got %q", cookieHttps)
	}

	// 3. In production mode (!IsDevelopment): Secure is true
	depsProd := Deps{
		IsDevelopment: false,
		GuestSvc:      mockSvc,
	}
	routerProd := NewRouter(depsProd)

	reqProd := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest/verify", strings.NewReader(`{"email":"tamu@example.com","code":"123456"}`))
	reqProd.Header.Set("Content-Type", "application/json")
	recProd := httptest.NewRecorder()
	routerProd.ServeHTTP(recProd, reqProd)

	cookieProd := recProd.Header().Get("Set-Cookie")
	if !strings.Contains(cookieProd, "Secure") {
		t.Errorf("expected Secure flag in production mode, got %q", cookieProd)
	}

	// 4. Logout in production mode sets Secure expired cookie
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest/logout", nil)
	reqLogout.Header.Set("Authorization", "Bearer gst_sess_valid")
	recLogout := httptest.NewRecorder()
	routerProd.ServeHTTP(recLogout, reqLogout)

	if recLogout.Code != http.StatusOK {
		t.Fatalf("logout failed: %d", recLogout.Code)
	}
	if cc := recLogout.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("expected Cache-Control: no-store on logout, got %q", cc)
	}
	cookieLogout := recLogout.Header().Get("Set-Cookie")
	if !strings.Contains(cookieLogout, "Max-Age=0") && !strings.Contains(cookieLogout, "Max-Age=-1") {
		t.Errorf("expected cookie cleared on logout, got %q", cookieLogout)
	}
	if !strings.Contains(cookieLogout, "Secure") {
		t.Errorf("expected Secure flag on cleared cookie in prod, got %q", cookieLogout)
	}
}
