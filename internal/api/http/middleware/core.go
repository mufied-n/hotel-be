package middleware

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DefaultMaxBodyBytes adalah batas ukuran maksimal payload request (1 MB = 1,048,576 byte) (BE-R18).
const DefaultMaxBodyBytes int64 = 1 << 20

// ProblemDetails merepresentasikan format error standar RFC 7807 dengan kode error yang ramah mesin (BE-G15, BE-R18).
// Field "error" tetap disertakan untuk kompatibilitas ke belakang dengan client yang sudah ada.
type ProblemDetails struct {
	Code      string `json:"code"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteProblemDetails menulis payload application/problem+json seragam.
func WriteProblemDetails(c *gin.Context, status int, title, detail, code string) {
	c.Header("Content-Type", "application/problem+json")
	c.Status(status)
	reqID := c.Writer.Header().Get("X-Request-Id")
	_ = json.MarshalWrite(c.Writer, ProblemDetails{
		Code:      code,
		Error:     detail,
		Message:   detail,
		Detail:    detail,
		Title:     title,
		Status:    status,
		RequestID: reqID,
	})
}

// WriteError menulis respons RFC 7807 terstandarisasi tunggal (FR-26).
func WriteError(c *gin.Context, status int, code, msg string) {
	title := http.StatusText(status)
	WriteProblemDetails(c, status, title, msg, code)
}

// WriteJSON menulis response body berformat application/json utf-8.
func WriteJSON(c *gin.Context, code int, v any) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(code)
	_ = json.MarshalWrite(c.Writer, v)
}

// HttpErrorCode alias untuk WriteError.
func HttpErrorCode(c *gin.Context, code int, msg, errCode string) {
	WriteError(c, code, errCode, msg)
}

// BodySizeLimit membatasi konsumsi stream request body menggunakan http.MaxBytesReader (BE-R18).
func BodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// IsMaxBytesError mengecek apakah error berasal dari http.MaxBytesReader.
func IsMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
}

// DecodeJSON membaca JSON dari c.Request.Body dengan perlindungan payload limit dan error formatting (BE-R18).
func DecodeJSON(c *gin.Context, v any, invalidJSONCode, invalidJSONMsg string) bool {
	err := json.UnmarshalRead(c.Request.Body, v)
	if err == nil {
		return true
	}
	if IsMaxBytesError(err) {
		HttpErrorCode(c, http.StatusRequestEntityTooLarge, "request body melebihi batas 1MB", "PAYLOAD_TOO_LARGE")
		return false
	}
	code := invalidJSONCode
	if code == "" {
		code = "INVALID_JSON"
	}
	msg := invalidJSONMsg
	if msg == "" {
		msg = "body JSON tidak valid"
	}
	HttpErrorCode(c, http.StatusBadRequest, msg, code)
	return false
}

// RequestID menginjeksi header X-Request-Id ke request dan response (FR-20).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
		if reqID == "" {
			reqID = strings.ReplaceAll(uuid.New().String(), "-", "")
		}
		c.Writer.Header().Set("X-Request-Id", reqID)
		c.Header("X-Request-Id", reqID)
		c.Next()
	}
}

// AccessLog mencatat akses HTTP terstruktur via log/slog tanpa mencatat PII (FR-21).
func AccessLog(loggers ...*slog.Logger) gin.HandlerFunc {
	var logger *slog.Logger
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	} else {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		reqID := c.Writer.Header().Get("X-Request-Id")
		role := "guest"
		if r, ok := c.Request.Context().Value(RoleKey).(string); ok && r != "" {
			role = r
		}

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"route", route,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"bytes", c.Writer.Size(),
			"request_id", reqID,
			"role", role,
		}

		if logger != nil {
			if status >= 500 {
				logger.Error("http.request", attrs...)
			} else {
				logger.Info("http.request", attrs...)
			}
		}
	}
}

// RecoverProblem menangkap panic dan mengembalikan respons 500 RFC 7807 tanpa membocorkan stack trace (FR-22).
func RecoverProblem(isDev bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := c.Writer.Header().Get("X-Request-Id")
				slog.Error("panic recovered in http handler",
					"error", fmt.Sprint(rec),
					"route", c.FullPath(),
					"request_id", reqID,
				)

				msg := "an unexpected internal error occurred"
				if isDev {
					msg = fmt.Sprintf("panic: %v", rec)
				}
				WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", msg)
				c.Abort()
			}
		}()
		c.Next()
	}
}

// SecureHeaders memasang header nosniff untuk proteksi MIME sniffing (FR-05).
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Next()
	}
}

// NoStore mencegah caching pada endpoint dinamis dan transaksi checkout (FR-05).
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Writer.Header().Set("Pragma", "no-cache")
		c.Next()
	}
}

// TimeoutContext membungkus context request dengan batas waktu eksekusi (FR-06).
func TimeoutContext(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// NotFound mengembalikan format RFC 7807 terstandarisasi untuk rute yang tidak ditemukan (FR-09).
func NotFound() gin.HandlerFunc {
	return func(c *gin.Context) {
		WriteError(c, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found")
	}
}

// MethodNotAllowed mengembalikan 405 RFC 7807 dengan header Allow (FR-10).
func MethodNotAllowed() gin.HandlerFunc {
	return func(c *gin.Context) {
		WriteError(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "http method not allowed for this resource")
	}
}

// CORS mengatur izin Cross-Origin Resource Sharing secara terkelola dan aman (FR-23).
func CORS(allowedOrigins []string, isDev bool) gin.HandlerFunc {
	originsMap := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originsMap[strings.TrimRight(strings.TrimSpace(o), "/")] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		trimmedOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")

		if origin != "" {
			if originsMap[trimmedOrigin] || (isDev && strings.HasPrefix(origin, "http://localhost:")) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, Idempotency-Key, X-Guest-Token, X-Guest-Session, x-callback-token")
				c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, Idempotency-Replayed, Retry-After")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
