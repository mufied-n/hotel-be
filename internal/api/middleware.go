package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/casbin/casbin/v2"
)

type contextKey string

const (
	RoleKey    contextKey = "user_role"
	SubjectKey contextKey = "user_subject"
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

// IdentifySubject mengekstrak identitas subjek dan peran dari header HTTP.
// Mendukung header X-User-Role / X-User-ID atau Authorization Bearer token.
// Jika tidak ada kredensial, default ke subjek "anonymous" dengan role "guest".
func IdentifySubject() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := r.Header.Get("X-User-Role")
			sub := r.Header.Get("X-User-ID")

			if auth := r.Header.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				if role == "" {
					role = token
				}
			}

			if role == "" {
				role = "guest"
			}
			if sub == "" {
				sub = "anonymous"
			}

			ctx := context.WithValue(r.Context(), RoleKey, role)
			ctx = context.WithValue(ctx, SubjectKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authorize mengevaluasi hak akses request menggunakan Casbin SyncedEnforcer.
// Jika peran subjek tidak diizinkan mengakses path dan HTTP method, HTTP 403 Forbidden dikembalikan.
func Authorize(enforcer *casbin.SyncedEnforcer) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if enforcer == nil {
				next.ServeHTTP(w, r)
				return
			}

			authCtx := GetAuthContext(r.Context())
			allowed, err := enforcer.Enforce(authCtx.Role, r.URL.Path, r.Method)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_server_error",
					"message": fmt.Sprintf("authorization evaluation error: %v", err),
				})
				return
			}

			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": fmt.Sprintf("role '%s' is not authorized to %s %s", authCtx.Role, r.Method, r.URL.Path),
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
