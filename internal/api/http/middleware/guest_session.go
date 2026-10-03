package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

type GuestContextKey struct{}

var (
	GuestSessionContextKey = GuestContextKey{}
	guestSessionContextKey = GuestSessionContextKey
)

// GuestSessionFromContext mengambil sesi tamu aktif dari context request.
func GuestSessionFromContext(ctx context.Context) *guest.GuestSession {
	if s, ok := ctx.Value(guestSessionContextKey).(*guest.GuestSession); ok {
		return s
	}
	return nil
}

// ExtractGuestSessionToken mengekstrak token sesi tamu dari Authorization header, header khusus, atau cookie.
func ExtractGuestSessionToken(r *http.Request) string {
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

// WriteGuestError menulis respons error dual-shape yang kompatibel dengan klien portal tamu.
func WriteGuestError(c *gin.Context, status int, code, message string) {
	WriteJSON(c, status, map[string]any{
		"error":   code,
		"code":    code,
		"message": message,
		"detail":  message,
		"title":   http.StatusText(status),
		"status":  status,
	})
}

// RequireGuestSession memvalidasi bahwa request menyertakan token sesi portal tamu yang aktif (BE-R04).
func RequireGuestSession(guestSvc guest.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if guestSvc == nil {
			WriteGuestError(c, http.StatusNotImplemented, "GUEST_AUTH_NOT_CONFIGURED", "Layanan autentikasi tamu belum dikonfigurasi.")
			c.Abort()
			return
		}

		token := ExtractGuestSessionToken(c.Request)
		if token == "" {
			WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tamu tidak ditemukan. Silakan masuk terlebih dahulu.")
			c.Abort()
			return
		}

		sess, err := guestSvc.ValidateSession(c.Request.Context(), token)
		if err != nil {
			WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tamu tidak valid atau telah berakhir.")
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), guestSessionContextKey, sess)
		c.Request = c.Request.WithContext(ctx)
		c.Set("guest_session", sess)
		c.Next()
	}
}
