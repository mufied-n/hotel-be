package guest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:25432/hotel_booking?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping postgres store test: %v", err)
		return nil
	}
	cfg.MaxConns = 25

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
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

func TestPostgresStore_AtomicCooldown(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("cooldown_%d@example.com", time.Now().UnixNano())
	now := time.Now().UTC()

	ch1 := &Challenge{
		Email:       testEmail,
		CodeHash:    HashString("111111"),
		Attempts:    0,
		MaxAttempts: 3,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
	}

	// 1. Initial creation succeeds
	if err := store.CreateChallengeWithCooldown(ctx, ch1, 60*time.Second); err != nil {
		t.Fatalf("first challenge creation failed: %v", err)
	}

	// 2. Second creation immediately within 60s cooldown fails with ErrRateLimited
	ch2 := &Challenge{
		Email:       testEmail,
		CodeHash:    HashString("222222"),
		Attempts:    0,
		MaxAttempts: 3,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now.Add(5 * time.Second),
	}
	err := store.CreateChallengeWithCooldown(ctx, ch2, 60*time.Second)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got: %v", err)
	}

	// 3. Concurrent challenge creation for a different email: exactly 1 succeeds, others rate limited
	concurrentEmail := fmt.Sprintf("concurrent_cd_%d@example.com", time.Now().UnixNano())
	concurrency := 8
	var wg sync.WaitGroup
	var successCount int64
	var rateLimitedCount int64

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			c := &Challenge{
				Email:       concurrentEmail,
				CodeHash:    HashString(fmt.Sprintf("%06d", idx)),
				Attempts:    0,
				MaxAttempts: 3,
				ExpiresAt:   now.Add(10 * time.Minute),
				CreatedAt:   time.Now().UTC(),
			}
			err := store.CreateChallengeWithCooldown(ctx, c, 60*time.Second)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, ErrRateLimited) {
				atomic.AddInt64(&rateLimitedCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful challenge creation, got %d", successCount)
	}
	if rateLimitedCount != int64(concurrency-1) {
		t.Errorf("expected %d rate-limited challenges, got %d", concurrency-1, rateLimitedCount)
	}
}

func TestPostgresStore_AtomicVerifyAndConsume(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("verify_%d@example.com", time.Now().UnixNano())
	now := time.Now().UTC()
	validCode := "654321"
	validHash := HashString(validCode)

	ch := &Challenge{
		Email:       testEmail,
		CodeHash:    validHash,
		Attempts:    0,
		MaxAttempts: 3,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
	}
	if err := store.CreateChallenge(ctx, ch); err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	// 1. Wrong code attempt 1 -> returns ErrInvalidOrExpiredCode
	sess := &GuestSession{
		GuestEmail: testEmail,
		TokenHash:  HashString("token_dummy_1"),
		ExpiresAt:  now.Add(24 * time.Hour),
		CreatedAt:  now,
	}
	_, err := store.VerifyAndConsumeChallenge(ctx, testEmail, HashString("000001"), now, sess)
	if !errors.Is(err, ErrInvalidOrExpiredCode) {
		t.Fatalf("expected ErrInvalidOrExpiredCode on wrong attempt 1, got %v", err)
	}

	// 2. Wrong code attempt 2 -> returns ErrInvalidOrExpiredCode
	_, err = store.VerifyAndConsumeChallenge(ctx, testEmail, HashString("000002"), now, sess)
	if !errors.Is(err, ErrInvalidOrExpiredCode) {
		t.Fatalf("expected ErrInvalidOrExpiredCode on wrong attempt 2, got %v", err)
	}

	// 3. Wrong code attempt 3 -> hits max attempts -> returns ErrMaxAttemptsExceeded
	_, err = store.VerifyAndConsumeChallenge(ctx, testEmail, HashString("000003"), now, sess)
	if !errors.Is(err, ErrMaxAttemptsExceeded) {
		t.Fatalf("expected ErrMaxAttemptsExceeded on wrong attempt 3, got %v", err)
	}

	// 4. Correct code now still fails because max attempts was reached
	_, err = store.VerifyAndConsumeChallenge(ctx, testEmail, validHash, now, sess)
	if !errors.Is(err, ErrMaxAttemptsExceeded) {
		t.Fatalf("expected ErrMaxAttemptsExceeded even with valid code after lockout, got %v", err)
	}
}

func TestPostgresStore_AtomicVerify_ConcurrentSingleWinner(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("race_%d@example.com", time.Now().UnixNano())
	now := time.Now().UTC()
	validCode := "888999"
	validHash := HashString(validCode)

	ch := &Challenge{
		Email:       testEmail,
		CodeHash:    validHash,
		Attempts:    0,
		MaxAttempts: 5,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
	}
	if err := store.CreateChallenge(ctx, ch); err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	concurrency := 10
	var wg sync.WaitGroup
	var successCount int64
	var failCount int64

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			sess := &GuestSession{
				GuestEmail: testEmail,
				TokenHash:  HashString(fmt.Sprintf("race_token_%d_%d", time.Now().UnixNano(), idx)),
				ExpiresAt:  now.Add(24 * time.Hour),
				CreatedAt:  now,
			}
			created, err := store.VerifyAndConsumeChallenge(ctx, testEmail, validHash, now, sess)
			if err == nil && created != nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, ErrInvalidOrExpiredCode) {
				atomic.AddInt64(&failCount, 1)
			} else {
				t.Logf("goroutine %d unexpected error: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 winner in concurrent verification race, got %d", successCount)
	}
	if failCount != int64(concurrency-1) {
		t.Errorf("expected %d verification attempts rejected as already consumed, got %d", concurrency-1, failCount)
	}
}

func TestPostgresStore_GetBookingReceiptData(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("receipt_%d@example.com", time.Now().UnixNano())

	// 1. Non-existent booking returns nil, nil
	r, err := store.GetBookingReceiptData(ctx, testEmail, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error for non-existent booking: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil receipt, got %v", r)
	}

	// 2. Insert confirmed booking and fetch receipt
	var bookingID string
	query := `
		INSERT INTO bookings (
			room_type_id, check_in, check_out, num_rooms, num_guests,
			status, total_price_minor, currency, guest_name, guest_email,
			guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
			room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor
		) VALUES (
			'01900000-0000-7000-8000-000000000001', '2026-11-01', '2026-11-03', 1, 2,
			'confirmed', 150000000, 'IDR', 'Tamu Resit', $1,
			'token_receipt_test', 'room_only', 'flexible_48h', 'Free cancellation up to 48h',
			150000000, 0, 0, 0
		) RETURNING id
	`
	err = pool.QueryRow(ctx, query, testEmail).Scan(&bookingID)
	if err != nil {
		t.Fatalf("insert test booking failed: %v", err)
	}

	r, err = store.GetBookingReceiptData(ctx, testEmail, bookingID)
	if err != nil {
		t.Fatalf("GetBookingReceiptData failed: %v", err)
	}
	if r == nil {
		t.Fatalf("expected non-nil receipt")
	}
	if r.BookingID != bookingID {
		t.Errorf("expected booking ID %s, got %s", bookingID, r.BookingID)
	}
	if r.GuestDetails.Email != testEmail {
		t.Errorf("expected guest email %s, got %s", testEmail, r.GuestDetails.Email)
	}
	if r.StayDetails.TotalNights != 2 {
		t.Errorf("expected 2 nights, got %d", r.StayDetails.TotalNights)
	}
}

func TestPostgresStore_CaseInsensitiveEmailQueries(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	rawEmail := fmt.Sprintf("  Case.Test_%d@EXAMPLE.Com  ", time.Now().UnixNano())
	canonicalEmail := strings.ToLower(strings.TrimSpace(rawEmail))
	expiresAt := time.Now().UTC().Add(15 * time.Minute)

	var bookingID string
	query := `
		INSERT INTO bookings (
			room_type_id, check_in, check_out, num_rooms, num_guests,
			status, total_price_minor, currency, guest_name, guest_email,
			guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
			room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor,
			expires_at
		) VALUES (
			'01900000-0000-7000-8000-000000000001', '2026-11-10', '2026-11-12', 1, 2,
			'confirmed', 120000000, 'IDR', 'Mixed Case Guest', $1,
			'token_case_test', 'BAR_RO', 'flexible_48h', 'Free cancellation up to 48h',
			120000000, 0, 0, 0, $2
		) RETURNING id
	`
	err := pool.QueryRow(ctx, query, rawEmail, expiresAt).Scan(&bookingID)
	if err != nil {
		t.Fatalf("insert test booking failed: %v", err)
	}

	// 1. Count active bookings using lowercase canonical email
	count, err := store.CountActiveBookingsByEmail(ctx, canonicalEmail)
	if err != nil {
		t.Fatalf("CountActiveBookingsByEmail failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 active booking, got %d", count)
	}

	// 2. List bookings using lowercase canonical email
	list, err := store.ListBookingsByEmail(ctx, canonicalEmail, "all", 10)
	if err != nil {
		t.Fatalf("ListBookingsByEmail failed: %v", err)
	}
	found := false
	for _, b := range list {
		if b.ID == bookingID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("booking %s not found in list for email %s", bookingID, canonicalEmail)
	}

	// 3. Get booking detail and verify cancellation_policy, rate_plan_code, and expires_at
	detail, err := store.GetBookingDetailByEmail(ctx, canonicalEmail, bookingID)
	if err != nil {
		t.Fatalf("GetBookingDetailByEmail failed: %v", err)
	}
	if detail == nil {
		t.Fatalf("expected non-nil detail")
	}
	if detail.CancellationPolicy != "flexible_48h" {
		t.Errorf("expected cancellation_policy 'flexible_48h', got '%s'", detail.CancellationPolicy)
	}
	if detail.RatePlanCode != "BAR_RO" {
		t.Errorf("expected rate_plan_code 'BAR_RO', got '%s'", detail.RatePlanCode)
	}
	if detail.ExpiresAt == nil {
		t.Errorf("expected non-nil expires_at")
	}

	// 4. Get booking receipt data using lowercase canonical email
	receipt, err := store.GetBookingReceiptData(ctx, canonicalEmail, bookingID)
	if err != nil {
		t.Fatalf("GetBookingReceiptData failed: %v", err)
	}
	if receipt == nil {
		t.Fatalf("expected non-nil receipt")
	}
	if receipt.BookingID != bookingID {
		t.Errorf("expected receipt booking ID %s, got %s", bookingID, receipt.BookingID)
	}
}
