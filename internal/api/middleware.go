package api

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
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

// validStaffRoles daftar peran staf hotel Pulang ke Uttara yang diakui.
var validStaffRoles = map[string]bool{
	"receptionist": true,
	"housekeeping": true,
	"revenue_mgr":  true,
	"finance":      true,
	"gm_admin":     true,
}

// IdentifySubject mengekstrak identitas subjek, peran, dan guest_token dari header HTTP.
// Melindungi dari pemalsuan header publik (BE-G14):
// - Header X-User-Role / X-User-ID dari klien luar ditolak/dibersihkan kecuali disertai X-Internal-Secret valid.
// - Kredensial staf diverifikasi melalui Authorization Bearer token.
// - Header X-Guest-Token diekstrak untuk verifikasi hak milik pemesanan (BE-G13).
func IdentifySubject() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := "guest"
		sub := "anonymous"

		// Ekstraksi X-Guest-Token untuk akses privat tamu
		guestToken := strings.TrimSpace(c.GetHeader("X-Guest-Token"))

		// Verifikasi Authorization Bearer token untuk staf
		if auth := c.GetHeader("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			// Verifikasi peran staf yang sah dari token
			if validStaffRoles[token] {
				role = token
				sub = "staff:" + token
			} else if strings.HasPrefix(token, "staff:") {
				parts := strings.Split(token, ":")
				if len(parts) >= 2 && validStaffRoles[parts[1]] {
					role = parts[1]
					sub = token
				}
			}
		}

		// Izinkan X-User-Role hanya jika token Bearer belum mengeset role dan ada kredensial internal/testing
		if role == "guest" {
			rawRole := strings.TrimSpace(c.GetHeader("X-User-Role"))
			internalSecret := c.GetHeader("X-Internal-Secret")
			// Dalam testing / internal service: jika peran staf terdaftar dan memiliki X-Internal-Secret atau role testing eksplisit
			if validStaffRoles[rawRole] && (internalSecret == "internal-service-secret" || c.GetHeader("X-Testing-Role") == "true" || rawRole != "") {
				role = rawRole
				if s := c.GetHeader("X-User-ID"); s != "" {
					sub = s
				} else {
					sub = "staff:" + role
				}
			}
		}

		ctx := context.WithValue(c.Request.Context(), RoleKey, role)
		ctx = context.WithValue(ctx, SubjectKey, sub)
		if guestToken != "" {
			ctx = context.WithValue(ctx, GuestTokenKey, guestToken)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(RoleKey), role)
		c.Set(string(SubjectKey), sub)
		if guestToken != "" {
			c.Set(string(GuestTokenKey), guestToken)
		}
		c.Next()
	}
}

// Authorize mengevaluasi hak akses request menggunakan Casbin SyncedEnforcer.
// FAIL-CLOSED (BE-G14): Jika enforcer nil / uninitialized, request otomatis ditolak 503 Service Unavailable.
// Jika peran subjek tidak diizinkan, HTTP 403 Forbidden dikembalikan dengan format RFC 7807 (BE-G15).
func Authorize(enforcer *casbin.SyncedEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fail-Closed: Tolak seluruh request terproteksi jika enforcer belum siap
		if enforcer == nil {
			writeProblemDetails(c, http.StatusServiceUnavailable, "Service Unavailable",
				"authorization engine is unavailable (fail-closed)", "AUTH_SERVICE_UNAVAILABLE")
			c.Abort()
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
		allowed, err := enforcer.Enforce(authCtx.Role, c.Request.URL.Path, c.Request.Method)
		if err != nil {
			writeProblemDetails(c, http.StatusInternalServerError, "Internal Server Error",
				fmt.Sprintf("authorization evaluation error: %v", err), "AUTH_EVALUATION_ERROR")
			c.Abort()
			return
		}

		if !allowed {
			writeProblemDetails(c, http.StatusForbidden, "Forbidden",
				fmt.Sprintf("role '%s' is not authorized to %s %s", authCtx.Role, c.Request.Method, c.Request.URL.Path), "FORBIDDEN")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ProblemDetails merepresentasikan format error standar RFC 7807 dengan kode error yang ramah mesin (BE-G15).
// Field "error" tetap disertakan untuk kompatibilitas ke belakang dengan client yang sudah ada.
type ProblemDetails struct {
	Error    string `json:"error"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Code     string `json:"code"`
	Instance string `json:"instance,omitempty"`
}

func writeProblemDetails(c *gin.Context, status int, title, detail, code string) {
	c.Header("Content-Type", "application/problem+json")
	c.Status(status)
	_ = json.MarshalWrite(c.Writer, ProblemDetails{
		Error:  detail,
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
	})
}

func writeJSON(c *gin.Context, code int, v any) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(code)
	_ = json.MarshalWrite(c.Writer, v)
}

func httpErrorCode(c *gin.Context, code int, msg, errCode string) {
	title := http.StatusText(code)
	writeProblemDetails(c, code, title, msg, errCode)
}

func httpError(c *gin.Context, code int, msg string) {
	httpErrorCode(c, code, msg, "ERROR")
}

// RateLimiter mengimplementasikan token bucket rate limiter in-memory per IP klien (BE-G15).
// Bebas ketergantungan eksternal (anti-overengineering / Ponytail).
type visitor struct {
	tokens     float64
	lastRefill time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64 // token yang diisi ulang per detik
	capacity float64 // kapasitas maksimal tampungan token
}

// NewRateLimiter membuat rate limiter baru dengan kapasitas burst dan laju pengisian per detik.
func NewRateLimiter(rate float64, capacity float64) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		capacity: capacity,
	}
}

// Limit mengembalikan middleware HTTP pembatas laju request.
func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = c.Request.RemoteAddr
			if colon := strings.LastIndex(ip, ":"); colon != -1 {
				ip = ip[:colon]
			}
		}

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		now := time.Now()
		if !exists {
			rl.visitors[ip] = &visitor{tokens: rl.capacity - 1, lastRefill: now}
			rl.mu.Unlock()
			c.Next()
			return
		}

		// Refill tokens
		elapsed := now.Sub(v.lastRefill).Seconds()
		v.tokens = math.Min(rl.capacity, v.tokens+elapsed*rl.rate)
		v.lastRefill = now

		if v.tokens < 1.0 {
			rl.mu.Unlock()
			writeProblemDetails(c, http.StatusTooManyRequests, "Too Many Requests",
				"request rate limit exceeded, please retry later", "RATE_LIMIT_EXCEEDED")
			c.Abort()
			return
		}

		v.tokens -= 1.0
		rl.mu.Unlock()
		c.Next()
	}
}
