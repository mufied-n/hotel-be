package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/hotel-booking/internal/staffauth"
	"github.com/gin-gonic/gin"
)

type mockStaffVerifier struct {
	verifyFn func(ctx context.Context, token string) (staffauth.Principal, error)
}

func (m *mockStaffVerifier) VerifyStaffToken(ctx context.Context, token string) (staffauth.Principal, error) {
	if m.verifyFn != nil {
		return m.verifyFn(ctx, token)
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}

func TestResolveStaff_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		authHeader  string
		verifier    StaffVerifier
		wantOutcome staffOutcome
		wantRole    string
	}{
		{
			name:        "no authorization header",
			authHeader:  "",
			verifier:    nil,
			wantOutcome: outcomeGuest,
		},
		{
			name:        "bearer token without stf_ prefix with nil verifier",
			authHeader:  "Bearer some_guest_jwt",
			verifier:    nil,
			wantOutcome: outcomeGuest,
		},
		{
			name:        "stf_ token with nil verifier fail-closed",
			authHeader:  "Bearer stf_abcdef123456",
			verifier:    nil,
			wantOutcome: outcomeUnavailable,
		},
		{
			name:       "stf_ token verified successfully with recognized staff role",
			authHeader: "Bearer stf_valid_token",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{StaffID: "s1", Username: "reception1", Role: "receptionist"}, nil
				},
			},
			wantOutcome: outcomeStaff,
			wantRole:    "receptionist",
		},
		{
			name:       "stf_ token verified but with unrecognized role",
			authHeader: "Bearer stf_valid_token",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{StaffID: "s2", Username: "intruder", Role: "superuser"}, nil
				},
			},
			wantOutcome: outcomeUnauthorized,
		},
		{
			name:       "stf_ token unauthorized error",
			authHeader: "Bearer stf_expired_token",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{}, staffauth.ErrUnauthorized
				},
			},
			wantOutcome: outcomeUnauthorized,
		},
		{
			name:       "stf_ token verifier infrastructure error",
			authHeader: "Bearer stf_token",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{}, errors.New("db connection failure")
				},
			},
			wantOutcome: outcomeUnavailable,
		},
		{
			name:       "non-stf bearer token with err unauthorized stays guest",
			authHeader: "Bearer gst_token",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{}, staffauth.ErrUnauthorized
				},
			},
			wantOutcome: outcomeGuest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, outcome := resolveStaff(context.Background(), tt.authHeader, tt.verifier)
			if outcome != tt.wantOutcome {
				t.Errorf("outcome = %v, want %v", outcome, tt.wantOutcome)
			}
			if tt.wantRole != "" && p.Role != tt.wantRole {
				t.Errorf("principal.Role = %v, want %v", p.Role, tt.wantRole)
			}
		})
	}
}

func TestIdentifySubject_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		authHeader   string
		guestToken   string
		verifier     StaffVerifier
		wantCode     int
		wantRole     string
		wantSub      string
		wantGstToken string
	}{
		{
			name:         "anonymous guest with guest token",
			authHeader:   "",
			guestToken:   "gst_tok_123",
			verifier:     nil,
			wantCode:     http.StatusOK,
			wantRole:     "guest",
			wantSub:      "anonymous",
			wantGstToken: "gst_tok_123",
		},
		{
			name:       "valid staff member",
			authHeader: "Bearer stf_valid",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{StaffID: "s1", Username: "alice", Role: "housekeeping"}, nil
				},
			},
			wantCode: http.StatusOK,
			wantRole: "housekeeping",
			wantSub:  "staff:alice",
		},
		{
			name:       "invalid staff token gives 401",
			authHeader: "Bearer stf_invalid",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{}, staffauth.ErrUnauthorized
				},
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:       "infra error gives 503",
			authHeader: "Bearer stf_db_err",
			verifier: &mockStaffVerifier{
				verifyFn: func(_ context.Context, _ string) (staffauth.Principal, error) {
					return staffauth.Principal{}, errors.New("db error")
				},
			},
			wantCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(IdentifySubject(tt.verifier))
			var capturedCtx AuthContext
			var capturedGstToken string
			r.GET("/test", func(c *gin.Context) {
				capturedCtx = GetAuthContext(c.Request.Context())
				capturedGstToken = GetGuestToken(c.Request.Context())
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.guestToken != "" {
				req.Header.Set("X-Guest-Token", tt.guestToken)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantCode == http.StatusOK {
				if capturedCtx.Role != tt.wantRole {
					t.Errorf("role = %q, want %q", capturedCtx.Role, tt.wantRole)
				}
				if capturedCtx.Subject != tt.wantSub {
					t.Errorf("subject = %q, want %q", capturedCtx.Subject, tt.wantSub)
				}
				if capturedGstToken != tt.wantGstToken {
					t.Errorf("guestToken = %q, want %q", capturedGstToken, tt.wantGstToken)
				}
			}
		})
	}
}

func TestRequireStaffSession_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		role     string
		wantCode int
	}{
		{"guest role unauthorized", "guest", http.StatusUnauthorized},
		{"receptionist authorized", "receptionist", http.StatusOK},
		{"finance authorized", "finance", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), RoleKey, tt.role)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			r.Use(RequireStaffSession())
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
