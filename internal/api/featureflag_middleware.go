package api

import (
	"fmt"
	"net/http"

	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/gin-gonic/gin"
)

// RequireFeature membuat middleware yang memeriksa apakah feature flag tertentu aktif.
// Middleware ini context-aware: otomatis mengekstrak role dari AuthContext jika ada.
func RequireFeature(ff featureflag.Manager, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ff == nil {
			// Fail-open jika feature flag manager belum dikonfigurasi (backward compatibility)
			c.Next()
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
		ctx := featureflag.WithRole(c.Request.Context(), authCtx.Role)
		c.Request = c.Request.WithContext(ctx)

		if !ff.IsEnabled(ctx, key) {
			writeProblemDetails(c, http.StatusServiceUnavailable, "Service Unavailable",
				fmt.Sprintf("Fitur '%s' sedang dinonaktifkan sementara.", key), "FEATURE_DISABLED")
			c.Abort()
			return
		}

		c.Next()
	}
}
