package finance

import (
	"context"
	"fmt"
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

func createTestBooking(t *testing.T, pool *pgxpool.Pool, status string, totalMinor int64, email string) string {
	t.Helper()
	ctx := context.Background()
	var bookingID string
	query := `
		INSERT INTO bookings (
			room_type_id, check_in, check_out, num_rooms, num_guests,
			status, total_price_minor, currency, guest_name, guest_email
		) VALUES (
			'01900000-0000-7000-8000-000000000001',
			CURRENT_DATE + 5,
			CURRENT_DATE + 7,
			1, 2,
			$1, $2, 'IDR', 'Finance Test Guest', $3
		)
		RETURNING id::text
	`
	err := pool.QueryRow(ctx, query, status, totalMinor, email).Scan(&bookingID)
	if err != nil {
		t.Fatalf("failed to insert test booking: %v", err)
	}
	return bookingID
}

func insertTestPaymentAttempt(t *testing.T, pool *pgxpool.Pool, bookingID, invID string, amountMinor int64, status string) {
	t.Helper()
	ctx := context.Background()
	query := `
		INSERT INTO payment_attempts (
			booking_id, provider, provider_reference, amount_minor, currency, status
		) VALUES (
			$1, 'xendit', $2, $3, 'IDR', $4
		)
	`
	_, err := pool.Exec(ctx, query, bookingID, invID, amountMinor, status)
	if err != nil {
		t.Fatalf("failed to insert test payment attempt: %v", err)
	}
}

func TestPostgresStore_RefundLifecycle(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("fin_refund_%d@example.com", time.Now().UnixNano())
	bookingID := createTestBooking(t, pool, "confirmed", 2500000, testEmail)
	invID := fmt.Sprintf("inv_test_%d", time.Now().UnixNano())
	insertTestPaymentAttempt(t, pool, bookingID, invID, 2500000, "paid")

	// 1. Get refundable balance
	captured, refunded, remaining, status, fetchedInvID, err := store.GetRefundableBalance(ctx, bookingID)
	if err != nil {
		t.Fatalf("GetRefundableBalance failed: %v", err)
	}
	if captured != 2500000 || refunded != 0 || remaining != 2500000 {
		t.Errorf("unexpected balance: captured=%d, refunded=%d, remaining=%d", captured, refunded, remaining)
	}
	if status != "confirmed" {
		t.Errorf("status = %v, want confirmed", status)
	}
	if fetchedInvID != invID {
		t.Errorf("invID = %v, want %v", fetchedInvID, invID)
	}

	// 2. Create refund record
	refID := fmt.Sprintf("rfnd_tst_%d", time.Now().UnixNano())
	refund := &PaymentRefund{
		BookingID:   bookingID,
		ReferenceID: refID,
		AmountMinor: 1000000,
		Currency:    "IDR",
		Reason:      "Customer requested partial cancellation",
		Status:      "pending",
		Provider:    "xendit",
		ActorID:     "fin_user_01",
		ActorRole:   "finance",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := store.CreateRefund(ctx, refund); err != nil {
		t.Fatalf("CreateRefund failed: %v", err)
	}
	if refund.ID == "" {
		t.Fatalf("expected non-empty refund ID")
	}

	// 3. Balance should reflect the pending refund
	captured, refunded, remaining, _, _, err = store.GetRefundableBalance(ctx, bookingID)
	if err != nil {
		t.Fatalf("GetRefundableBalance failed: %v", err)
	}
	if refunded != 1000000 || remaining != 1500000 {
		t.Errorf("unexpected balance after refund: captured=%d, refunded=%d, remaining=%d", captured, refunded, remaining)
	}

	// 4. Update refund status
	providerRefID := "rfd_xen_success_01"
	if err := store.UpdateRefundStatus(ctx, refund.ID, "succeeded", providerRefID); err != nil {
		t.Fatalf("UpdateRefundStatus failed: %v", err)
	}

	// 5. List refunds by booking
	list, err := store.ListRefundsByBookingID(ctx, bookingID)
	if err != nil {
		t.Fatalf("ListRefundsByBookingID failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 refund, got %d", len(list))
	}
	if list[0].Status != "succeeded" || list[0].ProviderRefundID != providerRefID {
		t.Errorf("unexpected refund data in list: %+v", list[0])
	}

	// 6. Non-existent booking returns ErrBookingNotFound
	fakeUUID := "00000000-0000-0000-0000-000000000000"
	_, _, _, _, _, err = store.GetRefundableBalance(ctx, fakeUUID)
	if err != ErrBookingNotFound {
		t.Errorf("expected ErrBookingNotFound, got %v", err)
	}

	// 7. Non-existent refund update returns ErrRefundNotFound
	err = store.UpdateRefundStatus(ctx, fakeUUID, "failed", "")
	if err != ErrRefundNotFound {
		t.Errorf("expected ErrRefundNotFound, got %v", err)
	}
}

func TestPostgresStore_PaymentCasesLifecycle(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("fin_cases_%d@example.com", time.Now().UnixNano())
	bookingID := createTestBooking(t, pool, "expired", 1200000, testEmail)

	// 1. Create payment case
	pc := &PaymentCase{
		BookingID:         bookingID,
		CaseType:          "late_payment",
		Status:            "open",
		AmountMinor:       1200000,
		Currency:          "IDR",
		ProviderReference: "inv_late_xen_999",
		Notes:             "Payment received 15 mins after expiry",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if err := store.CreatePaymentCase(ctx, pc); err != nil {
		t.Fatalf("CreatePaymentCase failed: %v", err)
	}
	if pc.ID == "" {
		t.Fatalf("expected non-empty case ID")
	}

	// 2. Get payment case by ID
	fetched, err := store.GetPaymentCaseByID(ctx, pc.ID)
	if err != nil {
		t.Fatalf("GetPaymentCaseByID failed: %v", err)
	}
	if fetched.BookingID != bookingID || fetched.AmountMinor != 1200000 || fetched.Status != "open" {
		t.Errorf("unexpected case data: %+v", fetched)
	}

	// 3. List payment cases
	cases, err := store.ListPaymentCases(ctx, "open", 10)
	if err != nil {
		t.Fatalf("ListPaymentCases failed: %v", err)
	}
	found := false
	for _, c := range cases {
		if c.ID == pc.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("newly created case %s not found in list", pc.ID)
	}

	// 4. Resolve payment case
	err = store.ResolvePaymentCase(ctx, pc.ID, "refunded", "Manual refund dispatched by finance officer", "fin_staff_01")
	if err != nil {
		t.Fatalf("ResolvePaymentCase failed: %v", err)
	}

	resolvedCase, err := store.GetPaymentCaseByID(ctx, pc.ID)
	if err != nil {
		t.Fatalf("GetPaymentCaseByID failed: %v", err)
	}
	if resolvedCase.Status != "resolved" || resolvedCase.ResolutionAction != "refunded" {
		t.Errorf("unexpected resolved case: %+v", resolvedCase)
	}

	// 5. Resolving an already resolved case returns ErrCaseAlreadyResolved
	err = store.ResolvePaymentCase(ctx, pc.ID, "refunded", "duplicate attempt", "fin_staff_02")
	if err != ErrCaseAlreadyResolved {
		t.Errorf("expected ErrCaseAlreadyResolved, got %v", err)
	}

	// 6. Non-existent case
	fakeUUID := "00000000-0000-0000-0000-000000000000"
	_, err = store.GetPaymentCaseByID(ctx, fakeUUID)
	if err != ErrCaseNotFound {
		t.Errorf("expected ErrCaseNotFound, got %v", err)
	}
	err = store.ResolvePaymentCase(ctx, fakeUUID, "dismissed", "note", "fin_01")
	if err != ErrCaseNotFound {
		t.Errorf("expected ErrCaseNotFound, got %v", err)
	}
}

func TestPostgresStore_SummaryAndOwnership(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	testEmail := fmt.Sprintf("fin_summary_%d@example.com", time.Now().UnixNano())
	bookingID := createTestBooking(t, pool, "confirmed", 1500000, testEmail)

	// Summary test
	summary, err := store.GetReconciliationSummary(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationSummary failed: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected non-nil summary")
	}

	// Ownership verification test
	isOwner, err := store.VerifyBookingOwnership(ctx, bookingID, testEmail)
	if err != nil {
		t.Fatalf("VerifyBookingOwnership failed: %v", err)
	}
	if !isOwner {
		t.Errorf("expected isOwner true for %s", testEmail)
	}

	isOwnerFake, err := store.VerifyBookingOwnership(ctx, bookingID, "impostor@example.com")
	if err != nil {
		t.Fatalf("VerifyBookingOwnership failed: %v", err)
	}
	if isOwnerFake {
		t.Errorf("expected isOwner false for impostor")
	}
}
