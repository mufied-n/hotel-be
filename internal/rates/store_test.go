package rates

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCalendarStore_TableDriven(t *testing.T) {
	store := NewMemoryCalendarStore()
	ctx := context.Background()

	roomID := "01900000-0000-7000-8000-000000000001"
	price := int64(1_500_000)
	stopSell := true

	// 1. Bulk Upsert Validation Errors
	testsValidation := []struct {
		name    string
		req     BulkCalendarUpdateRequest
		wantErr bool
	}{
		{
			name: "Invalid StartDate format",
			req: BulkCalendarUpdateRequest{
				StartDate: "bad-date",
				EndDate:   "2026-12-31",
			},
			wantErr: true,
		},
		{
			name: "Invalid EndDate format",
			req: BulkCalendarUpdateRequest{
				StartDate: "2026-12-01",
				EndDate:   "bad-date",
			},
			wantErr: true,
		},
		{
			name: "EndDate before StartDate",
			req: BulkCalendarUpdateRequest{
				StartDate: "2026-12-31",
				EndDate:   "2026-12-01",
			},
			wantErr: true,
		},
		{
			name: "Valid request with default RO plan",
			req: BulkCalendarUpdateRequest{
				RoomTypeIDs:      []string{roomID},
				StartDate:        "2026-12-25",
				EndDate:          "2026-12-26",
				PriceOverrideIDR: &price,
				IsStopSell:       &stopSell,
			},
			wantErr: false,
		},
	}

	for _, tc := range testsValidation {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.BulkUpsertOverrides(ctx, tc.req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("BulkUpsertOverrides() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}

	// 2. Query Calendar
	start := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 26, 0, 0, 0, 0, time.UTC)

	res, err := store.GetCalendar(ctx, start, end, "RO", &roomID)
	if err != nil {
		t.Fatalf("GetCalendar() err = %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("GetCalendar() len = %d, want 2", len(res))
	}
	if !res[0].IsStopSell || res[0].EffectivePrice != 1_500_000 {
		t.Fatalf("GetCalendar()[0] invalid: %+v", res[0])
	}
}

func TestMemoryPromoStore_TableDriven(t *testing.T) {
	store := NewMemoryPromoStore()
	ctx := context.Background()

	now := time.Now()

	// 1. Create Campaign
	p := PromoCampaign{
		Code:          "SAVE10",
		Name:          "Diskon 10%",
		DiscountType:  "PERCENT",
		DiscountValue: 10,
		QuotaTotal:    2,
		ValidFrom:     now.Add(-1 * time.Hour),
		ValidTo:       now.Add(24 * time.Hour),
		IsActive:      true,
	}

	id, err := store.CreateCampaign(ctx, p)
	if err != nil || id == "" {
		t.Fatalf("CreateCampaign() err = %v, id = %s", err, id)
	}

	// 2. GetByCode
	got, err := store.GetByCode(ctx, "save10") // case-insensitive
	if err != nil {
		t.Fatalf("GetByCode() err = %v", err)
	}
	if got.Code != "SAVE10" {
		t.Fatalf("GetByCode() code = %s, want SAVE10", got.Code)
	}

	// 3. ListCampaigns
	list, err := store.ListCampaigns(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListCampaigns() err = %v, len = %d", err, len(list))
	}

	// 4. UpdateCampaign
	if err := store.UpdateCampaign(ctx, id, false, 5); err != nil {
		t.Fatalf("UpdateCampaign() err = %v", err)
	}
	if err := store.UpdateCampaign(ctx, "non-existent", false, 5); !errors.Is(err, ErrPromoNotFound) {
		t.Fatalf("UpdateCampaign() want ErrPromoNotFound, got %v", err)
	}

	// Re-activate
	_ = store.UpdateCampaign(ctx, id, true, 2)

	// 5. Reserve & Quota
	if err := store.ReserveQuotaAtomic(ctx, "SAVE10"); err != nil {
		t.Fatalf("ReserveQuotaAtomic(1) err = %v", err)
	}
	if err := store.ReserveQuotaAtomic(ctx, "SAVE10"); err != nil {
		t.Fatalf("ReserveQuotaAtomic(2) err = %v", err)
	}
	if err := store.ReserveQuotaAtomic(ctx, "SAVE10"); !errors.Is(err, ErrPromoQuotaExhausted) {
		t.Fatalf("ReserveQuotaAtomic(3) want ErrPromoQuotaExhausted, got %v", err)
	}

	// 6. Release & Re-reserve
	if err := store.ReleaseQuotaAtomic(ctx, "SAVE10"); err != nil {
		t.Fatalf("ReleaseQuotaAtomic() err = %v", err)
	}
	if err := store.ReserveQuotaAtomic(ctx, "SAVE10"); err != nil {
		t.Fatalf("ReserveQuotaAtomic() after release err = %v", err)
	}

	// 7. Non-existent promo
	if _, err := store.GetByCode(ctx, "NOPE"); !errors.Is(err, ErrPromoNotFound) {
		t.Fatalf("GetByCode(NOPE) want ErrPromoNotFound, got %v", err)
	}
	if err := store.ReserveQuotaAtomic(ctx, "NOPE"); !errors.Is(err, ErrPromoNotFound) {
		t.Fatalf("ReserveQuotaAtomic(NOPE) want ErrPromoNotFound, got %v", err)
	}
	if err := store.ReleaseQuotaAtomic(ctx, "NOPE"); !errors.Is(err, ErrPromoNotFound) {
		t.Fatalf("ReleaseQuotaAtomic(NOPE) want ErrPromoNotFound, got %v", err)
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

	// 4. Default TTL branch
	storeDefault := NewMemoryQuoteStore(0)
	if storeDefault == nil || storeDefault.ttl != 15*time.Minute {
		t.Errorf("expected default TTL 15m")
	}
}
