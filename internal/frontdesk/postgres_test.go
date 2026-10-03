package frontdesk

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping postgres store test: %v", err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgres store test: ping failed: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPostgresStore_GetDailyRosterAndHandover(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	// 1. Uji GetDailyRoster
	roster, err := store.GetDailyRoster(ctx, time.Now())
	if err != nil {
		t.Fatalf("GetDailyRoster failed: %v", err)
	}

	if roster.Metrics.TotalRooms != 95 {
		t.Errorf("expected 95 total rooms, got %d", roster.Metrics.TotalRooms)
	}
	if roster.Date == "" {
		t.Errorf("expected date string, got empty")
	}

	// 2. Uji CreateHandoverNote
	note := &HandoverNote{
		Shift:          ShiftMorning,
		CashFloatMinor: 1500000,
		PendingIssues:  "AC kamar 205 selesai diservis",
		VIPGuestNotes:  "Pak Budi check-out jam 12:00",
		ActorID:        "staff:receptionist_01",
		ActorRole:      "receptionist",
		CreatedAt:      time.Now().UTC(),
	}

	if err := store.CreateHandoverNote(ctx, note); err != nil {
		t.Fatalf("CreateHandoverNote failed: %v", err)
	}
	if note.ID == "" {
		t.Errorf("expected generated UUID ID, got empty")
	}

	// 3. Uji ListHandoverNotes
	notes, total, err := store.ListHandoverNotes(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListHandoverNotes failed: %v", err)
	}
	if total < 1 || len(notes) < 1 {
		t.Errorf("expected at least 1 note, got total %d, count %d", total, len(notes))
	}
	if notes[0].Shift == "" {
		t.Errorf("expected note shift to be populated, got empty")
	}
}
