package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetTestPool mengembalikan connection pool ke database PostgreSQL 18 nyata.
// Jika database tidak dapat dijangkau, tes akan dilewati (t.Skip) secara graceful.
func GetTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	// ResetTestData menjalankan TRUNCATE; DSN wajib eksplisit dan harus menunjuk database "*_test"
	// (kecuali ALLOW_DESTRUCTIVE_TESTS=1) agar data dev/produksi tidak terhapus tak sengaja.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test: TEST_DATABASE_URL is not set")
		return nil
	}
	if cfg, err := pgxpool.ParseConfig(dsn); err == nil &&
		!strings.HasSuffix(cfg.ConnConfig.Database, "_test") && os.Getenv("ALLOW_DESTRUCTIVE_TESTS") != "1" {
		t.Skipf("skipping integration test: database %q is not named *_test (set ALLOW_DESTRUCTIVE_TESTS=1 to override)", cfg.ConnConfig.Database)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: failed to configure pgx pool (%s): %v", dsn, err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: database ping failed (%s): %v", dsn, err)
		return nil
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// ResetTestData membersihkan data transaksi dan mereset inventaris kamar uji.
func ResetTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cleanupQuery := `
		TRUNCATE TABLE 
			bookings, 
			holds, 
			reservation_room_nights, 
			room_assignments, 
			outbox, 
			payment_attempts, 
			idempotency_keys 
		CASCADE;
	`
	if _, err := pool.Exec(ctx, cleanupQuery); err != nil {
		t.Fatalf("failed to truncate test tables: %v", err)
	}

	// Reset seluruh 95 kamar ke status siap huni default (inspected)
	if _, err := pool.Exec(ctx, "UPDATE rooms SET cleanliness_status = 'inspected', maintenance_notes = '', updated_by = 'system', updated_at = now();"); err != nil {
		t.Fatalf("failed to reset rooms cleanliness: %v", err)
	}

	// Reset ketersediaan inventaris untuk tipe Deluxe Balcony King (01900000-0000-7000-8000-000000000001)
	roomTypeID := "01900000-0000-7000-8000-000000000001"
	if _, err := pool.Exec(ctx, "DELETE FROM inventory WHERE room_type_id = $1;", roomTypeID); err != nil {
		t.Fatalf("failed to delete test inventory: %v", err)
	}
	insertQuery := `
		INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
		SELECT 
			$1, 
			CURRENT_DATE + i, 
			20, 
			20
		FROM generate_series(0, 30) AS i;
	`
	if _, err := pool.Exec(ctx, insertQuery, roomTypeID); err != nil {
		t.Fatalf("failed to insert test inventory: %v", err)
	}
}
