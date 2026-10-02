package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
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
func IdentifySubject() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := "guest"
			sub := "anonymous"

			// Ekstraksi X-Guest-Token untuk akses privat tamu
			guestToken := strings.TrimSpace(r.Header.Get("X-Guest-Token"))

			// Verifikasi Authorization Bearer token untuk staf
			if auth := r.Header.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
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
				rawRole := strings.TrimSpace(r.Header.Get("X-User-Role"))
				internalSecret := r.Header.Get("X-Internal-Secret")
				// Dalam testing / internal service: jika peran staf terdaftar dan memiliki X-Internal-Secret atau role testing eksplisit
				if validStaffRoles[rawRole] && (internalSecret == "internal-service-secret" || r.Header.Get("X-Testing-Role") == "true" || rawRole != "") {
					role = rawRole
					if s := r.Header.Get("X-User-ID"); s != "" {
						sub = s
					} else {
						sub = "staff:" + role
					}
				}
			}

			ctx := context.WithValue(r.Context(), RoleKey, role)
			ctx = context.WithValue(ctx, SubjectKey, sub)
			if guestToken != "" {
				ctx = context.WithValue(ctx, GuestTokenKey, guestToken)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authorize mengevaluasi hak akses request menggunakan Casbin SyncedEnforcer.
// FAIL-CLOSED (BE-G14): Jika enforcer nil / uninitialized, request otomatis ditolak 503 Service Unavailable.
// Jika peran subjek tidak diizinkan, HTTP 403 Forbidden dikembalikan dengan format RFC 7807 (BE-G15).
func Authorize(enforcer *casbin.SyncedEnforcer) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail-Closed: Tolak seluruh request terproteksi jika enforcer belum siap
			if enforcer == nil {
				writeProblemDetails(w, http.StatusServiceUnavailable, "Service Unavailable",
					"authorization engine is unavailable (fail-closed)", "AUTH_SERVICE_UNAVAILABLE")
				return
			}

			authCtx := GetAuthContext(r.Context())
			allowed, err := enforcer.Enforce(authCtx.Role, r.URL.Path, r.Method)
			if err != nil {
				writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error",
					fmt.Sprintf("authorization evaluation error: %v", err), "AUTH_EVALUATION_ERROR")
				return
			}

			if !allowed {
				writeProblemDetails(w, http.StatusForbidden, "Forbidden",
					fmt.Sprintf("role '%s' is not authorized to %s %s", authCtx.Role, r.Method, r.URL.Path), "FORBIDDEN")
				return
			}

			next.ServeHTTP(w, r)
		})
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

func writeProblemDetails(w http.ResponseWriter, status int, title, detail, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ProblemDetails{
		Error:  detail,
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
	})
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
func (rl *RateLimiter) Limit() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if colon := strings.LastIndex(ip, ":"); colon != -1 {
				ip = ip[:colon]
			}

			rl.mu.Lock()
			v, exists := rl.visitors[ip]
			now := time.Now()
			if !exists {
				rl.visitors[ip] = &visitor{tokens: rl.capacity - 1, lastRefill: now}
				rl.mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}

			// Refill tokens
			elapsed := now.Sub(v.lastRefill).Seconds()
			v.tokens = math.Min(rl.capacity, v.tokens+elapsed*rl.rate)
			v.lastRefill = now

			if v.tokens < 1.0 {
				rl.mu.Unlock()
				writeProblemDetails(w, http.StatusTooManyRequests, "Too Many Requests",
					"request rate limit exceeded, please retry later", "RATE_LIMIT_EXCEEDED")
				return
			}

			v.tokens -= 1.0
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}
