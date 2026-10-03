package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/gin-gonic/gin"
)

const (
	// IdempotencyReserveTTL membatasi umur reservasi in-progress (BE-R08).
	IdempotencyReserveTTL = 2 * time.Minute
	// IdempotencyResultTTL adalah masa replay respons yang sudah selesai.
	IdempotencyResultTTL = 24 * time.Hour
)

// IdempotencyRecord menyimpan snapshot respons API untuk replay yang aman (BE-G09, IETF draft).
type IdempotencyRecord struct {
	Key          string    `json:"key"`
	RequestHash  string    `json:"request_hash"`
	ResponseCode int       `json:"response_code"`
	ResponseBody string    `json:"response_body"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// InProgress true bila request pemilik key belum menyelesaikan efek sampingnya.
func (r IdempotencyRecord) InProgress() bool { return r.ResponseCode == 0 }

// HashBody menghitung fingerprint SHA-256 dari byte slice.
func HashBody(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// IdempotencyStore adalah kontrak penyimpanan idempotency keys dengan klaim atomik (BE-R08).
type IdempotencyStore interface {
	Reserve(ctx context.Context, key, requestHash string) (rec IdempotencyRecord, acquired bool, err error)
	Complete(ctx context.Context, rec IdempotencyRecord) error
	Release(ctx context.Context, key string) error
}

// ScopeIdempotencyKey scopes an idempotency key to (subject | HTTP method | path | key),
// preventing cross-tenant or cross-endpoint collision (D-01, FR-12).
// The resulting string is a 64-character SHA-256 hex digest, conforming to VARCHAR(64).
func ScopeIdempotencyKey(subject, method, path, key string) string {
	if subject == "" {
		subject = "anonymous"
	}
	return HashBody([]byte(subject + "|" + method + "|" + path + "|" + key))
}

// WithIdempotency executes fn within an atomic idempotency lock if enabled.
// If the key is already acquired by another request, it handles replay or conflict.
// If the action succeeds, it stores the response with context.WithoutCancel (D-10).
// If the action fails, it releases the reservation with context.WithoutCancel.
func WithIdempotency(c *gin.Context, store IdempotencyStore, ff featureflag.Manager, rawKey string, bodyBytes []byte, fn func() (int, []byte, error)) {
	useIdem := rawKey != "" && store != nil && FeatureEnabled(c, ff, "ff_checkout_idempotency")

	if !useIdem {
		code, respBytes, err := fn()
		if err != nil {
			return
		}
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Status(code)
		_, _ = c.Writer.Write(respBytes)
		return
	}

	authCtx := GetAuthContext(c.Request.Context())
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	scopedKey := ScopeIdempotencyKey(authCtx.Subject, c.Request.Method, path, rawKey)
	reqHash := HashBody(bodyBytes)

	// Klaim atomik sebelum efek samping apa pun (BE-R08).
	rec, acquired, err := store.Reserve(c.Request.Context(), scopedKey, reqHash)
	if err != nil {
		HttpErrorCode(c, http.StatusServiceUnavailable, "idempotency store tidak tersedia", "IDEMPOTENCY_UNAVAILABLE")
		return
	}
	if !acquired {
		switch {
		case rec.RequestHash != reqHash:
			HttpErrorCode(c, http.StatusConflict, "idempotency key reused with different request payload", "IDEMPOTENCY_CONFLICT")
		case rec.InProgress():
			c.Header("Retry-After", "1")
			HttpErrorCode(c, http.StatusConflict, "request dengan idempotency key ini masih diproses", "IDEMPOTENCY_IN_PROGRESS")
		default:
			c.Header("Content-Type", "application/json; charset=utf-8")
			c.Header("Idempotency-Replayed", "true")
			c.Status(rec.ResponseCode)
			_, _ = c.Writer.Write([]byte(rec.ResponseBody))
		}
		return
	}

	completed := false
	defer func() {
		if !completed {
			_ = store.Release(context.WithoutCancel(c.Request.Context()), scopedKey)
		}
	}()

	code, respBytes, err := fn()
	if err != nil {
		return
	}

	now := time.Now().UTC()
	if err := store.Complete(context.WithoutCancel(c.Request.Context()), IdempotencyRecord{
		Key:          scopedKey,
		RequestHash:  reqHash,
		ResponseCode: code,
		ResponseBody: string(respBytes),
		CreatedAt:    now,
		ExpiresAt:    now.Add(IdempotencyResultTTL),
	}); err != nil {
		slog.Error("idempotency.complete_failed", "key", scopedKey, "error", err)
	}
	completed = true

	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(code)
	_, _ = c.Writer.Write(respBytes)
}

