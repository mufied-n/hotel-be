package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// fakeRedisCmdable adalah mock minimal untuk redis.Cmdable berbasis map.
type fakeRedisCmdable struct {
	redis.Cmdable
	mu      sync.Mutex
	data    map[string]string
	ttls    map[string]time.Duration
	failSet bool
	failGet bool
}

func newFakeRedisCmdable() *fakeRedisCmdable {
	return &fakeRedisCmdable{
		data: make(map[string]string),
		ttls: make(map[string]time.Duration),
	}
}

func (f *fakeRedisCmdable) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSet {
		cmd.SetErr(errors.New("connection refused: dial tcp valkey:6379"))
		return cmd
	}
	switch v := value.(type) {
	case string:
		f.data[key] = v
	case []byte:
		f.data[key] = string(v)
	default:
		f.data[key] = fmt.Sprint(v)
	}
	f.ttls[key] = expiration
	cmd.SetVal("OK")
	return cmd
}

func (f *fakeRedisCmdable) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		cmd.SetErr(errors.New("connection reset by peer"))
		return cmd
	}
	val, ok := f.data[key]
	if !ok {
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd.SetVal(val)
	return cmd
}

func TestValkeyQuoteStore_TableDriven(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		setupStore    func() *ValkeyQuoteStore
		op            func(s *ValkeyQuoteStore) error
		wantErr       bool
		expectedErrIs error
	}{
		{
			name: "Nil client returns error on SaveQuote",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(nil, 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				return s.SaveQuote(ctx, LockedQuote{ID: "test-id"})
			},
			wantErr: true,
		},
		{
			name: "Nil client returns error on GetQuote",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(nil, 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "test-id")
				return err
			},
			wantErr: true,
		},
		{
			name: "Save quote with empty ID returns error",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(newFakeRedisCmdable(), 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				return s.SaveQuote(ctx, LockedQuote{ID: ""})
			},
			wantErr: true,
		},
		{
			name: "Get quote with empty ID returns ErrQuoteNotFound",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(newFakeRedisCmdable(), 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "")
				return err
			},
			wantErr:       true,
			expectedErrIs: ErrQuoteNotFound,
		},
		{
			name: "Save and Get successful round-trip with dynamic TTL",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(newFakeRedisCmdable(), 15*time.Minute)
			},
			op: func(s *ValkeyQuoteStore) error {
				now := time.Now()
				q := LockedQuote{
					ID:           "quote-round-trip-123",
					CreatedAt:    now,
					ExpiresAt:    now.Add(10 * time.Minute),
					RoomTypeID:   "room-001",
					RatePlanCode: "bed_and_breakfast",
					NumRooms:     1,
					NumGuests:    2,
					Pricing: PricingBreakdown{
						RoomSubtotalMinor:    1_000_000,
						BreakfastChargeMinor: 200_000,
						TaxMinor:             120_000,
						TotalPriceMinor:      1_320_000,
						Currency:             "IDR",
					},
				}
				if err := s.SaveQuote(ctx, q); err != nil {
					return err
				}
				retrieved, err := s.GetQuote(ctx, "quote-round-trip-123")
				if err != nil {
					return err
				}
				if retrieved.ID != q.ID || retrieved.Pricing.TotalPriceMinor != q.Pricing.TotalPriceMinor {
					return fmt.Errorf("retrieved quote mismatch: got %+v, want %+v", retrieved, q)
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "Get non-existent key returns ErrQuoteNotFound",
			setupStore: func() *ValkeyQuoteStore {
				return NewValkeyQuoteStore(newFakeRedisCmdable(), 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "non-existent-key")
				return err
			},
			wantErr:       true,
			expectedErrIs: ErrQuoteNotFound,
		},
		{
			name: "Get expired quote returns ErrQuoteExpired",
			setupStore: func() *ValkeyQuoteStore {
				fake := newFakeRedisCmdable()
				s := NewValkeyQuoteStore(fake, 0)
				past := time.Now().Add(-5 * time.Minute)
				expiredQ := LockedQuote{
					ID:        "expired-quote-1",
					CreatedAt: past.Add(-15 * time.Minute),
					ExpiresAt: past,
				}
				payload, _ := json.Marshal(expiredQ)
				fake.data[DefaultQuotePrefix+"expired-quote-1"] = string(payload)
				return s
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "expired-quote-1")
				return err
			},
			wantErr:       true,
			expectedErrIs: ErrQuoteExpired,
		},
		{
			name: "Get corrupted JSON returns unmarshal error",
			setupStore: func() *ValkeyQuoteStore {
				fake := newFakeRedisCmdable()
				s := NewValkeyQuoteStore(fake, 0)
				fake.data[DefaultQuotePrefix+"corrupt-quote"] = "{broken json string..."
				return s
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "corrupt-quote")
				return err
			},
			wantErr: true,
		},
		{
			name: "Redis Set error propagates on SaveQuote",
			setupStore: func() *ValkeyQuoteStore {
				fake := newFakeRedisCmdable()
				fake.failSet = true
				return NewValkeyQuoteStore(fake, 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				return s.SaveQuote(ctx, LockedQuote{ID: "quote-fail-set"})
			},
			wantErr: true,
		},
		{
			name: "Redis Get error propagates on GetQuote",
			setupStore: func() *ValkeyQuoteStore {
				fake := newFakeRedisCmdable()
				fake.failGet = true
				return NewValkeyQuoteStore(fake, 0)
			},
			op: func(s *ValkeyQuoteStore) error {
				_, err := s.GetQuote(ctx, "quote-fail-get")
				return err
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setupStore()
			err := tt.op(s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if tt.expectedErrIs != nil && !errors.Is(err, tt.expectedErrIs) {
				t.Fatalf("expected errors.Is(%v), got: %v", tt.expectedErrIs, err)
			}
		})
	}
}

// failingQuoteStore untuk menguji fail-closed persistence failure
type failingQuoteStore struct{}

func (f *failingQuoteStore) SaveQuote(_ context.Context, _ LockedQuote) error {
	return errors.New("valkey cluster unavailable (ECONNREFUSED)")
}

func (f *failingQuoteStore) GetQuote(_ context.Context, _ string) (LockedQuote, error) {
	return LockedQuote{}, ErrQuoteNotFound
}

func TestEngine_FailClosedOnSaveQuoteError(t *testing.T) {
	base := map[string]int64{
		"room-001": 500_000,
	}
	eng := NewEngine(base, 1.25)
	eng.SetQuoteStore(&failingQuoteStore{})

	// Pastikan QuoteStore() mengembalikan failingQuoteStore
	if _, ok := eng.QuoteStore().(*failingQuoteStore); !ok {
		t.Fatal("expected QuoteStore() to be failingQuoteStore")
	}

	req := QuoteRequest{
		RoomTypeID:   "room-001",
		RatePlanCode: RatePlanRoomOnly,
		CheckIn:      time.Now().AddDate(0, 0, 1),
		CheckOut:     time.Now().AddDate(0, 0, 2),
		NumRooms:     1,
		NumGuests:    2,
	}

	lq, err := eng.CalculateLockedQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on save quote failure, got nil")
	}
	if !errors.Is(err, ErrSaveQuoteFailed) {
		t.Fatalf("expected errors.Is(ErrSaveQuoteFailed), got %v", err)
	}
	if lq.ID != "" {
		t.Fatalf("expected empty LockedQuote ID, got %s", lq.ID)
	}
}

func TestAPI_QuoteHandler_InternalErrorOnSaveFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	base := map[string]int64{
		"room-test-01": 500_000,
	}
	eng := NewEngine(base, 1.0)
	eng.SetQuoteStore(&failingQuoteStore{})

	router := gin.New()
	router.POST("/api/v1/quotes", func(c *gin.Context) {
		// Mock implementasi handler router yang menangani error fail-closed
		var in struct {
			RoomTypeID   string `json:"room_type_id"`
			RatePlanCode string `json:"rate_plan_code"`
			CheckIn      string `json:"check_in"`
			CheckOut     string `json:"check_out"`
			NumRooms     int    `json:"num_rooms"`
			NumGuests    int    `json:"num_guests"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
			return
		}
		ci, _ := time.Parse("2006-01-02", in.CheckIn)
		co, _ := time.Parse("2006-01-02", in.CheckOut)
		q, err := eng.CalculateLockedQuote(c.Request.Context(), QuoteRequest{
			RoomTypeID:   in.RoomTypeID,
			RatePlanCode: in.RatePlanCode,
			CheckIn:      ci,
			CheckOut:     co,
			NumRooms:     in.NumRooms,
			NumGuests:    in.NumGuests,
		})
		if errors.Is(err, ErrSaveQuoteFailed) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "gagal menyimpan kuotasi harga",
				"status": 500,
				"code":   "INTERNAL_ERROR",
			})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, q)
	})

	body := `{"room_type_id":"room-test-01","rate_plan_code":"room_only","check_in":"2026-10-10","check_out":"2026-10-11","num_rooms":1,"num_guests":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected response body to contain INTERNAL_ERROR, got: %s", w.Body.String())
	}
}
