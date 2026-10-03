// Package http menyediakan HTTP transport layer / driving adapter (Gin engine,
// routing, middleware pipeline, dan domain handlers) untuk Pulang ke Uttara.
package http

import (
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/gin-gonic/gin"
)

// NewRouter merakit seluruh route HTTP menggunakan Gin engine (FR-01, FR-18).
func NewRouter(d Deps) *gin.Engine {
	if d.IdempotencyStore == nil {
		d.IdempotencyStore = NewMemoryIdempotencyStore()
	}
	d = d.WithDefaults()

	r := gin.New()
	_ = r.SetTrustedProxies(d.TrustedProxies)
	r.HandleMethodNotAllowed = true
	r.NoRoute(middleware.NotFound())
	r.NoMethod(middleware.MethodNotAllowed())

	// Pipeline Middleware Terurut (FR-18):
	// 1. RequestID (korelasi & tracing)
	r.Use(middleware.RequestID())
	// 2. AccessLog (observabilitas bebas PII)
	r.Use(middleware.AccessLog())
	// 3. Panic Recovery (RFC 7807 problem+json)
	r.Use(middleware.RecoverProblem(d.IsDevelopment))
	// 4. Secure Headers (nosniff)
	r.Use(middleware.SecureHeaders())
	// 5. CORS (opt-in allowlist)
	if len(d.CORSOrigins) > 0 {
		r.Use(middleware.CORS(d.CORSOrigins, d.IsDevelopment))
	}
	// 6. Timeout Context (default 30s)
	r.Use(middleware.TimeoutContext(d.RequestTimeout))
	// 7. Global Rate Limiter
	if d.RateLimiter != nil {
		r.Use(d.RateLimiter.Limit(d.SkipRateLimitRoutes...))
	}
	// 8. Body Size Limit (1 MB) (BE-R18)
	r.Use(middleware.BodySizeLimit(middleware.DefaultMaxBodyBytes))

	registerRoutes(r, d)

	return r
}
