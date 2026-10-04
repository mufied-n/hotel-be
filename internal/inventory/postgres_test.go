package inventory

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getInventoryTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping postgres inventory test: %v", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("skipping postgres inventory test: %v", err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgres inventory test: ping failed: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPostgresStore_Integration(t *testing.T) {
	pool := getInventoryTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := &PostgresStore{Pool: pool}

	roomTypeID := "01900000-0000-7000-8000-000000000001"
	from := time.Date(2028, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2028, 6, 3, 0, 0, 0, 0, time.UTC)

	// EnsureRows
	err := store.EnsureRows(ctx, roomTypeID, from, to, 5)
	if err != nil {
		t.Fatalf("EnsureRows failed: %v", err)
	}

	// GetByDate
	avail, err := store.GetByDate(ctx, roomTypeID, from, to)
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}
	if len(avail) != 2 {
		t.Fatalf("expected 2 nights, got %d", len(avail))
	}

	// CheckAvailability
	if err := store.CheckAvailability(ctx, roomTypeID, from, to, 1); err != nil {
		t.Fatalf("CheckAvailability failed: %v", err)
	}

	// CheckAvailabilityWithBuffer
	if err := store.CheckAvailabilityWithBuffer(ctx, roomTypeID, from, to, 1, 1); err != nil {
		t.Fatalf("CheckAvailabilityWithBuffer failed: %v", err)
	}

	// EnsureHorizon
	if err := store.EnsureHorizon(ctx, 30); err != nil {
		t.Fatalf("EnsureHorizon failed: %v", err)
	}
}
