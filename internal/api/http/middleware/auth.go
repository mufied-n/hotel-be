package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/staffauth"
)

type contextKey string

const (
	RoleKey       contextKey = "user_role"
	SubjectKey    contextKey = "user_subject"
	GuestTokenKey contextKey = "guest_token"
)

// AuthContext menyimpan informasi subjek dan perannya dari request.
type AuthContext struct {
	Subject string
	Role    string
}

// GetAuthContext mengambil role dan subject dari request context.
func GetAuthContext(ctx context.Context) AuthContext {
	role, _ := ctx.Value(RoleKey).(string)
	if role == "" {
		role = "guest"
	}
	sub, _ := ctx.Value(SubjectKey).(string)
	if sub == "" {
		sub = "anonymous"
	}
	return AuthContext{
		Subject: sub,
		Role:    role,
	}
}

// GetGuestToken mengambil token tamu dari request context (BE-G13).
func GetGuestToken(ctx context.Context) string {
	tok, _ := ctx.Value(GuestTokenKey).(string)
	return tok
}

// StaffVerifier memverifikasi bearer token sesi staf menjadi identitas Principal (BE-R01).
type StaffVerifier interface {
	VerifyStaffToken(ctx context.Context, token string) (staffauth.Principal, error)
}

// StaffAuthService memadukan verifikasi token dan flow kredensial staf.
type StaffAuthService interface {
	StaffVerifier
	Login(ctx context.Context, username, password string) (string, time.Time, staffauth.Principal, error)
	Logout(ctx context.Context, token string) error
}

type testStaffAuth struct{}

func (testStaffAuth) VerifyStaffToken(_ context.Context, token string) (staffauth.Principal, error) {
	if token == "stf_test_token" {
		return staffauth.Principal{StaffID: "test-receptionist", Username: "test_staff", Role: "receptionist"}, nil
	}
	if validStaffRoles[token] {
		return staffauth.Principal{StaffID: "test-" + token, Username: token, Role: token}, nil
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}

func (testStaffAuth) Login(context.Context, string, string) (string, time.Time, staffauth.Principal, error) {
	return "stf_test_token", time.Now().Add(time.Hour), staffauth.Principal{Username: "test_staff", Role: "receptionist"}, nil
}

func (testStaffAuth) Logout(context.Context, string) error { return nil }

// TestStaffVerifier mengembalikan implementasi verifier pengujian statis.
func TestStaffVerifier() StaffAuthService { return testStaffAuth{} }

// validStaffRoles daftar peran staf hotel Pulang ke Uttara yang diakui.
var validStaffRoles = map[string]bool{
	"receptionist": true,
	"housekeeping": true,
	"revenue_mgr":  true,
	"finance":      true,
	"gm_admin":     true,
}

type staffOutcome int

const (
	outcomeGuest staffOutcome = iota
	outcomeStaff
	outcomeUnauthorized
	outcomeUnavailable
)

// resolveStaff memeriksa header otorisasi dan memverifikasi token sesi staf secara terisolasi (FR-24).
func resolveStaff(ctx context.Context, authHeader string, verifier StaffVerifier) (staffauth.Principal, staffOutcome) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return staffauth.Principal{}, outcomeGuest
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	isStaffToken := strings.HasPrefix(token, staffauth.TokenPrefix)

	if verifier == nil {
		if isStaffToken {
			return staffauth.Principal{}, outcomeUnavailable
		}
		return staffauth.Principal{}, outcomeGuest
	}

	p, err := verifier.VerifyStaffToken(ctx, token)
	switch {
	case err == nil && validStaffRoles[p.Role]:
		return p, outcomeStaff
	case err == nil || errors.Is(err, staffauth.ErrUnauthorized):
		if isStaffToken {
			return staffauth.Principal{}, outcomeUnauthorized
		}
		return staffauth.Principal{}, outcomeGuest
	default:
		if isStaffToken {
			return staffauth.Principal{}, outcomeUnavailable
		}
		return staffauth.Principal{}, outcomeGuest
	}
}

// IdentifySubject mengekstrak identitas subjek, peran, dan guest_token dari request (BE-R01, BE-G14).
func IdentifySubject(verifier StaffVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := "guest"
		sub := "anonymous"

		guestToken := strings.TrimSpace(c.GetHeader("X-Guest-Token"))
		authHeader := c.GetHeader("Authorization")

		p, outcome := resolveStaff(c.Request.Context(), authHeader, verifier)
		switch outcome {
		case outcomeStaff:
			role = p.Role
			sub = "staff:" + p.Username
		case outcomeUnauthorized:
			WriteProblemDetails(c, http.StatusUnauthorized, "Unauthorized",
				"staff session is invalid, expired or revoked", "AUTHENTICATION_REQUIRED")
			c.Abort()
			return
		case outcomeUnavailable:
			WriteProblemDetails(c, http.StatusServiceUnavailable, "Service Unavailable",
				"staff authentication is unavailable", "AUTH_UNAVAILABLE")
			c.Abort()
			return
		case outcomeGuest:
			// default role=guest, sub=anonymous
		}

		ctx := context.WithValue(c.Request.Context(), RoleKey, role)
		ctx = context.WithValue(ctx, SubjectKey, sub)
		if guestToken != "" {
			ctx = context.WithValue(ctx, GuestTokenKey, guestToken)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequireStaffSession memastikan request membawa sesi staf yang valid (role bukan guest).
func RequireStaffSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetAuthContext(c.Request.Context()).Role == "guest" {
			WriteProblemDetails(c, http.StatusUnauthorized, "Unauthorized", "staff session required", "AUTHENTICATION_REQUIRED")
			c.Abort()
			return
		}
		c.Next()
	}
}

// Authorize mengevaluasi hak akses request menggunakan Casbin SyncedEnforcer via c.FullPath() (FR-25).
// FAIL-CLOSED (BE-G14): Jika enforcer nil / uninitialized, request otomatis ditolak 503 Service Unavailable.
// Jika peran subjek tidak diizinkan, HTTP 403 Forbidden dikembalikan dengan format RFC 7807 (BE-G15).
func Authorize(enforcer *casbin.SyncedEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enforcer == nil {
			WriteProblemDetails(c, http.StatusServiceUnavailable, "Service Unavailable",
				"authorization engine is unavailable (fail-closed)", "AUTH_SERVICE_UNAVAILABLE")
			c.Abort()
			return
		}

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		authCtx := GetAuthContext(c.Request.Context())
		allowed, err := enforcer.Enforce(authCtx.Role, path, c.Request.Method)
		if err != nil {
			WriteProblemDetails(c, http.StatusInternalServerError, "Internal Server Error",
				fmt.Sprintf("authorization evaluation error: %v", err), "AUTH_EVALUATION_ERROR")
			c.Abort()
			return
		}

		if !allowed {
			WriteProblemDetails(c, http.StatusForbidden, "Forbidden",
				"role not authorized for this resource", "FORBIDDEN")
			c.Abort()
			return
		}

		c.Next()
	}
}
