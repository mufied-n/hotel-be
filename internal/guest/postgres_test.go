package guest

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

	testEmail := "test_pg_" + time.Now().Format("150405") + "@example.com"
	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Challenge lifecycle
	ch := &Challenge{
		Email:       testEmail,
		CodeHash:    HashString("112233"),
		Attempts:    0,
		MaxAttempts: 3,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
	}
	err := store.CreateChallenge(ctx, ch)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}
	if ch.ID == "" {
		t.Fatalf("expected non-empty challenge ID")
	}

	fetchedCh, err := store.GetLatestActiveChallenge(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetLatestActiveChallenge failed: %v", err)
	}
	if fetchedCh == nil || fetchedCh.CodeHash != ch.CodeHash {
		t.Fatalf("expected code hash %s, got %v", ch.CodeHash, fetchedCh)
	}

	err = store.UpdateChallengeAttempts(ctx, ch.ID, 1)
	if err != nil {
		t.Fatalf("UpdateChallengeAttempts failed: %v", err)
	}

	err = store.MarkChallengeVerified(ctx, ch.ID, now)
	if err != nil {
		t.Fatalf("MarkChallengeVerified failed: %v", err)
	}

	// 2. Session lifecycle
	rawToken := "gst_sess_test_pg_" + time.Now().Format("150405")
	tokenHash := HashString(rawToken)
	sess := &GuestSession{
		GuestEmail:   testEmail,
		TokenHash:    tokenHash,
		ExpiresAt:    now.Add(24 * time.Hour),
		LastActiveAt: now,
		CreatedAt:    now,
	}
	err = store.CreateSession(ctx, sess)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.ID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	fetchedSess, err := store.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash failed: %v", err)
	}
	if fetchedSess == nil || fetchedSess.GuestEmail != testEmail {
		t.Fatalf("expected session for %s, got %v", testEmail, fetchedSess)
	}

	err = store.TouchSession(ctx, sess.ID, now.Add(5*time.Minute), now.Add(25*time.Hour))
	if err != nil {
		t.Fatalf("TouchSession failed: %v", err)
	}

	// 3. Count active bookings & List bookings
	count, err := store.CountActiveBookingsByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("CountActiveBookingsByEmail failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 active bookings, got %d", count)
	}

	listAll, err := store.ListBookingsByEmail(ctx, testEmail, "all", 10)
	if err != nil {
		t.Fatalf("ListBookingsByEmail(all) failed: %v", err)
	}
	if len(listAll) != 0 {
		t.Errorf("expected 0 bookings, got %d", len(listAll))
	}

	_, _ = store.ListBookingsByEmail(ctx, testEmail, "upcoming", 10)
	_, _ = store.ListBookingsByEmail(ctx, testEmail, "completed", 10)
	_, _ = store.ListBookingsByEmail(ctx, testEmail, "cancelled", 10)

	// Non-existent booking detail
	detail, err := store.GetBookingDetailByEmail(ctx, testEmail, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetBookingDetailByEmail unexpected err: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil detail for non-existent booking")
	}

	// 4. Delete session
	err = store.DeleteSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("DeleteSessionByTokenHash failed: %v", err)
	}

	deletedSess, err := store.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if deletedSess != nil {
		t.Errorf("expected nil session after deletion, got %v", deletedSess)
	}
}
