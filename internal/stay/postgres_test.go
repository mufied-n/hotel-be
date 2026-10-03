package stay

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

func TestPostgresStore_StayModificationLifecycle(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()

	// 1. Setup test fixture
	bID := "01900000-0000-7000-8000-000000000777"
	roomTypeID := "01900000-0000-7000-8000-000000000001" // Superior King
	today := time.Now().UTC().Truncate(24 * time.Hour)
	checkIn := today
	checkOut := today.AddDate(0, 0, 2)

	// Clean up previous test residue
	_, _ = pool.Exec(ctx, "DELETE FROM room_move_logs WHERE booking_id = $1", bID)
	_, _ = pool.Exec(ctx, "DELETE FROM room_assignments WHERE booking_id = $1", bID)
	_, _ = pool.Exec(ctx, "DELETE FROM reservation_room_nights WHERE booking_id = $1", bID)
	_, _ = pool.Exec(ctx, "DELETE FROM bookings WHERE id = $1", bID)

	// Pastikan kamar 201 dan 202 ada dan reset status
	_, _ = pool.Exec(ctx, `
		INSERT INTO rooms (room_number, room_type_id, floor, cleanliness_status) 
		VALUES ('201', $1, 2, 'occupied'), ('202', $1, 2, 'inspected')
		ON CONFLICT (room_number) DO UPDATE SET cleanliness_status = EXCLUDED.cleanliness_status`, roomTypeID)
	_, _ = pool.Exec(ctx, "UPDATE rooms SET cleanliness_status = 'occupied' WHERE room_number = '201'")
	_, _ = pool.Exec(ctx, "UPDATE rooms SET cleanliness_status = 'inspected' WHERE room_number = '202'")

	// Insert booking test
	_, err := pool.Exec(ctx, `
		INSERT INTO bookings (id, room_type_id, guest_name, guest_email, check_in, check_out, num_rooms, num_guests, total_price_minor, status)
		VALUES ($1, $2, 'Pak Test Stay', 'teststay@example.com', $3, $4, 1, 2, 1100000, 'checked_in')`,
		bID, roomTypeID, checkIn, checkOut)
	if err != nil {
		t.Fatalf("failed to insert test booking: %v", err)
	}

	// Insert room_assignment awal di kamar 201
	_, err = pool.Exec(ctx, `
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		VALUES ($1, '201', daterange($2::date, $3::date, '[)'))`,
		bID, checkIn.Format("2006-01-02"), checkOut.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("failed to insert initial room assignment: %v", err)
	}

	store := NewPostgresStore(pool)

	// 2. Test GetBooking
	bDetails, err := store.GetBooking(ctx, bID)
	if err != nil {
		t.Fatalf("GetBooking failed: %v", err)
	}
	if bDetails.CurrentRoom != "201" {
		t.Errorf("current room = %s, want 201", bDetails.CurrentRoom)
	}

	// 3. Test MoveRoom: 201 -> 202
	moveInput := RoomMoveInput{
		BookingID:        bID,
		TargetRoomNumber: "202",
		ReasonCategory:   ReasonMaintenanceDefect,
		Notes:            "AC 201 rusak",
		ActorID:          "staff:receptionist_01",
	}

	moveRes, err := store.MoveRoom(ctx, moveInput, today)
	if err != nil {
		t.Fatalf("MoveRoom failed: %v", err)
	}
	if moveRes.PreviousRoomNumber != "201" || moveRes.NewRoomNumber != "202" {
		t.Errorf("unexpected move result: %+v", moveRes)
	}

	// Verifikasi status kamar 201 sekarang vacant_dirty
	var status201, status202 string
	_ = pool.QueryRow(ctx, "SELECT cleanliness_status FROM rooms WHERE room_number = '201'").Scan(&status201)
	_ = pool.QueryRow(ctx, "SELECT cleanliness_status FROM rooms WHERE room_number = '202'").Scan(&status202)
	if status201 != "vacant_dirty" {
		t.Errorf("room 201 status = %s, want vacant_dirty", status201)
	}
	if status202 != "occupied" {
		t.Errorf("room 202 status = %s, want occupied", status202)
	}

	// 4. Test ListRoomMoves
	moves, err := store.ListRoomMoves(ctx, bID)
	if err != nil {
		t.Fatalf("ListRoomMoves failed: %v", err)
	}
	if len(moves) == 0 {
		t.Fatal("expected at least 1 room move log")
	}
	if moves[0].FromRoomNumber != "201" || moves[0].ToRoomNumber != "202" {
		t.Errorf("unexpected log data: %+v", moves[0])
	}

	// 5. Test ExtendStay: tambah 1 malam
	newCheckOut := checkOut.AddDate(0, 0, 1)
	// Pastikan ada stok inventaris untuk tanggal newCheckOut
	_, _ = pool.Exec(ctx, `
		INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
		VALUES ($1, $2, 20, 10)
		ON CONFLICT (room_type_id, date) DO UPDATE SET available_rooms = 10`,
		roomTypeID, checkOut.Format("2006-01-02"))

	extRes, err := store.ExtendStay(ctx, bID, 1, newCheckOut, []int64{550_000}, 550_000)
	if err != nil {
		t.Fatalf("ExtendStay failed: %v", err)
	}
	if extRes.AdditionalNights != 1 || extRes.NewTotalPriceMinor != 1_650_000 {
		t.Errorf("unexpected extension result: %+v", extRes)
	}

	// 6. Test Mid-Stay Move (moveDate > checkIn) & Negative Tests
	bID2 := "01900000-0000-7000-8000-000000000778"
	_, _ = pool.Exec(ctx, "DELETE FROM room_move_logs WHERE booking_id = $1", bID2)
	_, _ = pool.Exec(ctx, "DELETE FROM room_assignments WHERE booking_id = $1", bID2)
	_, _ = pool.Exec(ctx, "DELETE FROM bookings WHERE id = $1", bID2)

	twoDaysAgo := today.AddDate(0, 0, -2)
	twoDaysLater := today.AddDate(0, 0, 2)
	_, _ = pool.Exec(ctx, `
		INSERT INTO bookings (id, room_type_id, guest_name, guest_email, check_in, check_out, num_rooms, num_guests, total_price_minor, status)
		VALUES ($1, $2, 'Pak Mid Stay', 'midstay@example.com', $3, $4, 1, 2, 2200000, 'checked_in')`,
		bID2, roomTypeID, twoDaysAgo, twoDaysLater)

	_, _ = pool.Exec(ctx, `
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		VALUES ($1, '201', daterange($2::date, $3::date, '[)'))`,
		bID2, twoDaysAgo.Format("2006-01-02"), twoDaysLater.Format("2006-01-02"))

	// Kamar 203 disiapkan inspected
	_, _ = pool.Exec(ctx, `
		INSERT INTO rooms (room_number, room_type_id, floor, cleanliness_status) 
		VALUES ('203', $1, 2, 'inspected')
		ON CONFLICT (room_number) DO UPDATE SET cleanliness_status = 'inspected'`, roomTypeID)

	// Negative Test: Same room move -> ErrSameRoomMove
	_, errSame := store.MoveRoom(ctx, RoomMoveInput{
		BookingID:        bID2,
		TargetRoomNumber: "201",
		ReasonCategory:   ReasonUpgrade,
	}, today)
	if !errors.Is(errSame, ErrSameRoomMove) {
		t.Errorf("expected ErrSameRoomMove, got %v", errSame)
	}

	// Negative Test: Target room not inspected -> ErrTargetRoomNotReady
	_, _ = pool.Exec(ctx, "UPDATE rooms SET cleanliness_status = 'vacant_dirty' WHERE room_number = '203'")
	_, errNotReady := store.MoveRoom(ctx, RoomMoveInput{
		BookingID:        bID2,
		TargetRoomNumber: "203",
		ReasonCategory:   ReasonUpgrade,
	}, today)
	if !errors.Is(errNotReady, ErrTargetRoomNotReady) {
		t.Errorf("expected ErrTargetRoomNotReady, got %v", errNotReady)
	}

	// Positive Test: Mid-stay move (moveDate > checkIn)
	_, _ = pool.Exec(ctx, "UPDATE rooms SET cleanliness_status = 'inspected' WHERE room_number = '203'")
	resMid, errMid := store.MoveRoom(ctx, RoomMoveInput{
		BookingID:        bID2,
		TargetRoomNumber: "203",
		ReasonCategory:   ReasonUpgrade,
		Notes:            "Upgrade to executive",
		ActorID:          "staff:receptionist_02",
	}, today)
	if errMid != nil {
		t.Fatalf("mid-stay move failed: %v", errMid)
	}
	if resMid.NewRoomNumber != "203" {
		t.Errorf("expected new room 203, got %s", resMid.NewRoomNumber)
	}

	// Negative Test: Extend stay with zero inventory -> ErrNoAvailabilityForExtension
	_, _ = pool.Exec(ctx, `
		INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
		VALUES ($1, $2, 20, 0)
		ON CONFLICT (room_type_id, date) DO UPDATE SET available_rooms = 0`,
		roomTypeID, twoDaysLater.Format("2006-01-02"))
	_, errNoInv := store.ExtendStay(ctx, bID2, 1, twoDaysLater.AddDate(0, 0, 1), []int64{550_000}, 550_000)
	if !errors.Is(errNoInv, ErrNoAvailabilityForExtension) {
		t.Errorf("expected ErrNoAvailabilityForExtension, got %v", errNoInv)
	}

	// Cleanup
	_, _ = pool.Exec(ctx, "DELETE FROM room_move_logs WHERE booking_id IN ($1, $2)", bID, bID2)
	_, _ = pool.Exec(ctx, "DELETE FROM room_assignments WHERE booking_id IN ($1, $2)", bID, bID2)
	_, _ = pool.Exec(ctx, "DELETE FROM reservation_room_nights WHERE booking_id IN ($1, $2)", bID, bID2)
	_, _ = pool.Exec(ctx, "DELETE FROM bookings WHERE id IN ($1, $2)", bID, bID2)
}
