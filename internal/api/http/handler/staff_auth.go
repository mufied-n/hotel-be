package handler

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/staffauth"
	"github.com/gin-gonic/gin"
)

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// StaffLogin menangani autentikasi staf (POST /api/v1/auth/staff/login).
func StaffLogin(d Deps) gin.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<10))
		var in req
		if err != nil || json.Unmarshal(body, &in) != nil || strings.TrimSpace(in.Username) == "" || in.Password == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "username dan password wajib diisi", "INVALID_REQUEST")
			return
		}
		if d.StaffAuth == nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "staff authentication tidak tersedia", "AUTH_UNAVAILABLE")
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
			middleware.HttpErrorCode(c, http.StatusTooManyRequests, "akun dikunci sementara, coba lagi nanti", "ACCOUNT_LOCKED")
		case errors.Is(err, staffauth.ErrInvalidCredentials):
			middleware.HttpErrorCode(c, http.StatusUnauthorized, "username atau password salah", "INVALID_CREDENTIALS")
		case err != nil:
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "staff authentication tidak tersedia", "AUTH_UNAVAILABLE")
		default:
			middleware.WriteJSON(c, http.StatusOK, map[string]any{
				"token":      token,
				"expires_at": expires.UTC(),
				"staff":      map[string]string{"username": p.Username, "role": p.Role, "full_name": p.FullName},
			})
		}
	}
}

// StaffMe membaca profil staf yang sedang login (GET /api/v1/auth/staff/me).
func StaffMe(_ Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ac := middleware.GetAuthContext(c.Request.Context())
		middleware.WriteJSON(c, http.StatusOK, map[string]string{"username": strings.TrimPrefix(ac.Subject, "staff:"), "role": ac.Role})
	}
}

// StaffLogout mengakhiri sesi aktif staf (POST /api/v1/auth/staff/logout).
func StaffLogout(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if d.StaffAuth == nil {
			c.Status(http.StatusNoContent)
			return
		}
		if err := d.StaffAuth.Logout(c.Request.Context(), bearerToken(c)); err != nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "logout gagal, coba lagi", "AUTH_UNAVAILABLE")
			return
		}
		c.Status(http.StatusNoContent)
	}
}
