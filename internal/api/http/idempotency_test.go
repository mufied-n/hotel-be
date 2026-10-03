package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

// BE-R08: Reserve harus atomik; hanya satu pemanggil yang memperoleh klaim.
func TestMemoryIdempotencyStore_ReserveConcurrent(t *testing.T) {
	store := NewMemoryIdempotencyStore()
	const workers = 20
	var acquired int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, err := store.Reserve(context.Background(), "k-par", "hash-1")
			if err != nil {
				t.Errorf("Reserve err = %v", err)
				return
			}
			if ok {
				atomic.AddInt32(&acquired, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if acquired != 1 {
		t.Fatalf("acquired = %d, want exactly 1", acquired)
	}
}

func TestMemoryIdempotencyStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		steps func(s *MemoryIdempotencyStore) (rec IdempotencyRecord, acquired bool)
		want  struct {
			acquired   bool
			inProgress bool
			hash       string
		}
	}{
		{
			name: "klaim pertama berhasil",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				rec, ok, _ := s.Reserve(ctx, "k", "h1")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: true, inProgress: true, hash: "h1"},
		},
		{
			name: "klaim kedua saat in-progress ditolak",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				_, _, _ = s.Reserve(ctx, "k", "h1")
				rec, ok, _ := s.Reserve(ctx, "k", "h1")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: false, inProgress: true, hash: "h1"},
		},
		{
			name: "hash berbeda terlihat dari record yang ada",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				_, _, _ = s.Reserve(ctx, "k", "h1")
				rec, ok, _ := s.Reserve(ctx, "k", "h2")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: false, inProgress: true, hash: "h1"},
		},
		{
			name: "setelah Complete record berisi respons",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				_, _, _ = s.Reserve(ctx, "k", "h1")
				_ = s.Complete(ctx, IdempotencyRecord{Key: "k", RequestHash: "h1", ResponseCode: 201, ResponseBody: "{}", ExpiresAt: time.Now().Add(time.Hour)})
				rec, ok, _ := s.Reserve(ctx, "k", "h1")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: false, inProgress: false, hash: "h1"},
		},
		{
			name: "Release membebaskan key",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				_, _, _ = s.Reserve(ctx, "k", "h1")
				_ = s.Release(ctx, "k")
				rec, ok, _ := s.Reserve(ctx, "k", "h1")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: true, inProgress: true, hash: "h1"},
		},
		{
			name: "reservasi kedaluwarsa dapat diambil alih",
			steps: func(s *MemoryIdempotencyStore) (IdempotencyRecord, bool) {
				_, _, _ = s.Reserve(ctx, "k", "h1")
				s.mu.Lock()
				r := s.records["k"]
				r.ExpiresAt = time.Now().Add(-time.Second)
				s.records["k"] = r
				s.mu.Unlock()
				rec, ok, _ := s.Reserve(ctx, "k", "h2")
				return rec, ok
			},
			want: struct {
				acquired   bool
				inProgress bool
				hash       string
			}{acquired: true, inProgress: true, hash: "h2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := tc.steps(NewMemoryIdempotencyStore())
			if ok != tc.want.acquired {
				t.Errorf("acquired = %v, want %v", ok, tc.want.acquired)
			}
			if rec.InProgress() != tc.want.inProgress {
				t.Errorf("InProgress = %v, want %v", rec.InProgress(), tc.want.inProgress)
			}
			if rec.RequestHash != tc.want.hash {
				t.Errorf("hash = %q, want %q", rec.RequestHash, tc.want.hash)
			}
		})
	}
}

// failingIdempotencyStore mensimulasikan DB idempotency tidak tersedia.
type failingIdempotencyStore struct{ MemoryIdempotencyStore }

func (f *failingIdempotencyStore) Reserve(context.Context, string, string) (IdempotencyRecord, bool, error) {
	return IdempotencyRecord{}, false, errors.New("db down")
}

func postBooking(router http.Handler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// BE-R08: kontrak handler createBooking terhadap klaim idempotency atomik.
func TestCreateBooking_IdempotencyReservation(t *testing.T) {
	store := NewMemoryIdempotencyStore()
	router, _ := setupTestRouterWithStore(store)
	quoteID := mustQuoteID(t, router, "std", 1, 1)
	body := func(quote string) string {
		return fmt.Sprintf(`{"quote_id":%q,"terms_accepted":true,"privacy_accepted":true,"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"Budi","guest_email":"budi@example.com"}`, quote)
	}

	tests := []struct {
		name       string
		run        func(t *testing.T) *httptest.ResponseRecorder
		wantStatus int
		wantCode   string
		wantHeader map[string]string
	}{
		{
			name: "request pertama sukses lalu replay",
			run: func(t *testing.T) *httptest.ResponseRecorder {
				if rec := postBooking(router, "k-replay", body(quoteID)); rec.Code != http.StatusCreated {
					t.Fatalf("first = %d: %s", rec.Code, rec.Body.String())
				}
				return postBooking(router, "k-replay", body(quoteID))
			},
			wantStatus: http.StatusCreated,
			wantHeader: map[string]string{"Idempotency-Replayed": "true"},
		},
		{
			name: "request sedang berjalan ditolak 409 in-progress",
			run: func(t *testing.T) *httptest.ResponseRecorder {
				h := hashBody([]byte(body(quoteID)))
				scopedKey := ScopeIdempotencyKey("anonymous", http.MethodPost, "/api/v1/bookings", "k-inflight")
				if _, ok, _ := store.Reserve(context.Background(), scopedKey, h); !ok {
					t.Fatal("setup reserve failed")
				}
				return postBooking(router, "k-inflight", body(quoteID))
			},
			wantStatus: http.StatusConflict,
			wantCode:   "IDEMPOTENCY_IN_PROGRESS",
			wantHeader: map[string]string{"Retry-After": "1"},
		},
		{
			name: "hash beda saat in-progress tetap conflict",
			run: func(t *testing.T) *httptest.ResponseRecorder {
				scopedKey := ScopeIdempotencyKey("anonymous", http.MethodPost, "/api/v1/bookings", "k-diff")
				_, _, _ = store.Reserve(context.Background(), scopedKey, "other-hash")
				return postBooking(router, "k-diff", body(quoteID))
			},
			wantStatus: http.StatusConflict,
			wantCode:   "IDEMPOTENCY_CONFLICT",
		},
		{
			name: "create gagal melepas key sehingga retry bisa jalan",
			run: func(t *testing.T) *httptest.ResponseRecorder {
				bad := body("")
				if rec := postBooking(router, "k-release", bad); rec.Code != http.StatusBadRequest {
					t.Fatalf("bad create = %d, want 400", rec.Code)
				}
				scopedKey := ScopeIdempotencyKey("anonymous", http.MethodPost, "/api/v1/bookings", "k-release")
				if _, ok, _ := store.Reserve(context.Background(), scopedKey, "x"); !ok {
					t.Fatal("key tidak dilepas setelah create gagal")
				}
				return httptest.NewRecorder()
			},
			wantStatus: http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := tc.run(t)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" && !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Errorf("body %s tidak memuat code %s", rec.Body.String(), tc.wantCode)
			}
			for k, v := range tc.wantHeader {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestCreateBooking_IdempotencyStoreUnavailable(t *testing.T) {
	router, _ := setupTestRouterWithStore(&failingIdempotencyStore{*NewMemoryIdempotencyStore()})
	rec := postBooking(router, "k-down", `{"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"B","guest_email":"b@example.com"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_UNAVAILABLE") {
		t.Fatalf("got %d %s, want 503 IDEMPOTENCY_UNAVAILABLE", rec.Code, rec.Body.String())
	}
}

// BE-R06: create tanpa quote_id ditolak 400 QUOTE_REQUIRED dan tidak mengubah state.
func TestCreateBooking_QuoteRequired(t *testing.T) {
	router, _ := setupTestRouter()
	rec := postBooking(router, "", `{"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"B","guest_email":"b@example.com","terms_accepted":true,"privacy_accepted":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "QUOTE_REQUIRED") {
		t.Fatalf("got %d %s, want 400 QUOTE_REQUIRED", rec.Code, rec.Body.String())
	}
}

// Quote yang sudah menjadi booking ditolak 409 QUOTE_ALREADY_USED (satu quote = satu booking).
func TestCreateBooking_QuoteAlreadyUsed(t *testing.T) {
	router, txMock := setupTestRouter()
	quoteID := mustQuoteID(t, router, "std", 1, 1)
	txMock.insertErr = booking.ErrQuoteAlreadyUsed
	body := fmt.Sprintf(`{"quote_id":%q,"terms_accepted":true,"privacy_accepted":true,"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"B","guest_email":"b@example.com"}`, quoteID)
	rec := postBooking(router, "", body)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "QUOTE_ALREADY_USED") {
		t.Fatalf("got %d %s, want 409 QUOTE_ALREADY_USED", rec.Code, rec.Body.String())
	}
}

func TestScopeIdempotencyKey_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		method   string
		path     string
		key      string
		validate func(t *testing.T, key string)
	}{
		{
			name:    "length is exactly 64 characters hex",
			subject: "staff:receptionist",
			method:  http.MethodPost,
			path:    "/api/v1/bookings",
			key:     "ik-test-1",
			validate: func(t *testing.T, k string) {
				if len(k) != 64 {
					t.Errorf("len(key) = %d, want 64", len(k))
				}
			},
		},
		{
			name:    "empty subject defaults to anonymous",
			subject: "",
			method:  http.MethodPost,
			path:    "/api/v1/bookings",
			key:     "ik-test-2",
			validate: func(t *testing.T, k string) {
				want := ScopeIdempotencyKey("anonymous", http.MethodPost, "/api/v1/bookings", "ik-test-2")
				if k != want {
					t.Errorf("key = %q, want %q", k, want)
				}
			},
		},
		{
			name:    "different subjects yield different keys (tenant isolation)",
			subject: "staff:user-1",
			method:  http.MethodPost,
			path:    "/api/v1/bookings",
			key:     "ik-shared",
			validate: func(t *testing.T, k string) {
				other := ScopeIdempotencyKey("staff:user-2", http.MethodPost, "/api/v1/bookings", "ik-shared")
				if k == other {
					t.Errorf("expected different keys for different subjects, got identical %q", k)
				}
			},
		},
		{
			name:    "different paths yield different keys (endpoint isolation)",
			subject: "anonymous",
			method:  http.MethodPost,
			path:    "/api/v1/bookings",
			key:     "ik-shared",
			validate: func(t *testing.T, k string) {
				other := ScopeIdempotencyKey("anonymous", http.MethodPost, "/api/v1/other", "ik-shared")
				if k == other {
					t.Errorf("expected different keys for different paths, got identical %q", k)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := ScopeIdempotencyKey(tc.subject, tc.method, tc.path, tc.key)
			tc.validate(t, k)
		})
	}
}

// TestCreateBooking_TenantIsolation memverifikasi bahwa dua subjek berbeda
// yang menggunakan Idempotency-Key yang persis sama TIDAK saling bertabrakan (D-01).
func TestCreateBooking_TenantIsolation(t *testing.T) {
	store := NewMemoryIdempotencyStore()
	router, _ := setupTestRouterWithStore(store)
	quoteID1 := mustQuoteID(t, router, "std", 1, 1)
	quoteID2 := mustQuoteID(t, router, "std", 1, 1)

	bodyA := fmt.Sprintf(`{"quote_id":%q,"terms_accepted":true,"privacy_accepted":true,"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"Alice","guest_email":"alice@example.com"}`, quoteID1)
	bodyB := fmt.Sprintf(`{"quote_id":%q,"terms_accepted":true,"privacy_accepted":true,"room_type_id":"std","check_in":"2026-10-10","check_out":"2026-10-12","num_rooms":1,"num_guests":1,"guest_name":"Bob","guest_email":"bob@example.com"}`, quoteID2)

	const sharedKey = "shared-ik-key-123"

	// Request 1: oleh staff receptionist
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(bodyA))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", sharedKey)
	req1.Header.Set("Authorization", "Bearer receptionist")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("req1 status = %d (body: %s), want 201", rec1.Code, rec1.Body.String())
	}

	// Request 2: oleh tamu publik (anonymous) dengan Idempotency-Key yang SAMA
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(bodyB))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", sharedKey)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	// Tidak boleh 409 conflict ataupun 200 replay milik req1!
	if rec2.Code != http.StatusCreated {
		t.Fatalf("req2 status = %d (body: %s), want 201 Created (isolated from staff)", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Idempotency-Replayed") == "true" {
		t.Fatal("req2 should not be treated as a replay of req1")
	}
}

