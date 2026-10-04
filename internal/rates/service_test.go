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
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.0)
	_, err := eng.Quote(context.Background(), "unknown", date("2026-10-10"), date("2026-10-11"))
	if !errors.Is(err, ErrUnknownRoomType) {
		t.Errorf("want ErrUnknownRoomType, got %v", err)
	}
}

func TestCalculateLockedQuote_Scenarios(t *testing.T) {
	baseRates := map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000, // Superior King
	}
	eng := NewEngine(baseRates, 1.0)
	promoStore := NewMemoryPromoStore()
	_, _ = promoStore.CreateCampaign(context.Background(), PromoCampaign{
		Code:          "OCTOBREAK",
		Name:          "October Break Flash Sale",
		DiscountType:  "PERCENT",
		DiscountValue: 15,
		MinStayNights: 1,
		QuotaTotal:    1000,
		ValidFrom:     time.Now().Add(-24 * time.Hour),
		ValidTo:       time.Now().Add(24 * 30 * time.Hour),
		IsActive:      true,
	})
	eng.SetPromoStore(promoStore)
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
			wantTax:       150_000, // 10% of 1.5M
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
		{
			name: "Multi-room: 2 rooms, 4 adults, 2 nights (BE-R10 no double counting)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"), // 2 nights
				NumRooms:     2,
				Adults:       4,
				Children:     0,
			},
			wantSubtotal:  2_200_000,
			wantBreakfast: 800_000, // 4 adults * 2 nights * 100k (not multiplied by 2 rooms!)
			wantDiscount:  0,
			wantTax:       300_000, // 10% of 3M
			wantTotal:     3_300_000,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "flexible_48h",
		},
		{
			name: "Multi-room: 3 rooms, 6 adults, 1 night (BE-R10)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-11"), // 1 night
				NumRooms:     3,
				Adults:       6,
				Children:     0,
			},
			wantSubtotal:  1_650_000, // 3 * 550k
			wantBreakfast: 600_000,   // 6 adults * 1 night * 100k
			wantDiscount:  0,
			wantTax:       225_000, // 10% of 2.25M
			wantTotal:     2_475_000,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "flexible_48h",
		},
		{
			name: "Family with Child Tiers: 2 adults, toddler (4yo), child (8yo), teen (14yo) (BE-R10)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-11"), // 1 night
				NumRooms:     1,
				Adults:       2,
				Children:     3,
				ChildAges:    []int{4, 8, 14}, // 4yo: free, 8yo: 50k, 14yo: 100k
			},
			wantSubtotal:  550_000,
			// Breakfast: (2 * 100k) + (0) + (50k) + (100k) = 350k
			wantBreakfast: 350_000,
			wantDiscount:  0,
			wantTax:       90_000, // 10% of 900k
			wantTotal:     990_000,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "flexible_48h",
		},
		{
			name: "Children without child_ages fallback to 50% rate (BE-R10)",
			req: QuoteRequest{
				RoomTypeID:   "01900000-0000-7000-8000-000000000001",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-11"), // 1 night
				NumRooms:     1,
				Adults:       2,
				Children:     2, // No child_ages -> 2 * 50k = 100k
			},
			wantSubtotal:  550_000,
			// Breakfast: (2 * 100k) + (2 * 50k) = 300k
			wantBreakfast: 300_000,
			wantDiscount:  0,
			wantTax:       85_000, // 10% of 850k
			wantTotal:     935_000,
			wantPlan:      "bed_and_breakfast",
			wantPolicy:    "flexible_48h",
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

func TestEngine_QuoteStoreConfiguration(t *testing.T) {
	engDefault := NewEngine(nil, 1.25)
	if engDefault.QuoteStore() == nil {
		t.Error("expected non-nil QuoteStore from default store")
	}

	svc := NewService(map[string]int64{"test": 100}, 1.25)
	if svc == nil {
		t.Error("expected non-nil Service")
	}

	customStore := NewMemoryQuoteStore(0) // tests ttl <= 0 branch
	engCustom := NewEngineWithQuoteStore(map[string]int64{"test": 100}, 1.25, customStore)
	if engCustom.QuoteStore() != customStore {
		t.Error("expected custom store returned")
	}

	newStore := NewMemoryQuoteStore(10 * time.Minute)
	engCustom.SetQuoteStore(newStore)
	if engCustom.QuoteStore() != newStore {
		t.Error("expected updated quote store")
	}
}

func TestEngine_BaseRateSource_DynamicAndFallback(t *testing.T) {
	ctx := context.Background()

	catStore := catalog.NewMemoryStore([]catalog.RoomVariant{
		{
			ID:             "room-priced-1",
			Code:           "priced-1",
			BasePriceMinor: 800_000,
		},
		{
			ID:             "room-unpriced-2",
			Code:           "unpriced-2",
			BasePriceMinor: 0,
		},
	})

	staticRates := map[string]int64{
		"room-fallback-3": 600_000,
		"room-zero-4":     0,
	}

	eng := NewEngine(staticRates, 1.0)
	eng.SetBaseRateSource(catStore)

	from := date("2026-10-10")
	to := date("2026-10-12")

	tests := []struct {
		name       string
		roomTypeID string
		wantRate   int64
		wantErr    error
	}{
		{
			name:       "Catalog variant overrides static map",
			roomTypeID: "room-priced-1",
			wantRate:   800_000,
			wantErr:    nil,
		},
		{
			name:       "Catalog variant with zero price returns ErrUnpricedRoomType",
			roomTypeID: "room-unpriced-2",
			wantErr:    ErrUnpricedRoomType,
		},
		{
			name:       "Fallback to static map when not in catalog",
			roomTypeID: "room-fallback-3",
			wantRate:   600_000,
			wantErr:    nil,
		},
		{
			name:       "Static map with zero rate returns ErrUnpricedRoomType",
			roomTypeID: "room-zero-4",
			wantErr:    ErrUnpricedRoomType,
		},
		{
			name:       "Completely unknown room returns ErrUnknownRoomType",
			roomTypeID: "completely-unknown",
			wantErr:    ErrUnknownRoomType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotes, err := eng.Quote(ctx, tt.roomTypeID, from, to)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(quotes) != 2 {
				t.Fatalf("expected 2 quotes, got %d", len(quotes))
			}
			if quotes[0].RateMinor != tt.wantRate {
				t.Errorf("expected rate %d, got %d", tt.wantRate, quotes[0].RateMinor)
			}
		})
	}
}

func TestEngine_Concurrency(t *testing.T) {
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.25)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := QuoteRequest{
				RoomTypeID:   "std",
				RatePlanCode: "bed_and_breakfast",
				CheckIn:      date("2026-10-10"),
				CheckOut:     date("2026-10-12"),
				NumRooms:     1,
				NumGuests:    2,
			}
			q, err := eng.CalculateLockedQuote(ctx, req)
			if err != nil {
				t.Errorf("concurrency error: %v", err)
			}
			if q.Pricing.TotalPriceMinor <= 0 {
				t.Errorf("invalid total price: %d", q.Pricing.TotalPriceMinor)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================================
// CALENDAR & RESTRICTIONS TESTS
// ============================================================================

type mockCalendarStore struct {
	overrides []CalendarOverride
	bulkErr   error
}

func (m *mockCalendarStore) GetCalendar(_ context.Context, _, _ time.Time, _ string, _ *string) ([]CalendarOverride, error) {
	return m.overrides, nil
}

func (m *mockCalendarStore) BulkUpsertOverrides(_ context.Context, _ BulkCalendarUpdateRequest) (int64, error) {
	if m.bulkErr != nil {
		return 0, m.bulkErr
	}
	return int64(len(m.overrides)), nil
}

func TestCalendar_OverridesAndRestrictions(t *testing.T) {
	date1 := time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	date3 := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	roomID := "01900000-0000-7000-8000-000000000001"
	overridePrice := int64(2_000_000)

	tests := []struct {
		name       string
		overrides  []CalendarOverride
		from       time.Time
		to         time.Time
		wantErr    error
		wantCount  int
		wantRate0  int64
		checkAvail bool
		wantReason string
	}{
		{
			name: "Normal rate without stop-sell applies override price",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, PriceOverrideIDR: &overridePrice},
				{Date: date2, RoomTypeID: roomID, PriceOverrideIDR: &overridePrice},
			},
			from:       date1,
			to:         date3,
			wantErr:    nil,
			wantCount:  2,
			wantRate0:  2_000_000,
			checkAvail: true,
			wantReason: "AVAILABLE",
		},
		{
			name: "Stop-sell on arrival date returns ErrStopSellApplied",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, IsStopSell: true},
			},
			from:       date1,
			to:         date3,
			wantErr:    ErrStopSellApplied,
			checkAvail: true,
			wantReason: "STOP_SELL",
		},
		{
			name: "Closed to Arrival (CTA) on arrival date returns ErrClosedToArrival",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, IsCTA: true},
			},
			from:       date1,
			to:         date3,
			wantErr:    ErrClosedToArrival,
			checkAvail: true,
			wantReason: "CLOSED_TO_ARRIVAL",
		},
		{
			name: "Closed to Departure (CTD) on departure date returns ErrClosedToDeparture",
			overrides: []CalendarOverride{
				{Date: date3, RoomTypeID: roomID, IsCTD: true},
			},
			from:       date1,
			to:         date3,
			wantErr:    ErrClosedToDeparture,
			checkAvail: true,
			wantReason: "CLOSED_TO_DEPARTURE",
		},
		{
			name: "Minimum Length of Stay (MinLOS) violation returns ErrMinLengthOfStay",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, MinLOS: 3},
			},
			from:       date1,
			to:         date3, // 2 nights
			wantErr:    ErrMinLengthOfStay,
			checkAvail: true,
			wantReason: "MIN_LENGTH_OF_STAY_NOT_MET",
		},
		{
			name: "Maximum Length of Stay (MaxLOS) violation returns ErrMaxLengthOfStay",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, MaxLOS: 1},
			},
			from:       date1,
			to:         date3, // 2 nights > 1
			wantErr:    ErrMaxLengthOfStay,
			checkAvail: true,
			wantReason: "MAX_LENGTH_OF_STAY_EXCEEDED",
		},
		{
			name: "Fallback to base price when PriceOverrideIDR is nil",
			overrides: []CalendarOverride{
				{Date: date1, RoomTypeID: roomID, PriceOverrideIDR: nil},
			},
			from:       date1,
			to:         date2,
			wantErr:    nil,
			wantCount:  1,
			wantRate0:  1_000_000, // Static base
			checkAvail: true,
			wantReason: "AVAILABLE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockCalendarStore{overrides: tc.overrides}
			eng := NewEngineWithQuoteStore(map[string]int64{roomID: 1_000_000}, 1.25, nil)
			eng.SetCalendarStore(store)

			quotes, err := eng.Quote(context.Background(), roomID, tc.from, tc.to)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Quote() err = %v, want %v", err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Quote() unexpected err = %v", err)
				}
				if len(quotes) != tc.wantCount {
					t.Fatalf("Quote() len = %d, want %d", len(quotes), tc.wantCount)
				}
				if quotes[0].RateMinor != tc.wantRate0 {
					t.Fatalf("Quote()[0].RateMinor = %d, want %d", quotes[0].RateMinor, tc.wantRate0)
				}
			}

			if tc.checkAvail {
				stayNights := int(tc.to.Sub(tc.from).Hours() / 24)
				avail, reason, _, err := eng.EvaluateAvailability(context.Background(), roomID, tc.from, tc.to, stayNights)
				if err != nil && tc.wantErr == nil {
					t.Fatalf("EvaluateAvailability() unexpected err = %v", err)
				}
				if reason != tc.wantReason {
					t.Fatalf("EvaluateAvailability() reason = %q, want %q", reason, tc.wantReason)
				}
				if tc.wantErr == nil && !avail {
					t.Fatalf("EvaluateAvailability() avail = false, want true")
				}
			}
		})
	}
}

func TestEngine_CalendarGettersAndRestrictions(t *testing.T) {
	store := NewMemoryCalendarStore()
	eng := NewEngineWithQuoteStore(nil, 1.25, nil)
	eng.SetBaseRate("new-room", 1_000_000) // triggers e.base == nil branch
	eng.SetBaseRate("new-room", 1_200_000) // triggers e.base != nil branch
	eng.SetCalendarStore(store)
	if eng.CalendarStore() != store {
		t.Fatalf("CalendarStore() mismatch")
	}
	pStore := NewMemoryPromoStore()
	eng.SetPromoStore(pStore)
	if eng.PromoStore() != pStore {
		t.Fatalf("PromoStore() mismatch")
	}

	// Test EvaluateAvailability unexpected error branch
	_, _, _, err := eng.EvaluateAvailability(context.Background(), "completely-unknown", time.Now(), time.Now().AddDate(0, 0, 1), 1)
	if err == nil || !errors.Is(err, ErrUnknownRoomType) {
		t.Fatalf("EvaluateAvailability() want ErrUnknownRoomType, got %v", err)
	}

	// Test CalculateLockedQuote with calendar stop-sell restriction
	stopPrice := int64(2_000_000)
	stopStore := &mockCalendarStore{
		overrides: []CalendarOverride{
			{Date: time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), RoomTypeID: "room-restricted", IsStopSell: true, PriceOverrideIDR: &stopPrice},
		},
	}
	engRestricted := NewEngineWithQuoteStore(map[string]int64{"room-restricted": 1_000_000}, 1.25, nil)
	engRestricted.SetCalendarStore(stopStore)

	_, err = engRestricted.CalculateLockedQuote(context.Background(), QuoteRequest{
		RoomTypeID: "room-restricted",
		CheckIn:    time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
		CheckOut:   time.Date(2026, 12, 26, 0, 0, 0, 0, time.UTC),
		NumRooms:   1,
	})
	if !errors.Is(err, ErrStopSellApplied) {
		t.Fatalf("CalculateLockedQuote() want ErrStopSellApplied, got %v", err)
	}
}

// ============================================================================
// PROMO CAMPAIGN DYNAMIC TESTS
// ============================================================================

type mockPromoStore struct {
	campaigns  map[string]PromoCampaign
	reserveErr error
	releaseErr error
}

func (m *mockPromoStore) GetByCode(_ context.Context, code string) (PromoCampaign, error) {
	c, ok := m.campaigns[code]
	if !ok {
		return PromoCampaign{}, ErrPromoNotFound
	}
	return c, nil
}

func (m *mockPromoStore) ListCampaigns(_ context.Context) ([]PromoCampaign, error) {
	var list []PromoCampaign
	for _, c := range m.campaigns {
		list = append(list, c)
	}
	return list, nil
}

func (m *mockPromoStore) CreateCampaign(_ context.Context, p PromoCampaign) (string, error) {
	if m.campaigns == nil {
		m.campaigns = make(map[string]PromoCampaign)
	}
	m.campaigns[p.Code] = p
	return "mock-promo-id", nil
}

func (m *mockPromoStore) UpdateCampaign(_ context.Context, id string, isActive bool, quotaTotal int) error {
	for k, c := range m.campaigns {
		if c.ID == id {
			c.IsActive = isActive
			c.QuotaTotal = quotaTotal
			m.campaigns[k] = c
			return nil
		}
	}
	return ErrPromoNotFound
}

func (m *mockPromoStore) ReserveQuotaAtomic(_ context.Context, code string) error {
	if m.reserveErr != nil {
		return m.reserveErr
	}
	c, ok := m.campaigns[code]
	if !ok {
		return ErrPromoNotFound
	}
	if c.QuotaUsed >= c.QuotaTotal {
		return ErrPromoQuotaExhausted
	}
	c.QuotaUsed++
	m.campaigns[code] = c
	return nil
}

func (m *mockPromoStore) ReleaseQuotaAtomic(_ context.Context, code string) error {
	if m.releaseErr != nil {
		return m.releaseErr
	}
	c, ok := m.campaigns[code]
	if !ok {
		return ErrPromoNotFound
	}
	if c.QuotaUsed > 0 {
		c.QuotaUsed--
		m.campaigns[code] = c
	}
	return nil
}

func TestPromo_DynamicCampaigns(t *testing.T) {
	roomID := "01900000-0000-7000-8000-000000000001"
	otherRoomID := "01900000-0000-7000-8000-000000000002"

	now := time.Now()
	maxCap := int64(100_000)

	mockStore := &mockPromoStore{
		campaigns: map[string]PromoCampaign{
			"SUPER20": {
				ID:             "p-1",
				Code:           "SUPER20",
				Name:           "Diskon 20% capped 100k",
				DiscountType:   "PERCENT",
				DiscountValue:  20,
				MaxDiscountIDR: &maxCap,
				MinStayNights:  1,
				QuotaTotal:     10,
				QuotaUsed:      2,
				ValidFrom:      now.Add(-24 * time.Hour),
				ValidTo:        now.Add(24 * time.Hour),
				IsActive:       true,
			},
			"FLAT50K": {
				ID:            "p-2",
				Code:          "FLAT50K",
				Name:          "Potongan 50k",
				DiscountType:  "FIXED",
				DiscountValue: 50_000,
				MinStayNights: 1,
				QuotaTotal:    10,
				QuotaUsed:     0,
				ValidFrom:     now.Add(-24 * time.Hour),
				ValidTo:       now.Add(24 * time.Hour),
				IsActive:      true,
			},
			"EXPIREDPROMO": {
				ID:            "p-3",
				Code:          "EXPIREDPROMO",
				Name:          "Promo Kadaluarsa",
				DiscountType:  "PERCENT",
				DiscountValue: 10,
				MinStayNights: 1,
				QuotaTotal:    10,
				ValidFrom:     now.Add(-48 * time.Hour),
				ValidTo:       now.Add(-24 * time.Hour),
				IsActive:      true,
			},
			"HABISQUOTA": {
				ID:            "p-4",
				Code:          "HABISQUOTA",
				Name:          "Promo Kuota Habis",
				DiscountType:  "PERCENT",
				DiscountValue: 10,
				MinStayNights: 1,
				QuotaTotal:    5,
				QuotaUsed:     5,
				ValidFrom:     now.Add(-24 * time.Hour),
				ValidTo:       now.Add(24 * time.Hour),
				IsActive:      true,
			},
			"MINSTAY3": {
				ID:            "p-5",
				Code:          "MINSTAY3",
				Name:          "Promo Min 3 Malam",
				DiscountType:  "PERCENT",
				DiscountValue: 10,
				MinStayNights: 3,
				QuotaTotal:    10,
				QuotaUsed:     0,
				ValidFrom:     now.Add(-24 * time.Hour),
				ValidTo:       now.Add(24 * time.Hour),
				IsActive:      true,
			},
			"SPECIFICROOM": {
				ID:                  "p-6",
				Code:                "SPECIFICROOM",
				Name:                "Khusus Room Tertentu",
				DiscountType:        "PERCENT",
				DiscountValue:       10,
				MinStayNights:       1,
				QuotaTotal:          10,
				QuotaUsed:           0,
				ValidFrom:           now.Add(-24 * time.Hour),
				ValidTo:             now.Add(24 * time.Hour),
				ApplicableRoomTypes: []string{otherRoomID},
				IsActive:            true,
			},
		},
	}

	eng := NewEngineWithQuoteStore(map[string]int64{
		roomID:      1_000_000,
		otherRoomID: 1_500_000,
	}, 1.25, nil)
	eng.SetPromoStore(mockStore)

	tests := []struct {
		name         string
		promoCode    string
		roomID       string
		checkIn      time.Time
		checkOut     time.Time
		wantErr      error
		wantDiscount int64
	}{
		{
			name:         "Percent discount with max cap applied",
			promoCode:    "SUPER20",
			roomID:       roomID,
			checkIn:      now.Add(24 * time.Hour),
			checkOut:     now.Add(48 * time.Hour), // 1 night: 1,000,000. 20% = 200,000, capped at 100,000
			wantErr:      nil,
			wantDiscount: 100_000,
		},
		{
			name:         "Fixed discount applied",
			promoCode:    "FLAT50K",
			roomID:       roomID,
			checkIn:      now.Add(24 * time.Hour),
			checkOut:     now.Add(48 * time.Hour), // 1 night: 1,000,000 - 50,000
			wantErr:      nil,
			wantDiscount: 50_000,
		},
		{
			name:      "Expired promo returns ErrPromoExpired",
			promoCode: "EXPIREDPROMO",
			roomID:    roomID,
			checkIn:   now.Add(24 * time.Hour),
			checkOut:  now.Add(48 * time.Hour),
			wantErr:   ErrPromoExpired,
		},
		{
			name:      "Exhausted quota returns ErrPromoQuotaExhausted",
			promoCode: "HABISQUOTA",
			roomID:    roomID,
			checkIn:   now.Add(24 * time.Hour),
			checkOut:  now.Add(48 * time.Hour),
			wantErr:   ErrPromoQuotaExhausted,
		},
		{
			name:      "Min stay nights not met returns ErrPromoMinStayNotMet",
			promoCode: "MINSTAY3",
			roomID:    roomID,
			checkIn:   now.Add(24 * time.Hour),
			checkOut:  now.Add(48 * time.Hour), // 1 night < 3
			wantErr:   ErrPromoMinStayNotMet,
		},
		{
			name:         "Min stay nights satisfied succeeds",
			promoCode:    "MINSTAY3",
			roomID:       roomID,
			checkIn:      now.Add(24 * time.Hour),
			checkOut:     now.Add(96 * time.Hour), // 3 nights
			wantErr:      nil,
			wantDiscount: 300_000, // 3 * 1,000,000 = 3,000,000 * 10% = 300,000
		},
		{
			name:      "Room type not applicable returns ErrPromoRoomNotApplicable",
			promoCode: "SPECIFICROOM",
			roomID:    roomID,
			checkIn:   now.Add(24 * time.Hour),
			checkOut:  now.Add(48 * time.Hour),
			wantErr:   ErrPromoRoomNotApplicable,
		},
		{
			name:         "Room type applicable succeeds",
			promoCode:    "SPECIFICROOM",
			roomID:       otherRoomID,
			checkIn:      now.Add(24 * time.Hour),
			checkOut:     now.Add(48 * time.Hour), // 1 night: 1,500,000. 10% = 150,000
			wantErr:      nil,
			wantDiscount: 150_000,
		},
		{
			name:      "Non-existent promo returns ErrInvalidPromoCode",
			promoCode: "NONEXISTENTPROMO",
			roomID:    roomID,
			checkIn:   now.Add(24 * time.Hour),
			checkOut:  now.Add(48 * time.Hour),
			wantErr:   ErrInvalidPromoCode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lq, err := eng.CalculateLockedQuote(context.Background(), QuoteRequest{
				RoomTypeID: tc.roomID,
				CheckIn:    tc.checkIn,
				CheckOut:   tc.checkOut,
				NumRooms:   1,
				NumGuests:  2,
				PromoCode:  tc.promoCode,
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("CalculateLockedQuote() err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CalculateLockedQuote() unexpected err = %v", err)
			}
			if lq.Pricing.DiscountMinor != tc.wantDiscount {
				t.Fatalf("lq.Pricing.DiscountMinor = %d, want %d", lq.Pricing.DiscountMinor, tc.wantDiscount)
			}
			if lq.CancellationCode != PolicyNonRefundable {
				t.Fatalf("lq.CancellationCode = %s, want %s", lq.CancellationCode, PolicyNonRefundable)
			}
		})
	}
}

func TestPromoStore_ReserveAndRelease(t *testing.T) {
	ctx := context.Background()
	mockStore := &mockPromoStore{
		campaigns: map[string]PromoCampaign{
			"PROMO1": {
				Code:       "PROMO1",
				QuotaTotal: 2,
				QuotaUsed:  0,
			},
		},
	}

	if err := mockStore.ReserveQuotaAtomic(ctx, "PROMO1"); err != nil {
		t.Fatalf("ReserveQuotaAtomic(1) err = %v", err)
	}
	if err := mockStore.ReserveQuotaAtomic(ctx, "PROMO1"); err != nil {
		t.Fatalf("ReserveQuotaAtomic(2) err = %v", err)
	}
	if err := mockStore.ReserveQuotaAtomic(ctx, "PROMO1"); !errors.Is(err, ErrPromoQuotaExhausted) {
		t.Fatalf("ReserveQuotaAtomic(3) want ErrPromoQuotaExhausted, got %v", err)
	}

	if err := mockStore.ReleaseQuotaAtomic(ctx, "PROMO1"); err != nil {
		t.Fatalf("ReleaseQuotaAtomic() err = %v", err)
	}
	if err := mockStore.ReserveQuotaAtomic(ctx, "PROMO1"); err != nil {
		t.Fatalf("ReserveQuotaAtomic() after release err = %v", err)
	}

	mockStore.reserveErr = errors.New("db error")
	if err := mockStore.ReserveQuotaAtomic(ctx, "PROMO1"); err == nil {
		t.Fatalf("expected error from mockStore.reserveErr")
	}

	mockStore.releaseErr = errors.New("db error")
	if err := mockStore.ReleaseQuotaAtomic(ctx, "PROMO1"); err == nil {
		t.Fatalf("expected error from mockStore.releaseErr")
	}
}

