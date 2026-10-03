package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/staffauth"
)

// StaffVerifier memverifikasi token sesi staf; dipakai middleware IdentifySubject.
type StaffVerifier interface {
	VerifyStaffToken(ctx context.Context, token string) (staffauth.Principal, error)
}

// StaffAuthService adalah kontrak yang dipakai handler login/logout/me (diimplementasi *staffauth.Service).
type StaffAuthService interface {
	StaffVerifier
	Login(ctx context.Context, username, password string) (string, time.Time, staffauth.Principal, error)
	Logout(ctx context.Context, token string) error
}

// staffVerifierOrNil mencegah interface non-nil berisi nilai nil lolos sebagai verifier valid.
func staffVerifierOrNil(s StaffAuthService) StaffVerifier {
	if s == nil {
		return nil
	}
	return s
}

// TestStaffVerifier adalah fixture HANYA untuk test: nama role (mis. "finance") dipetakan ke principal
// dengan username = role. Tidak boleh dipasang di cmd/server.
func TestStaffVerifier() StaffAuthService { return testStaffAuth{} }

type testStaffAuth struct{}

func (testStaffAuth) VerifyStaffToken(_ context.Context, token string) (staffauth.Principal, error) {
	if validStaffRoles[token] {
		return staffauth.Principal{StaffID: "test-" + token, Username: token, Role: token}, nil
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}

func (testStaffAuth) Login(context.Context, string, string) (string, time.Time, staffauth.Principal, error) {
	return "", time.Time{}, staffauth.Principal{}, staffauth.ErrInvalidCredentials
}

func (testStaffAuth) Logout(context.Context, string) error { return nil }

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// POST /api/v1/auth/staff/login
func handleStaffLogin(d Deps) gin.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<10))
		var in req
		if err != nil || json.Unmarshal(body, &in) != nil || strings.TrimSpace(in.Username) == "" || in.Password == "" {
			httpErrorCode(c, http.StatusBadRequest, "username dan password wajib diisi", "INVALID_REQUEST")
			return
		}
		if d.StaffAuth == nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "staff authentication tidak tersedia", "AUTH_UNAVAILABLE")
			return
		}
		token, expires, p, err := d.StaffAuth.Login(c.Request.Context(), in.Username, in.Password)
		var locked *staffauth.LockedError
		switch {
		case errors.As(err, &locked):
			secs := int(time.Until(locked.Until).Seconds())
			if secs < 1 {
				secs = 1
			}
			c.Header("Retry-After", strconv.Itoa(secs))
			httpErrorCode(c, http.StatusTooManyRequests, "akun dikunci sementara, coba lagi nanti", "ACCOUNT_LOCKED")
		case errors.Is(err, staffauth.ErrInvalidCredentials):
			httpErrorCode(c, http.StatusUnauthorized, "username atau password salah", "INVALID_CREDENTIALS")
		case err != nil:
			httpErrorCode(c, http.StatusServiceUnavailable, "staff authentication tidak tersedia", "AUTH_UNAVAILABLE")
		default:
			writeJSON(c, http.StatusOK, map[string]any{
				"token":      token,
				"expires_at": expires.UTC(),
				"staff":      map[string]string{"username": p.Username, "role": p.Role, "full_name": p.FullName},
			})
		}
	}
}

// requireStaffSession memastikan request membawa sesi staf yang valid (role bukan guest).
func requireStaffSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetAuthContext(c.Request.Context()).Role == "guest" {
			writeProblemDetails(c, http.StatusUnauthorized, "Unauthorized", "staff session required", "AUTHENTICATION_REQUIRED")
			c.Abort()
			return
		}
		c.Next()
	}
}

// GET /api/v1/auth/staff/me
func handleStaffMe(_ Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ac := GetAuthContext(c.Request.Context())
		writeJSON(c, http.StatusOK, map[string]string{"username": strings.TrimPrefix(ac.Subject, "staff:"), "role": ac.Role})
	}
}

// POST /api/v1/auth/staff/logout
func handleStaffLogout(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if err := d.StaffAuth.Logout(c.Request.Context(), bearerToken(c)); err != nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "logout gagal, coba lagi", "AUTH_UNAVAILABLE")
			return
		}
		c.Status(http.StatusNoContent)
	}
}
