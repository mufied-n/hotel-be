package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestID_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		inHeader   string
		wantCustom bool
	}{
		{"empty generates 32 hex chars", "", false},
		{"valid custom id retained", "my-custom-request-id-123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequestID())
			var capturedID string
			r.GET("/test", func(c *gin.Context) {
				capturedID = c.Writer.Header().Get("X-Request-Id")
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.inHeader != "" {
				req.Header.Set("X-Request-Id", tt.inHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			respHeader := w.Header().Get("X-Request-Id")
			if respHeader == "" {
				t.Fatal("expected X-Request-Id in response header")
			}
			if respHeader != capturedID {
				t.Fatalf("response header %q != context id %q", respHeader, capturedID)
			}
			if tt.wantCustom && respHeader != tt.inHeader {
				t.Fatalf("got %q, want custom %q", respHeader, tt.inHeader)
			}
		})
	}
}

func TestRecoverProblem_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestID())
	r.Use(RecoverProblem(false))
	r.GET("/panic", func(c *gin.Context) {
		panic("database connection exploded: credentials=supersecret")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	body := w.Body.String()
	if bytes.Contains(w.Body.Bytes(), []byte("credentials=supersecret")) {
		t.Fatalf("panic details leaked to client: %s", body)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("INTERNAL_SERVER_ERROR")) {
		t.Fatalf("expected INTERNAL_SERVER_ERROR in body: %s", body)
	}
}

func TestSecureHeaders_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SecureHeaders())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected nosniff header, got %q", w.Header().Get("X-Content-Type-Options"))
	}
}

func TestNoStore_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(NoStore())
	r.GET("/sensitive", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/sensitive", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Cache-Control") != "no-store, no-cache, must-revalidate, private" {
		t.Errorf("expected no-store, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestCORS_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORS([]string{"https://hotel.example.com"}, false))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 1. Regular GET with matching origin
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://hotel.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://hotel.example.com" {
		t.Errorf("expected allowed origin header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}

	// 2. Preflight OPTIONS
	optReq := httptest.NewRequest(http.MethodOptions, "/test", nil)
	optReq.Header.Set("Origin", "https://hotel.example.com")
	optW := httptest.NewRecorder()
	r.ServeHTTP(optW, optReq)

	if optW.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", optW.Code)
	}

	// 3. Disallowed origin
	badReq := httptest.NewRequest(http.MethodGet, "/test", nil)
	badReq.Header.Set("Origin", "https://evil.com")
	badW := httptest.NewRecorder()
	r.ServeHTTP(badW, badReq)

	if badW.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("disallowed origin should not have Access-Control-Allow-Origin header")
	}
}

func TestRateLimiter_Features(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Retry-After header when rate limit exceeded", func(t *testing.T) {
		limiter := NewRateLimiter(1, 2)
		r := gin.New()
		r.Use(RateLimit(limiter))
		r.GET("/ping", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req1 := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("req 1 failed: %d", w1.Code)
		}

		req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("req 2 failed: %d", w2.Code)
		}

		req3 := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w3 := httptest.NewRecorder()
		r.ServeHTTP(w3, req3)

		if w3.Code != http.StatusTooManyRequests {
			t.Fatalf("req 3 expected 429, got %d", w3.Code)
		}
		if retryAfter := w3.Header().Get("Retry-After"); retryAfter == "" {
			t.Errorf("expected Retry-After header on 429 response")
		}
	})

	t.Run("Skip path exempts healthz", func(t *testing.T) {
		limiter := NewRateLimiter(1, 1, "/healthz")
		r := gin.New()
		r.Use(RateLimit(limiter))
		r.GET("/healthz", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("health check %d got throttled: %d", i, w.Code)
			}
		}
	})
}

func TestNoRouteAndNoMethod_ProblemDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.NoRoute(NotFound())
	r.NoMethod(MethodNotAllowed())
	r.GET("/items", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 1. 404 Not Found
	req404 := httptest.NewRequest(http.MethodGet, "/non-existent", nil)
	w404 := httptest.NewRecorder()
	r.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w404.Code)
	}
	if !bytes.Contains(w404.Body.Bytes(), []byte("NOT_FOUND")) {
		t.Fatalf("expected NOT_FOUND in problem details: %s", w404.Body.String())
	}

	// 2. 405 Method Not Allowed
	req405 := httptest.NewRequest(http.MethodPost, "/items", nil)
	w405 := httptest.NewRecorder()
	r.ServeHTTP(w405, req405)

	if w405.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w405.Code)
	}
	if !bytes.Contains(w405.Body.Bytes(), []byte("METHOD_NOT_ALLOWED")) {
		t.Fatalf("expected METHOD_NOT_ALLOWED in problem details: %s", w405.Body.String())
	}
}

func TestAccessLog_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(origLogger)

	r := gin.New()
	r.Use(RequestID(), AccessLog(slog.Default()))
	r.GET("/api/v1/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	logs := logBuf.String()
	if !bytes.Contains(logBuf.Bytes(), []byte("http.request")) {
		t.Errorf("expected http.request in logs, got %s", logs)
	}
}

func TestNoStore_Headers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(NoStore())
	r.GET("/nocache", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/nocache", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Header().Get("Cache-Control") != "no-store, no-cache, must-revalidate, private" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestTimeoutContext_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TimeoutContext(time.Second))
	r.GET("/timeout", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		if !ok || deadline.IsZero() {
			t.Errorf("expected deadline set on context")
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/timeout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestDecodeJSON_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
	}{
		{
			name:   "valid JSON decodes ok",
			body:   `{"key":"val"}`,
			wantOK: true,
		},
		{
			name:       "invalid JSON returns false and writes 400",
			body:       `{invalid`,
			wantOK:     false,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/decode", bytes.NewBufferString(tc.body))

			var v map[string]any
			ok := DecodeJSON(c, &v, "INVALID_JSON", "bad json")
			if ok != tc.wantOK {
				t.Errorf("DecodeJSON ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK && rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

