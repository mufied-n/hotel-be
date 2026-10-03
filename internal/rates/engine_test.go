package rates

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestQuote_WeekdayAndWeekend(t *testing.T) {
	// 2026-10-09 = Jumat, 2026-10-10 = Sabtu; 2026-10-11 = Minggu, 12 = Senin
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.25)

	quotes, err := eng.Quote(context.Background(), "std", date("2026-10-09"), date("2026-10-13"))
	if err != nil {
		t.Fatalf("Quote error: %v", err)
	}
	want := []int64{125_000, 125_000, 100_000, 100_000} // Fri, Sat, Sun, Mon
	if len(quotes) != len(want) {
		t.Fatalf("len quotes = %d, want %d", len(quotes), len(want))
	}
	for i, q := range quotes {
		if q.RateMinor != want[i] {
			t.Errorf("quote[%d] (%s) = %d, want %d", i, q.Date.Format("2006-01-02"), q.RateMinor, want[i])
		}
	}
}

func TestQuote_HalfOpen(t *testing.T) {
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.0)
	quotes, _ := eng.Quote(context.Background(), "std", date("2026-10-10"), date("2026-10-11"))
	if len(quotes) != 1 {
		t.Fatalf("1 malam → %d quote, want 1", len(quotes))
	}
}

func TestQuote_UnknownRoomType(t *testing.T) {
	eng := NewEngine(map[string]int64{}, 1.0)
	if _, err := eng.Quote(context.Background(), "xxx", date("2026-10-10"), date("2026-10-11")); !errors.Is(err, ErrUnknownRoomType) {
		t.Fatalf("expected ErrUnknownRoomType, got %v", err)
	}
}

func TestCalculateLockedQuote_Scenarios(t *testing.T) {
	baseRates := map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000, // Superior King
	}
	eng := NewEngine(baseRates, 1.0)
	ctx := context.Background()

	tests := []struct {
		name          string
		req           QuoteRequest
		wantErr       error
		wantSubtotal  int64
		wantBreakfast int64
		wantDiscount  int64
		wantTax       int64
		wantTotal     int64
		wantPlan      string
		wantPolicy    string
	}{
		{
			name: "Standard Room Only (2 nights, 1 room, 2 guests)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "room_only",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"),
				NumRooms:     1,
				NumGuests:    2,
			},
			wantSubtotal:  1_100_000,
			wantBreakfast: 0,
			wantDiscount:  0,
			wantTax:       110_000, // 10% PB1
			wantTotal:     1_210_000,
			wantPlan:      "room_only",
			wantPolicy:    "flexible_48h",
		},
		{
			name: "Bed & Breakfast Package (+100k/person/night)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"), // 2 nights
				NumRooms:     1,
				NumGuests:    2, // 2 guests * 2 nights * 100k = 400k
			},
			wantSubtotal:  1_100_000,
			wantBreakfast: 400_000,
			wantDiscount:  0,
			wantTax:       150_000, // 10% PB1 on 1.5M
			wantTotal:     1_650_000,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "flexible_48h",
		},
		{
			name: "Promo Code OCTOBREAK (15% discount, Non-refundable)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"),
				NumRooms:     1,
				NumGuests:    2,
				PromoCode:    "OCTOBREAK",
			},
			// Subtotal: 1.1M, Breakfast: 400k, Discount: 15% of 1.1M = 165k
			// Taxable: 1.1M + 400k - 165k = 1.335M
			// Tax: 10% of 1.335M = 133.5k
			// Total: 1.335M + 133.5k = 1.468.500
			wantSubtotal:  1_100_000,
			wantBreakfast: 400_000,
			wantDiscount:  165_000,
			wantTax:       133_500,
			wantTotal:     1_468_500,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "non_refundable",
		},
		{
			name: "Invalid promo code returns error",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "room_only",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"),
				PromoCode:    "EXPIRED_PROMO",
			},
			wantErr: ErrInvalidPromoCode,
		},
		{
			name: "Invalid rate plan returns error",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "all_inclusive_vip",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"),
			},
			wantErr: ErrInvalidRatePlan,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := eng.CalculateLockedQuote(ctx, tt.req)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Pricing.RoomSubtotalMinor != tt.wantSubtotal {
				t.Errorf("RoomSubtotalMinor = %d, want %d", q.Pricing.RoomSubtotalMinor, tt.wantSubtotal)
			}
			if q.Pricing.BreakfastChargeMinor != tt.wantBreakfast {
				t.Errorf("BreakfastChargeMinor = %d, want %d", q.Pricing.BreakfastChargeMinor, tt.wantBreakfast)
			}
			if q.Pricing.DiscountMinor != tt.wantDiscount {
				t.Errorf("DiscountMinor = %d, want %d", q.Pricing.DiscountMinor, tt.wantDiscount)
			}
			if q.Pricing.TaxMinor != tt.wantTax {
				t.Errorf("TaxMinor = %d, want %d", q.Pricing.TaxMinor, tt.wantTax)
			}
			if q.Pricing.TotalPriceMinor != tt.wantTotal {
				t.Errorf("TotalPriceMinor = %d, want %d", q.Pricing.TotalPriceMinor, tt.wantTotal)
			}
			if q.RatePlanCode != tt.wantPlan {
				t.Errorf("RatePlanCode = %s, want %s", q.RatePlanCode, tt.wantPlan)
			}
			if q.CancellationCode != tt.wantPolicy {
				t.Errorf("CancellationCode = %s, want %s", q.CancellationCode, tt.wantPolicy)
			}
			if q.ID == "" {
				t.Error("quote ID should not be empty")
			}
		})
	}
}

func TestMemoryQuoteStore_TTLAndExpiry(t *testing.T) {
	store := NewMemoryQuoteStore(50 * time.Millisecond)
	ctx := context.Background()

	q := LockedQuote{
		ID:        "q-test-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(50 * time.Millisecond),
	}
	if err := store.SaveQuote(ctx, q); err != nil {
		t.Fatalf("SaveQuote failed: %v", err)
	}

	// 1. Ambil sebelum expire
	got, err := store.GetQuote(ctx, "q-test-1")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if got.ID != "q-test-1" {
		t.Errorf("expected ID q-test-1, got %s", got.ID)
	}

	// 2. Ambil setelah expire
	time.Sleep(60 * time.Millisecond)
	_, err = store.GetQuote(ctx, "q-test-1")
	if !errors.Is(err, ErrQuoteExpired) {
		t.Errorf("expected ErrQuoteExpired, got %v", err)
	}

	// 3. Not found
	_, err = store.GetQuote(ctx, "non-existent")
	if !errors.Is(err, ErrQuoteNotFound) {
		t.Errorf("expected ErrQuoteNotFound, got %v", err)
	}

	engDefault := NewEngineWithQuoteStore(nil, 1.25, nil)
	if engDefault.QuoteStore() == nil {
		t.Error("expected non-nil QuoteStore from default store")
	}

	customStore := NewMemoryQuoteStore(0) // tests ttl <= 0 branch
	engCustom := NewEngineWithQuoteStore(map[string]int64{"test": 100}, 1.25, customStore)
	if engCustom.QuoteStore() != customStore {
		t.Error("expected custom store returned")
	}
}

func TestEngine_BaseRateSource_DynamicAndFallback(t *testing.T) {
	ctx := context.Background()

	catStore := catalog.NewMemoryStore([]catalog.RoomVariant{
		{
			ID:             "room-priced-1",
			Code:           "priced-1",
			Name:           "Priced Suite",
			BasePriceMinor: 800_000,
		},
		{
			ID:             "room-zero-price",
			Code:           "zero-price",
			Name:           "Free Room Error",
			BasePriceMinor: 0,
		},
	})

	staticBase := map[string]int64{
		"room-priced-1":  500_000, // Should be overridden by catalog (800k)
		"room-static-ok": 600_000, // Should be resolved via fallback
		"room-static-0":  0,       // Should return ErrUnpricedRoomType
	}

	eng := NewEngine(staticBase, 1.25)
	eng.SetBaseRateSource(catStore)

	tests := []struct {
		name       string
		roomID     string
		from       time.Time
		to         time.Time
		wantRate   int64
		wantErr    error
	}{
		{
			name:     "Catalog variant overrides static map",
			roomID:   "room-priced-1",
			from:     date("2026-10-14"), // Wednesday
			to:       date("2026-10-15"),
			wantRate: 800_000,
			wantErr:  nil,
		},
		{
			name:     "Catalog variant with zero price returns ErrUnpricedRoomType",
			roomID:   "room-zero-price",
			from:     date("2026-10-14"),
			to:       date("2026-10-15"),
			wantRate: 0,
			wantErr:  ErrUnpricedRoomType,
		},
		{
			name:     "Fallback to static map when not in catalog",
			roomID:   "room-static-ok",
			from:     date("2026-10-14"),
			to:       date("2026-10-15"),
			wantRate: 600_000,
			wantErr:  nil,
		},
		{
			name:     "Static map with zero rate returns ErrUnpricedRoomType",
			roomID:   "room-static-0",
			from:     date("2026-10-14"),
			to:       date("2026-10-15"),
			wantRate: 0,
			wantErr:  ErrUnpricedRoomType,
		},
		{
			name:     "Completely unknown room returns ErrUnknownRoomType",
			roomID:   "completely-unknown",
			from:     date("2026-10-14"),
			to:       date("2026-10-15"),
			wantRate: 0,
			wantErr:  ErrUnknownRoomType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotes, err := eng.Quote(ctx, tt.roomID, tt.from, tt.to)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(quotes) != 1 {
				t.Fatalf("expected 1 quote, got %d", len(quotes))
			}
			if quotes[0].RateMinor != tt.wantRate {
				t.Errorf("rate = %d, want %d", quotes[0].RateMinor, tt.wantRate)
			}
		})
	}

	// Dynamic update test: update price in catalog store, verify Quote reflects immediately
	_, err := catStore.UpdateVariant(ctx, "room-priced-1", catalog.RoomVariant{
		ID:             "room-priced-1",
		Code:           "priced-1",
		Name:           "Priced Suite Updated",
		BasePriceMinor: 950_000,
		MaxCapacity:    2,
	})
	if err != nil {
		t.Fatalf("failed to update catalog variant: %v", err)
	}

	quotes, err := eng.Quote(ctx, "room-priced-1", date("2026-10-14"), date("2026-10-15"))
	if err != nil {
		t.Fatalf("Quote after update failed: %v", err)
	}
	if quotes[0].RateMinor != 950_000 {
		t.Errorf("expected updated rate 950_000, got %d", quotes[0].RateMinor)
	}

	// Test SetBaseRate explicitly updates static map
	eng.SetBaseRate("room-new-static", 1_200_000)
	quotes, err = eng.Quote(ctx, "room-new-static", date("2026-10-14"), date("2026-10-15"))
	if err != nil {
		t.Fatalf("Quote for room-new-static failed: %v", err)
	}
	if quotes[0].RateMinor != 1_200_000 {
		t.Errorf("expected rate 1_200_000, got %d", quotes[0].RateMinor)
	}
}

func TestEngine_Concurrency(t *testing.T) {
	catStore := catalog.NewMemoryStore([]catalog.RoomVariant{
		{ID: "var-1", Code: "v1", Name: "V1", BasePriceMinor: 500_000},
	})
	eng := NewEngine(nil, 1.25)
	eng.SetBaseRateSource(catStore)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = eng.Quote(ctx, "var-1", date("2026-10-14"), date("2026-10-15"))
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				eng.SetBaseRate("dynamic-var", int64(100_000+id*1000+j))
			}
		}(i)
	}

	wg.Wait()
}
