package api

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
				if _, ok, _ := store.Reserve(context.Background(), "k-inflight", h); !ok {
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
				_, _, _ = store.Reserve(context.Background(), "k-diff", "other-hash")
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
				if _, ok, _ := store.Reserve(context.Background(), "k-release", "x"); !ok {
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
