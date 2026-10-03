package api

import (
	"fmt"
	"net/http"

	"github.com/example/hotel-booking/internal/platform/featureflag"
)

// RequireFeature membuat middleware yang memeriksa apakah feature flag tertentu aktif.
// Middleware ini context-aware: otomatis mengekstrak role dari AuthContext jika ada.
func RequireFeature(ff featureflag.Manager, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ff == nil {
				// Fail-open jika feature flag manager belum dikonfigurasi (backward compatibility)
				next.ServeHTTP(w, r)
				return
			}

			authCtx := GetAuthContext(r.Context())
			ctx := featureflag.WithRole(r.Context(), authCtx.Role)

			if !ff.IsEnabled(ctx, key) {
				writeProblemDetails(w, http.StatusServiceUnavailable, "Service Unavailable",
					fmt.Sprintf("Fitur '%s' sedang dinonaktifkan sementara.", key), "FEATURE_DISABLED")
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
