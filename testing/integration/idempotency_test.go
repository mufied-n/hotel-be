package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api"
)

// TestRealDB_IdempotencyReserveAtomic memverifikasi BE-R08 pada PostgreSQL nyata:
// 20 klaim paralel untuk key yang sama menghasilkan tepat satu pemilik; setelah Complete,
// klaim berikutnya melihat respons tersimpan; reservasi kedaluwarsa dapat diambil alih.
func TestRealDB_IdempotencyReserveAtomic(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	store := api.NewPostgresIdempotencyStore(pool)
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

	if err := store.Complete(ctx, api.IdempotencyRecord{
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
