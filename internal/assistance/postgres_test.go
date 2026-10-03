package assistance

import (
	"context"
	"errors"
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

func TestPostgresStore_Lifecycle_RealDB(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	// Cari 1 booking id yang ada untuk pengujian relasi
	var bookingID, guestEmail string
	err := pool.QueryRow(ctx, "SELECT id, guest_email FROM bookings LIMIT 1;").Scan(&bookingID, &guestEmail)
	if err != nil {
		t.Skipf("no bookings available for test: %v", err)
		return
	}

	// 1. GetBookingOwner
	owner, exists, err := store.GetBookingOwner(ctx, bookingID)
	if err != nil {
		t.Fatalf("GetBookingOwner error: %v", err)
	}
	if !exists || owner != guestEmail {
		t.Fatalf("expected owner %s, got %s (exists=%v)", guestEmail, owner, exists)
	}

	// 1b. GetBookingOwner non-existent
	_, notExists, err := store.GetBookingOwner(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetBookingOwner error on missing: %v", err)
	}
	if notExists {
		t.Fatalf("expected exists=false for missing booking")
	}

	// 2. CreateRequest
	now := time.Now().UTC().Truncate(time.Second)
	req := SpecialRequest{
		BookingID:   bookingID,
		Category:    CategoryCelebrationSetup,
		Department:  DepartmentHousekeeping,
		Description: "Uji coba permintaan dekorasi mawar",
		TargetTime:  "15:00",
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	created, err := store.CreateRequest(ctx, req)
	if err != nil {
		t.Fatalf("CreateRequest error: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
	if created.Category != CategoryCelebrationSetup {
		t.Errorf("expected category %s, got %s", CategoryCelebrationSetup, created.Category)
	}

	// Cleanup at test end
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM booking_special_requests WHERE id = $1", created.ID)
	})

	// 3. GetRequestByID
	fetched, err := store.GetRequestByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRequestByID error: %v", err)
	}
	if fetched == nil || fetched.ID != created.ID {
		t.Fatalf("expected fetched request with ID %s, got %+v", created.ID, fetched)
	}

	// 3b. GetRequestByID non-existent
	missing, err := store.GetRequestByID(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetRequestByID on missing error: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing request, got %+v", missing)
	}

	// 4. ListByBookingID
	list, err := store.ListByBookingID(ctx, bookingID)
	if err != nil {
		t.Fatalf("ListByBookingID error: %v", err)
	}
	if len(list) == 0 {
		t.Errorf("expected at least 1 request in list, got 0")
	}

	// 5. ListStaffQueue
	queue, err := store.ListStaffQueue(ctx, ListFilter{
		Department: string(DepartmentHousekeeping),
		Status:     string(StatusPending),
	})
	if err != nil {
		t.Fatalf("ListStaffQueue error: %v", err)
	}
	found := false
	for _, item := range queue {
		if item.ID == created.ID {
			found = true
			if item.GuestName == "" {
				t.Errorf("expected guest name populated in staff queue item")
			}
			break
		}
	}
	if !found {
		t.Errorf("created request %s not found in staff queue", created.ID)
	}

	// 6. UpdateStatus
	updated, err := store.UpdateStatus(ctx, created.ID, StatusAcknowledged, "Sudah disiapkan", "fo_receptionist", now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}
	if updated.Status != StatusAcknowledged {
		t.Errorf("expected status %s, got %s", StatusAcknowledged, updated.Status)
	}
	if updated.StaffNotes != "Sudah disiapkan" {
		t.Errorf("expected staff notes 'Sudah disiapkan', got '%s'", updated.StaffNotes)
	}

	// 6b. UpdateStatus missing ID
	_, err = store.UpdateStatus(ctx, "00000000-0000-0000-0000-000000000000", StatusFulfilled, "Catatan", "staff", now)
	if !errors.Is(err, ErrRequestNotFound) {
		t.Errorf("expected ErrRequestNotFound, got %v", err)
	}
}
