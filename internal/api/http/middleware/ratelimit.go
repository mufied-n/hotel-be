package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter menerapkan token bucket per-klien IP dengan pembersihan lazy sweep berkala.
type RateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientBucket
	rate         rate.Limit
	burst        int
	evictEvery   int
	requestCount int
	maxIdle      time.Duration
	skipRoutes   map[string]bool
}

// NewRateLimiter membuat rate limiter token bucket per client IP.
func NewRateLimiter(r float64, burst int, skipRoutes ...string) *RateLimiter {
	skipMap := make(map[string]bool, len(skipRoutes))
	for _, route := range skipRoutes {
		skipMap[route] = true
	}

	return &RateLimiter{
		clients:    make(map[string]*clientBucket),
		rate:       rate.Limit(r),
		burst:      burst,
		evictEvery: 100,
		maxIdle:    5 * time.Minute,
		skipRoutes: skipMap,
	}
}

func (rl *RateLimiter) cleanupOldClientsLocked(now time.Time) {
	for ip, b := range rl.clients {
		if now.Sub(b.lastSeen) > rl.maxIdle {
			delete(rl.clients, ip)
		}
	}
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rl.requestCount++
	if rl.requestCount >= rl.evictEvery {
		rl.cleanupOldClientsLocked(now)
		rl.requestCount = 0
	}

	b, exists := rl.clients[ip]
	if !exists {
		l := rate.NewLimiter(rl.rate, rl.burst)
		rl.clients[ip] = &clientBucket{limiter: l, lastSeen: now}
		return l
	}

	b.lastSeen = now
	return b.limiter
}

// Limit mengembalikan Gin middleware untuk rate limiting per IP klien (FR-04, FR-08, FR-27).
func (rl *RateLimiter) Limit(skipRoutes ...string) gin.HandlerFunc {
	if rl == nil {
		return func(c *gin.Context) { c.Next() }
	}

	skipMap := make(map[string]bool, len(rl.skipRoutes)+len(skipRoutes))
	for k, v := range rl.skipRoutes {
		skipMap[k] = v
	}
	for _, r := range skipRoutes {
		skipMap[r] = true
	}

	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		if skipMap[path] {
			c.Next()
			return
		}

		ip := c.ClientIP()
		l := rl.getLimiter(ip)
		if !l.Allow() {
			retryAfterSec := int(1.0 / float64(rl.rate))
			if retryAfterSec < 1 {
				retryAfterSec = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSec))
			WriteError(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "too many requests, please slow down")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimit mengevaluasi rate limit token bucket per IP klien (FR-04, FR-08, FR-27).
func RateLimit(limiter *RateLimiter) gin.HandlerFunc {
	return limiter.Limit()
}

