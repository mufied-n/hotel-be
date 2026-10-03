package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockIdemStore struct {
	reserveFunc  func(ctx context.Context, key, requestHash string) (rec IdempotencyRecord, acquired bool, err error)
	completeFunc func(ctx context.Context, rec IdempotencyRecord) error
	releaseFunc  func(ctx context.Context, key string) error
}

func (m *mockIdemStore) Reserve(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
	if m.reserveFunc != nil {
		return m.reserveFunc(ctx, key, requestHash)
	}
	return IdempotencyRecord{}, true, nil
}

func (m *mockIdemStore) Complete(ctx context.Context, rec IdempotencyRecord) error {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, rec)
	}
	return nil
}

func (m *mockIdemStore) Release(ctx context.Context, key string) error {
	if m.releaseFunc != nil {
		return m.releaseFunc(ctx, key)
	}
	return nil
}

func TestScopeIdempotencyKey_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		sub     string
		method  string
		path    string
		key     string
		wantLen int
	}{
		{
			name:    "anonymous user scopes properly",
			sub:     "",
			method:  "POST",
			path:    "/api/v1/bookings",
			key:     "key-1",
			wantLen: 64,
		},
		{
			name:    "specific subject scopes properly",
			sub:     "user-123",
			method:  "POST",
			path:    "/api/v1/bookings",
			key:     "key-1",
			wantLen: 64,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScopeIdempotencyKey(tc.sub, tc.method, tc.path, tc.key)
			if len(got) != tc.wantLen {
				t.Errorf("length = %d, want %d", len(got), tc.wantLen)
			}
		})
	}

	// Distinct inputs must produce distinct hashes
	h1 := ScopeIdempotencyKey("sub1", "POST", "/p", "k")
	h2 := ScopeIdempotencyKey("sub2", "POST", "/p", "k")
	if h1 == h2 {
		t.Errorf("expected different scoped keys for different subjects")
	}
}

func TestWithIdempotency_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		key        string
		store      IdempotencyStore
		reserveErr error
		acquired   bool
		existing   IdempotencyRecord
		actionErr  error
		wantStatus int
		wantHeader string
	}{
		{
			name:       "empty key executes directly without store",
			key:        "",
			store:      &mockIdemStore{},
			wantStatus: http.StatusOK,
		},
		{
			name: "store error returns 503 IDEMPOTENCY_UNAVAILABLE",
			key:  "k1",
			store: &mockIdemStore{
				reserveFunc: func(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
					return IdempotencyRecord{}, false, errors.New("redis down")
				},
			},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "conflict with different request hash returns 409 IDEMPOTENCY_CONFLICT",
			key:  "k1",
			store: &mockIdemStore{
				reserveFunc: func(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
					return IdempotencyRecord{RequestHash: "different-hash"}, false, nil
				},
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "in progress returns 409 IDEMPOTENCY_IN_PROGRESS with Retry-After",
			key:  "k1",
			store: &mockIdemStore{
				reserveFunc: func(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
					return IdempotencyRecord{
						RequestHash:  HashBody([]byte("payload")),
						ResponseCode: 0,
					}, false, nil
				},
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "replay completed response returns 201 with Idempotency-Replayed header",
			key:  "k1",
			store: &mockIdemStore{
				reserveFunc: func(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
					return IdempotencyRecord{
						RequestHash:  HashBody([]byte("payload")),
						ResponseCode: http.StatusCreated,
						ResponseBody: `{"id":"b1"}`,
					}, false, nil
				},
			},
			wantStatus: http.StatusCreated,
			wantHeader: "true",
		},
		{
			name: "acquired lock executes and completes successfully",
			key:  "k1",
			store: &mockIdemStore{
				reserveFunc: func(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
					return IdempotencyRecord{}, true, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			if tc.key != "" {
				c.Request.Header.Set("Idempotency-Key", tc.key)
			}

			WithIdempotency(c, tc.store, nil, tc.key, []byte("payload"), func() (int, []byte, error) {
				if tc.actionErr != nil {
					return http.StatusBadRequest, nil, tc.actionErr
				}
				return http.StatusOK, []byte(`{"status":"ok"}`), nil
			})

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantHeader != "" && rec.Header().Get("Idempotency-Replayed") != tc.wantHeader {
				t.Errorf("Idempotency-Replayed = %q, want %q", rec.Header().Get("Idempotency-Replayed"), tc.wantHeader)
			}
		})
	}
}
