package housekeeping

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

func TestPostgresStore_Lifecycle(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	// 1. Get seeded room (201 - Superior King, Floor 2)
	room, err := store.GetRoom(ctx, "201")
	if err != nil {
		t.Fatalf("GetRoom 201 failed: %v", err)
	}
	if room.RoomNumber != "201" || room.Floor != 2 {
		t.Errorf("unexpected room 201 data: %+v", room)
	}

	// 2. Update room cleanliness
	err = store.UpdateRoomCleanliness(ctx, "201", StatusCleaning, "Attendant in room", "staff:hk_attendant_01")
	if err != nil {
		t.Fatalf("UpdateRoomCleanliness failed: %v", err)
	}

	updatedRoom, err := store.GetRoom(ctx, "201")
	if err != nil {
		t.Fatalf("GetRoom 201 after update failed: %v", err)
	}
	if updatedRoom.CleanlinessStatus != StatusCleaning {
		t.Errorf("cleanliness status = %v, want cleaning", updatedRoom.CleanlinessStatus)
	}

	// Restore room 201 back to inspected
	_ = store.UpdateRoomCleanliness(ctx, "201", StatusInspected, "Restored for testing", "system")

	// 3. List rooms (95 total rooms seeded)
	allRooms, err := store.ListRooms(ctx, 0, "", "")
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}
	if len(allRooms) < 95 {
		t.Errorf("expected at least 95 rooms, got %d", len(allRooms))
	}

	// List rooms filtered by floor 2
	floor2Rooms, err := store.ListRooms(ctx, 2, "", "")
	if err != nil {
		t.Fatalf("ListRooms floor 2 failed: %v", err)
	}
	if len(floor2Rooms) == 0 {
		t.Errorf("expected rooms on floor 2")
	}

	// 4. Non-existent room returns ErrRoomNotFound
	_, err = store.GetRoom(ctx, "9999")
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
	err = store.UpdateRoomCleanliness(ctx, "9999", StatusCleaning, "", "actor")
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound on update, got %v", err)
	}

	// 5. Deduct inventory for OOO
	startDate := time.Now().UTC().Truncate(24 * time.Hour).Add(10 * 24 * time.Hour)
	endDate := startDate.Add(24 * time.Hour)
	err = store.DeductInventoryForOOO(ctx, room.RoomTypeID, startDate, endDate)
	if err != nil {
		t.Fatalf("DeductInventoryForOOO failed: %v", err)
	}
}
