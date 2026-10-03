package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/guest"
)

type mockGuestService struct {
	requestChallengeFunc func(ctx context.Context, email string) (int, error)
	verifyChallengeFunc  func(ctx context.Context, email, code string) (string, *guest.GuestSession, error)
	validateSessionFunc  func(ctx context.Context, rawToken string) (*guest.GuestSession, error)
	revokeSessionFunc    func(ctx context.Context, rawToken string) error
	getProfileFunc       func(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error)
	listBookingsFunc     func(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error)
	getBookingDetailFunc func(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error)
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
			name:           "Challenge request with invalid email returns 400 Bad Request",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/challenge",
			body:           `{"email": "invalid"}`,
			setupMock: func(m *mockGuestService) {
				m.requestChallengeFunc = func(ctx context.Context, email string) (int, error) {
					return 0, guest.ErrInvalidEmail
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Challenge request with cooldown returns 429 Too Many Requests",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/challenge",
			body:           `{"email": "tamu@example.com"}`,
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
			name:           "Verify with wrong/expired code returns 401 Unauthorized",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/verify",
			body:           `{"email": "tamu@example.com", "code": "000000"}`,
			setupMock: func(m *mockGuestService) {
				m.verifyChallengeFunc = func(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
					return "", nil, guest.ErrInvalidOrExpiredCode
				}
			},
			withGuestSvc:   true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Verify with max attempts exceeded returns 403 Forbidden",
			method:         http.MethodPost,
			path:           "/api/v1/auth/guest/verify",
			body:           `{"email": "tamu@example.com", "code": "000000"}`,
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
