package handler

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

type guestChallengeRequest struct {
	Email string `json:"email" validate:"required"`
}

type guestVerifyRequest struct {
	Email string `json:"email" validate:"required"`
	Code  string `json:"code" validate:"required"`
}

func isSecureCookie(c *gin.Context, isDev bool) bool {
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	return !isDev
}

// GuestChallenge meminta pengiriman kode OTP verifikasi sesi tamu (POST /api/v1/auth/guest/challenge).
func GuestChallenge(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan tamu tidak aktif.")
			return
		}

		var req guestChallengeRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			if middleware.IsMaxBytesError(err) {
				middleware.WriteGuestError(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Ukuran payload melebihi batas maksimal 1MB.")
				return
			}
			middleware.WriteGuestError(c, http.StatusBadRequest, "INVALID_JSON", "Format request tidak valid.")
			return
		}

		cd, err := d.GuestSvc.RequestChallenge(c.Request.Context(), req.Email)
		if err != nil {
			if errors.Is(err, guest.ErrInvalidEmail) {
				middleware.WriteGuestError(c, http.StatusBadRequest, "INVALID_EMAIL", "Format alamat email tidak valid.")
				return
			}
			if errors.Is(err, guest.ErrRateLimited) {
				middleware.WriteGuestError(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Harap tunggu 60 detik sebelum meminta kode verifikasi baru.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memproses permintaan OTP.")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"status":           "ok",
			"delivery_status":  "accepted",
			"message":          "Permintaan kode verifikasi telah diterima. Jika alamat email valid, kode verifikasi 6 digit akan dikirimkan ke email Anda.",
			"cooldown_seconds": cd,
		})
	}
}

// GuestVerify memverifikasi kode OTP dan menerbitkan token sesi (POST /api/v1/auth/guest/verify).
func GuestVerify(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan tamu tidak aktif.")
			return
		}

		var req guestVerifyRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			if middleware.IsMaxBytesError(err) {
				middleware.WriteGuestError(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Ukuran payload melebihi batas maksimal 1MB.")
				return
			}
			middleware.WriteGuestError(c, http.StatusBadRequest, "INVALID_JSON", "Format request tidak valid.")
			return
		}

		token, sess, err := d.GuestSvc.VerifyChallenge(c.Request.Context(), req.Email, req.Code)
		if err != nil {
			if errors.Is(err, guest.ErrMaxAttemptsExceeded) {
				middleware.WriteGuestError(c, http.StatusForbidden, "MAX_ATTEMPTS_EXCEEDED", "Batas percobaan terlampaui. Silakan minta kode verifikasi baru.")
				return
			}
			if errors.Is(err, guest.ErrInvalidOrExpiredCode) {
				middleware.WriteGuestError(c, http.StatusUnauthorized, "INVALID_OR_EXPIRED_CODE", "Kode verifikasi salah atau telah kedaluwarsa.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memvalidasi kode verifikasi.")
			return
		}

		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "guest_session",
			Value:    token,
			Path:     "/",
			Expires:  sess.ExpiresAt,
			HttpOnly: true,
			Secure:   isSecureCookie(c, d.IsDevelopment),
			SameSite: http.SameSiteLaxMode,
		})

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"token":      token,
			"email":      sess.GuestEmail,
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// GuestMe mengembalikan profil tamu dari sesi aktif (GET /api/v1/auth/guest/me).
func GuestMe(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan tamu tidak aktif.")
			return
		}

		profile, err := d.GuestSvc.GetSessionProfile(c.Request.Context(), sess)
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal mengambil profil tamu.")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, profile)
	}
}

// GuestLogout mengakhiri sesi aktif tamu (POST /api/v1/auth/guest/logout).
func GuestLogout(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		token := middleware.ExtractGuestSessionToken(c.Request)
		if token != "" && d.GuestSvc != nil {
			if err := d.GuestSvc.RevokeSession(c.Request.Context(), token); err != nil {
				middleware.WriteGuestError(c, http.StatusServiceUnavailable, "LOGOUT_FAILED", "Gagal mencabut sesi pada server. Silakan coba lagi.")
				return
			}
		}

		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "guest_session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   isSecureCookie(c, d.IsDevelopment),
			SameSite: http.SameSiteLaxMode,
		})

		middleware.WriteJSON(c, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "Sesi Anda telah berhasil diakhiri.",
		})
	}
}
