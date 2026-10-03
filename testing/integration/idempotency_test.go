package integration

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api/http"
	"github.com/example/hotel-booking/internal/booking"
)

// TestRealDB_IdempotencyReserveAtomic memverifikasi BE-R08 pada PostgreSQL nyata:
// 20 klaim paralel untuk key yang sama menghasilkan tepat satu pemilik; setelah Complete,
// klaim berikutnya melihat respons tersimpan; reservasi kedaluwarsa dapat diambil alih.
func TestRealDB_IdempotencyReserveAtomic(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	store := http.NewPostgresIdempotencyStore(pool)
	ctx := context.Background()

	var acquired int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, err := store.Reserve(ctx, "idem-par", "hash-a")
			if err != nil {
				t.Errorf("Reserve error: %v", err)
				return
			}
			if ok {
				atomic.AddInt32(&acquired, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if acquired != 1 {
		t.Fatalf("acquired = %d, want 1", acquired)
	}

	rec, ok, err := store.Reserve(ctx, "idem-par", "hash-a")
	if err != nil || ok || !rec.InProgress() {
		t.Fatalf("expected in-progress record, got ok=%v inProgress=%v err=%v", ok, rec.InProgress(), err)
	}

	if err := store.Complete(ctx, http.IdempotencyRecord{
		Key: "idem-par", RequestHash: "hash-a", ResponseCode: 201, ResponseBody: `{"ok":true}`,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	rec, ok, err = store.Reserve(ctx, "idem-par", "hash-a")
	if err != nil || ok || rec.InProgress() || rec.ResponseCode != 201 {
		t.Fatalf("expected completed replay, got ok=%v code=%d err=%v", ok, rec.ResponseCode, err)
	}

	// Release hanya menghapus reservasi in-progress, tidak respons selesai.
	_ = store.Release(ctx, "idem-par")
	if _, ok, _ = store.Reserve(ctx, "idem-par", "hash-a"); ok {
		t.Fatal("Release tidak boleh menghapus respons yang sudah selesai")
	}

	// Reservasi in-progress kedaluwarsa dapat diambil alih (proses pemilik mati).
	_, _, _ = store.Reserve(ctx, "idem-stale", "hash-old")
	if _, err := pool.Exec(ctx, "UPDATE idempotency_keys SET expires_at = NOW() - interval '1 second' WHERE key = 'idem-stale'"); err != nil {
		t.Fatalf("expire setup: %v", err)
	}
	rec, ok, err = store.Reserve(ctx, "idem-stale", "hash-new")
	if err != nil || !ok || rec.RequestHash != "hash-new" {
		t.Fatalf("expected takeover, got ok=%v hash=%q err=%v", ok, rec.RequestHash, err)
	}
}

// Satu quote hanya boleh menjadi satu booking; percobaan kedua gagal dan stok tidak bocor.
func TestRealDB_QuoteSingleUse(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, _, _ := setupRealBookingService(pool)
	ctx := context.Background()

	roomTypeID := "01900000-0000-7000-8000-000000000001"
	checkIn := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	in := quoted(svc, booking.CreateInput{
		RoomTypeID: roomTypeID, CheckIn: checkIn, CheckOut: checkIn.Add(24 * time.Hour),
		NumRooms: 1, NumGuests: 2, GuestName: "Satu Quote", GuestEmail: "satu@example.com",
	})
	if _, _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	var before int
	_ = pool.QueryRow(ctx, "SELECT available_rooms FROM inventory WHERE room_type_id=$1 AND date=$2", roomTypeID, checkIn).Scan(&before)

	if _, _, err := svc.Create(ctx, in); !errors.Is(err, booking.ErrQuoteAlreadyUsed) {
		t.Fatalf("second create err = %v, want ErrQuoteAlreadyUsed", err)
	}
	var after, count int
	_ = pool.QueryRow(ctx, "SELECT available_rooms FROM inventory WHERE room_type_id=$1 AND date=$2", roomTypeID, checkIn).Scan(&after)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM bookings").Scan(&count)
	if after != before || count != 1 {
		t.Errorf("stock leaked or duplicate booking: before=%d after=%d bookings=%d", before, after, count)
	}
}

// Check-in kedua pada booking yang sama idempoten: kamar yang sama, tanpa assignment baru.
func TestRealDB_CheckInIdempotentReplay(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, _, _ := setupRealBookingService(pool)
	ctx := context.Background()

	checkIn := time.Now().UTC().Truncate(24 * time.Hour)
	b, _, err := svc.Create(ctx, quoted(svc, booking.CreateInput{
		RoomTypeID: "01900000-0000-7000-8000-000000000001", CheckIn: checkIn, CheckOut: checkIn.Add(24 * time.Hour),
		NumRooms: 1, NumGuests: 2, GuestName: "Replay", GuestEmail: "replay@example.com",
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Confirm(ctx, b.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	first, err := svc.CheckIn(ctx, b.ID)
	if err != nil || first.Already {
		t.Fatalf("first check-in = %+v, err=%v", first, err)
	}
	second, err := svc.CheckIn(ctx, b.ID)
	if err != nil || !second.Already {
		t.Fatalf("second check-in = %+v, err=%v", second, err)
	}
	if len(second.RoomNumbers) != 1 || second.RoomNumbers[0] != first.RoomNumbers[0] {
		t.Errorf("rooms changed on replay: %v vs %v", first.RoomNumbers, second.RoomNumbers)
	}
}
