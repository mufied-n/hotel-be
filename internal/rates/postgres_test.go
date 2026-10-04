package rates

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getRatesTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping postgres rates store test: %v", err)
		return nil
	}
	cfg.MaxConns = 10

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("skipping postgres rates store test: %v", err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgres rates store test: ping failed: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPostgresRatesStores_Integration(t *testing.T) {
	pool := getRatesTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()

	calStore := NewPostgresCalendarStore(pool)
	promoStore := NewPostgresPromoStore(pool)

	// 1. Calendar test
	now := time.Now().Truncate(24 * time.Hour)
	roomID := "01900000-0000-7000-8000-000000000001"
	price := int64(1_800_000)
	stopSell := false

	affected, err := calStore.BulkUpsertOverrides(ctx, BulkCalendarUpdateRequest{
		RoomTypeIDs:      []string{roomID},
		StartDate:        now.Format("2006-01-02"),
		EndDate:          now.AddDate(0, 0, 1).Format("2006-01-02"),
		RatePlanCode:     "RO",
		PriceOverrideIDR: &price,
		IsStopSell:       &stopSell,
	})
	if err != nil {
		t.Fatalf("BulkUpsertOverrides() err = %v", err)
	}
	if affected == 0 {
		t.Fatalf("BulkUpsertOverrides() affected = 0")
	}

	items, err := calStore.GetCalendar(ctx, now, now.AddDate(0, 0, 1), "RO", &roomID)
	if err != nil {
		t.Fatalf("GetCalendar() err = %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("GetCalendar() expected items")
	}

	// 2. Promo test
	code := "TESTPROMO_" + time.Now().Format("150405")
	promoID, err := promoStore.CreateCampaign(ctx, PromoCampaign{
		Code:          code,
		Name:          "Test Promo",
		DiscountType:  "PERCENT",
		DiscountValue: 15,
		QuotaTotal:    5,
		ValidFrom:     now.Add(-24 * time.Hour),
		ValidTo:       now.Add(24 * time.Hour),
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("CreateCampaign() err = %v", err)
	}

	p, err := promoStore.GetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetByCode() err = %v", err)
	}
	if p.ID != promoID {
		t.Fatalf("GetByCode() ID mismatch")
	}

	promos, err := promoStore.ListCampaigns(ctx)
	if err != nil {
		t.Fatalf("ListCampaigns() err = %v", err)
	}
	if len(promos) == 0 {
		t.Fatalf("ListCampaigns() empty")
	}

	if err := promoStore.ReserveQuotaAtomic(ctx, code); err != nil {
		t.Fatalf("ReserveQuotaAtomic() err = %v", err)
	}
	if err := promoStore.ReleaseQuotaAtomic(ctx, code); err != nil {
		t.Fatalf("ReleaseQuotaAtomic() err = %v", err)
	}

	if err := promoStore.UpdateCampaign(ctx, promoID, false, 10); err != nil {
		t.Fatalf("UpdateCampaign() err = %v", err)
	}
}

func TestPostgresCalendarStore_Validation(t *testing.T) {
	store := NewPostgresCalendarStore(nil)

	// Test invalid start_date format
	_, err := store.BulkUpsertOverrides(context.Background(), BulkCalendarUpdateRequest{
		StartDate: "invalid-date",
		EndDate:   "2026-12-31",
	})
	if err == nil {
		t.Fatalf("BulkUpsertOverrides() expected error for invalid start_date")
	}

	// Test invalid end_date format
	_, err = store.BulkUpsertOverrides(context.Background(), BulkCalendarUpdateRequest{
		StartDate: "2026-12-01",
		EndDate:   "invalid-date",
	})
	if err == nil {
		t.Fatalf("BulkUpsertOverrides() expected error for invalid end_date")
	}

	// Test end_date before start_date
	_, err = store.BulkUpsertOverrides(context.Background(), BulkCalendarUpdateRequest{
		StartDate: "2026-12-31",
		EndDate:   "2026-12-01",
	})
	if err == nil {
		t.Fatalf("BulkUpsertOverrides() expected error for end_date before start_date")
	}

	// Test constructors
	pgCal := NewPostgresCalendarStore(nil)
	if pgCal == nil {
		t.Fatalf("NewPostgresCalendarStore() is nil")
	}
	pgPromo := NewPostgresPromoStore(nil)
	if pgPromo == nil {
		t.Fatalf("NewPostgresPromoStore() is nil")
	}
}
