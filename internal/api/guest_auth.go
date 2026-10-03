package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

type guestContextKey struct{}

var guestSessionContextKey = guestContextKey{}

// GuestSessionFromContext mengambil sesi tamu aktif dari context request.
func GuestSessionFromContext(ctx context.Context) *guest.GuestSession {
	if s, ok := ctx.Value(guestSessionContextKey).(*guest.GuestSession); ok {
		return s
	}
	return nil
}

func extractGuestSessionToken(r *http.Request) string {
	// 1. Cek Header Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if strings.HasPrefix(token, "gst_sess_") {
			return token
		}
	}

	// 2. Cek Header X-Guest-Session
	if token := r.Header.Get("X-Guest-Session"); token != "" {
		return strings.TrimSpace(token)
	}

	// 3. Cek Cookie guest_session
	if cookie, err := r.Cookie("guest_session"); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}

	return ""
}

// requireGuestSession adalah middleware yang memvalidasi token sesi tamu (OWASP ASVS V3).
func requireGuestSession(guestSvc guest.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if guestSvc == nil {
			writeJSON(c, http.StatusNotImplemented, map[string]string{
				"error":   "GUEST_AUTH_NOT_CONFIGURED",
				"message": "Layanan autentikasi tamu belum dikonfigurasi.",
			})
			c.Abort()
			return
		}

		token := extractGuestSessionToken(c.Request)
		if token == "" {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tamu tidak ditemukan. Silakan masuk terlebih dahulu.",
			})
			c.Abort()
			return
		}

		sess, err := guestSvc.ValidateSession(c.Request.Context(), token)
		if err != nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tamu tidak valid atau telah berakhir.",
			})
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), guestSessionContextKey, sess)
		c.Request = c.Request.WithContext(ctx)
		c.Set("guest_session", sess)
		c.Next()
	}
}

type guestChallengeRequest struct {
	Email string `json:"email" validate:"required"`
}

type guestVerifyRequest struct {
	Email string `json:"email" validate:"required"`
	Code  string `json:"code" validate:"required"`
}

func handleGuestChallenge(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil {
			writeJSON(c, http.StatusNotImplemented, map[string]string{
				"error":   "NOT_IMPLEMENTED",
				"message": "Layanan tamu tidak aktif.",
			})
			return
		}

		var req guestChallengeRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			writeJSON(c, http.StatusBadRequest, map[string]string{
				"error":   "INVALID_JSON",
				"message": "Format request tidak valid.",
			})
			return
		}

		cd, err := d.GuestSvc.RequestChallenge(c.Request.Context(), req.Email)
		if err != nil {
			if errors.Is(err, guest.ErrInvalidEmail) {
				writeJSON(c, http.StatusBadRequest, map[string]string{
					"error":   "INVALID_EMAIL",
					"message": "Format alamat email tidak valid.",
				})
				return
			}
			if errors.Is(err, guest.ErrRateLimited) {
				writeJSON(c, http.StatusTooManyRequests, map[string]string{
					"error":   "RATE_LIMIT_EXCEEDED",
					"message": "Harap tunggu 60 detik sebelum meminta kode verifikasi baru.",
				})
				return
			}
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memproses permintaan OTP.",
			})
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"status":           "ok",
			"message":          "Jika email terdaftar atau valid, kode verifikasi 6 digit telah dikirimkan ke email Anda.",
			"cooldown_seconds": cd,
		})
	}
}

func handleGuestVerify(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil {
			writeJSON(c, http.StatusNotImplemented, map[string]string{
				"error":   "NOT_IMPLEMENTED",
				"message": "Layanan tamu tidak aktif.",
			})
			return
		}

		var req guestVerifyRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			writeJSON(c, http.StatusBadRequest, map[string]string{
				"error":   "INVALID_JSON",
				"message": "Format request tidak valid.",
			})
			return
		}

		token, sess, err := d.GuestSvc.VerifyChallenge(c.Request.Context(), req.Email, req.Code)
		if err != nil {
			if errors.Is(err, guest.ErrMaxAttemptsExceeded) {
				writeJSON(c, http.StatusForbidden, map[string]string{
					"error":   "MAX_ATTEMPTS_EXCEEDED",
					"message": "Batas percobaan terlampaui. Silakan minta kode verifikasi baru.",
				})
				return
			}
			if errors.Is(err, guest.ErrInvalidOrExpiredCode) {
				writeJSON(c, http.StatusUnauthorized, map[string]string{
					"error":   "INVALID_OR_EXPIRED_CODE",
					"message": "Kode verifikasi salah atau telah kedaluwarsa.",
				})
				return
			}
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memvalidasi kode verifikasi.",
			})
			return
		}

		// Pasang header anti-cache pada response data privat sesi (BE-R04)
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		// Pasang cookie session yang aman (OWASP ASVS V3, BE-R04)
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "guest_session",
			Value:    token,
			Path:     "/",
			Expires:  sess.ExpiresAt,
			HttpOnly: true,
			Secure:   isSecureCookie(c, d.IsDevelopment),
			SameSite: http.SameSiteLaxMode,
		})

		writeJSON(c, http.StatusOK, map[string]any{
			"token":      token,
			"email":      sess.GuestEmail,
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		})
	}
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

func handleGuestMe(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		sess := GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		profile, err := d.GuestSvc.GetSessionProfile(c.Request.Context(), sess)
		if err != nil {
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal mengambil profil tamu.",
			})
			return
		}

		writeJSON(c, http.StatusOK, profile)
	}
}

func handleGuestLogout(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")

		token := extractGuestSessionToken(c.Request)
		if token != "" && d.GuestSvc != nil {
			if err := d.GuestSvc.RevokeSession(c.Request.Context(), token); err != nil {
				writeJSON(c, http.StatusServiceUnavailable, map[string]string{
					"error":   "LOGOUT_FAILED",
					"message": "Gagal mencabut sesi pada server. Silakan coba lagi.",
				})
				return
			}
		}

		// Hapus cookie session secara aman (BE-R04)
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

		writeJSON(c, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "Sesi Anda telah berhasil diakhiri.",
		})
	}
}

func handleGuestBookings(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		status := c.Query("status")
		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		bookings, err := d.GuestSvc.ListBookings(c.Request.Context(), sess.GuestEmail, status, limit)
		if err != nil {
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat daftar pemesanan.",
			})
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"data":  bookings,
			"total": len(bookings),
		})
	}
}

func handleGuestBookingDetail(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := c.Param("id")
		detail, err := d.GuestSvc.GetBookingDetail(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				// IDOR defense: Mengembalikan 404 generik
				writeJSON(c, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat detail pemesanan.",
			})
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"booking":         detail,
			"allowed_actions": detail.AllowedActions,
		})
	}
}

func handleGuestBookingReceipt(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := c.Param("id")
		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				writeJSON(c, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				writeJSON(c, http.StatusBadRequest, map[string]string{
					"error":   "RECEIPT_NOT_AVAILABLE",
					"message": "Invoice resmi dan bukti reservasi hanya tersedia setelah pembayaran dikonfirmasi.",
				})
				return
			}
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat invoice bukti pemesanan.",
			})
			return
		}

		c.Header("Cache-Control", "no-store, private")
		writeJSON(c, http.StatusOK, receipt)
	}
}

func handleGuestBookingCalendar(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			writeJSON(c, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := c.Param("id")
		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				writeJSON(c, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				writeJSON(c, http.StatusBadRequest, map[string]string{
					"error":   "RECEIPT_NOT_AVAILABLE",
					"message": "File kalender hanya tersedia setelah pembayaran dikonfirmasi.",
				})
				return
			}
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memproses file kalender.",
			})
			return
		}

		icsBytes, err := d.GuestSvc.GenerateCalendarICS(receipt)
		if err != nil {
			writeJSON(c, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal menghasilkan file kalender.",
			})
			return
		}

		c.Header("Content-Type", "text/calendar; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"pulang-booking-%s.ics\"", id))
		c.Header("Cache-Control", "no-store, private")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(icsBytes)
	}
}
