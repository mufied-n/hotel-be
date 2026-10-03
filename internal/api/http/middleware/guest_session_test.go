package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

type mockGuestService struct {
	guest.Service
	validateSessionFunc func(ctx context.Context, token string) (*guest.GuestSession, error)
}

func (m *mockGuestService) ValidateSession(ctx context.Context, token string) (*guest.GuestSession, error) {
	if m.validateSessionFunc != nil {
		return m.validateSessionFunc(ctx, token)
	}
	return nil, errors.New("not implemented")
}

func TestExtractGuestSessionToken_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		sessHeader string
		cookie     string
		wantToken  string
	}{
		{
			name:       "authorization bearer gst_sess_",
			authHeader: "Bearer gst_sess_12345",
			wantToken:  "gst_sess_12345",
		},
		{
			name:       "authorization bearer not gst_sess_",
			authHeader: "Bearer stf_token",
			wantToken:  "",
		},
		{
			name:       "x-guest-session header",
			sessHeader: "gst_sess_hdr_99",
			wantToken:  "gst_sess_hdr_99",
		},
		{
			name:      "cookie fallback",
			cookie:    "gst_sess_cookie_88",
			wantToken: "gst_sess_cookie_88",
		},
		{
			name:      "empty request",
			wantToken: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.sessHeader != "" {
				req.Header.Set("X-Guest-Session", tc.sessHeader)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "guest_session", Value: tc.cookie})
			}

			got := ExtractGuestSessionToken(req)
			if got != tc.wantToken {
				t.Errorf("ExtractGuestSessionToken() = %q, want %q", got, tc.wantToken)
			}
		})
	}
}

func TestRequireGuestSession_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		svc        guest.Service
		token      string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "service nil gives 501 GUEST_AUTH_NOT_CONFIGURED",
			svc:        nil,
			token:      "gst_sess_token",
			wantStatus: http.StatusNotImplemented,
			wantCode:   "GUEST_AUTH_NOT_CONFIGURED",
		},
		{
			name:       "missing token gives 401 UNAUTHORIZED",
			svc:        &mockGuestService{},
			token:      "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "invalid session gives 401 UNAUTHORIZED",
			svc: &mockGuestService{
				validateSessionFunc: func(ctx context.Context, token string) (*guest.GuestSession, error) {
					return nil, errors.New("invalid session")
				},
			},
			token:      "gst_sess_bad",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "valid session succeeds 200",
			svc: &mockGuestService{
				validateSessionFunc: func(ctx context.Context, token string) (*guest.GuestSession, error) {
					return &guest.GuestSession{
						GuestEmail: "guest@example.com",
						ExpiresAt:  time.Now().Add(time.Hour),
					}, nil
				},
			},
			token:      "gst_sess_good",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequireGuestSession(tc.svc))
			r.GET("/protected", func(c *gin.Context) {
				sess := GuestSessionFromContext(c.Request.Context())
				if sess == nil {
					c.String(http.StatusInternalServerError, "no session in context")
					return
				}
				c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.token != "" {
				req.Header.Set("X-Guest-Session", tc.token)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
