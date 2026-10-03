package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/guest"
	"github.com/go-chi/chi/v5"
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
func requireGuestSession(guestSvc guest.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if guestSvc == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{
					"error":   "GUEST_AUTH_NOT_CONFIGURED",
					"message": "Layanan autentikasi tamu belum dikonfigurasi.",
				})
				return
			}

			token := extractGuestSessionToken(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "UNAUTHORIZED",
					"message": "Sesi tamu tidak ditemukan. Silakan masuk terlebih dahulu.",
				})
				return
			}

			sess, err := guestSvc.ValidateSession(r.Context(), token)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "UNAUTHORIZED",
					"message": "Sesi tamu tidak valid atau telah berakhir.",
				})
				return
			}

			ctx := context.WithValue(r.Context(), guestSessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type guestChallengeRequest struct {
	Email string `json:"email"`
}

type guestVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func handleGuestChallenge(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.GuestSvc == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{
				"error":   "NOT_IMPLEMENTED",
				"message": "Layanan tamu tidak aktif.",
			})
			return
		}

		var req guestChallengeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "INVALID_JSON",
				"message": "Format request tidak valid.",
			})
			return
		}

		cd, err := d.GuestSvc.RequestChallenge(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, guest.ErrInvalidEmail) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error":   "INVALID_EMAIL",
					"message": "Format alamat email tidak valid.",
				})
				return
			}
			if errors.Is(err, guest.ErrRateLimited) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error":   "RATE_LIMIT_EXCEEDED",
					"message": "Harap tunggu 60 detik sebelum meminta kode verifikasi baru.",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memproses permintaan OTP.",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "ok",
			"message":          "Jika email terdaftar atau valid, kode verifikasi 6 digit telah dikirimkan ke email Anda.",
			"cooldown_seconds": cd,
		})
	}
}

func handleGuestVerify(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.GuestSvc == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{
				"error":   "NOT_IMPLEMENTED",
				"message": "Layanan tamu tidak aktif.",
			})
			return
		}

		var req guestVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "INVALID_JSON",
				"message": "Format request tidak valid.",
			})
			return
		}

		token, sess, err := d.GuestSvc.VerifyChallenge(r.Context(), req.Email, req.Code)
		if err != nil {
			if errors.Is(err, guest.ErrMaxAttemptsExceeded) {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error":   "MAX_ATTEMPTS_EXCEEDED",
					"message": "Batas percobaan terlampaui. Silakan minta kode verifikasi baru.",
				})
				return
			}
			if errors.Is(err, guest.ErrInvalidOrExpiredCode) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "INVALID_OR_EXPIRED_CODE",
					"message": "Kode verifikasi salah atau telah kedaluwarsa.",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memvalidasi kode verifikasi.",
			})
			return
		}

		// Pasang cookie session yang aman
		http.SetCookie(w, &http.Cookie{
			Name:     "guest_session",
			Value:    token,
			Path:     "/",
			Expires:  sess.ExpiresAt,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"token":      token,
			"email":      sess.GuestEmail,
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		})
	}
}

func handleGuestMe(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		profile, err := d.GuestSvc.GetSessionProfile(r.Context(), sess)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal mengambil profil tamu.",
			})
			return
		}

		writeJSON(w, http.StatusOK, profile)
	}
}

func handleGuestLogout(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractGuestSessionToken(r)
		if token != "" && d.GuestSvc != nil {
			_ = d.GuestSvc.RevokeSession(r.Context(), token)
		}

		// Hapus cookie session
		http.SetCookie(w, &http.Cookie{
			Name:     "guest_session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "Sesi Anda telah berhasil diakhiri.",
		})
	}
}

func handleGuestBookings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		status := r.URL.Query().Get("status")
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		bookings, err := d.GuestSvc.ListBookings(r.Context(), sess.GuestEmail, status, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat daftar pemesanan.",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  bookings,
			"total": len(bookings),
		})
	}
}

func handleGuestBookingDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := chi.URLParam(r, "id")
		detail, err := d.GuestSvc.GetBookingDetail(r.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				// IDOR defense: Mengembalikan 404 generik
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat detail pemesanan.",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"booking":         detail,
			"allowed_actions": detail.AllowedActions,
		})
	}
}

func handleGuestBookingReceipt(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := chi.URLParam(r, "id")
		receipt, err := d.GuestSvc.GetBookingReceipt(r.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error":   "RECEIPT_NOT_AVAILABLE",
					"message": "Invoice resmi dan bukti reservasi hanya tersedia setelah pembayaran dikonfirmasi.",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memuat invoice bukti pemesanan.",
			})
			return
		}

		w.Header().Set("Cache-Control", "no-store, private")
		writeJSON(w, http.StatusOK, receipt)
	}
}

func handleGuestBookingCalendar(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "UNAUTHORIZED",
				"message": "Sesi tidak ditemukan.",
			})
			return
		}

		id := chi.URLParam(r, "id")
		receipt, err := d.GuestSvc.GetBookingReceipt(r.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error":   "BOOKING_NOT_FOUND",
					"message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.",
				})
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error":   "RECEIPT_NOT_AVAILABLE",
					"message": "File kalender hanya tersedia setelah pembayaran dikonfirmasi.",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal memproses file kalender.",
			})
			return
		}

		icsBytes, err := d.GuestSvc.GenerateCalendarICS(receipt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Gagal menghasilkan file kalender.",
			})
			return
		}

		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"pulang-booking-%s.ics\"", id))
		w.Header().Set("Cache-Control", "no-store, private")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(icsBytes)
	}
}

