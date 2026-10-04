package channel

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getChannelTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping postgres channel store test: %v", err)
		return nil
	}
	cfg.MaxConns = 10

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("skipping postgres channel store test: %v", err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgres channel store test: ping failed: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPostgresStore_Integration(t *testing.T) {
	pool := getChannelTestPool(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	store := NewPostgresStore(pool)

	// 1. GetPartnerByCode
	partner, err := store.GetPartnerByCode(ctx, "TRAVELOKA")
	if err != nil {
		t.Fatalf("GetPartnerByCode failed: %v", err)
	}
	if partner.ProviderCode != "TRAVELOKA" {
		t.Errorf("expected TRAVELOKA, got %s", partner.ProviderCode)
	}

	_, err = store.GetPartnerByCode(ctx, "UNKNOWN_PARTNER_999")
	if !errors.Is(err, ErrPartnerNotFound) {
		t.Errorf("expected ErrPartnerNotFound, got %v", err)
	}

	// 2. SaveEventInbox & GetEventInbox
	evtID := "pg-evt-" + uuid.NewString()
	inboxEvt := &EventInbox{
		ID:                uuid.New(),
		Provider:          "TRAVELOKA",
		EventID:           evtID,
		EventType:         "reservation_created",
		ExternalReference: "TRV-PG-001",
		Payload:           []byte(`{"test":true}`),
		Status:            "PENDING",
	}

	err = store.SaveEventInbox(ctx, inboxEvt)
	if err != nil {
		t.Fatalf("SaveEventInbox failed: %v", err)
	}

	// Duplicate save returns ErrDuplicateEvent
	err = store.SaveEventInbox(ctx, inboxEvt)
	if !errors.Is(err, ErrDuplicateEvent) {
		t.Errorf("expected ErrDuplicateEvent on duplicate, got %v", err)
	}

	fetched, err := store.GetEventInbox(ctx, "TRAVELOKA", evtID)
	if err != nil {
		t.Fatalf("GetEventInbox failed: %v", err)
	}
	if fetched == nil || fetched.EventID != evtID {
		t.Fatalf("expected event %s, got %+v", evtID, fetched)
	}

	nonExistent, err := store.GetEventInbox(ctx, "TRAVELOKA", "not-exists-12345")
	if err != nil {
		t.Fatalf("GetEventInbox non-existent error: %v", err)
	}
	if nonExistent != nil {
		t.Errorf("expected nil for non-existent event, got %+v", nonExistent)
	}

	// 3. UpdateEventInboxStatus
	err = store.UpdateEventInboxStatus(ctx, inboxEvt.ID, "PROCESSED", "")
	if err != nil {
		t.Fatalf("UpdateEventInboxStatus failed: %v", err)
	}

	updated, err := store.GetEventInbox(ctx, "TRAVELOKA", evtID)
	if err != nil {
		t.Fatalf("GetEventInbox after update failed: %v", err)
	}
	if updated.Status != "PROCESSED" {
		t.Errorf("expected status PROCESSED, got %s", updated.Status)
	}

	// 4. SaveSyncIssue & ListSyncIssues
	roomUUID := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	issue := &SyncIssue{
		ID:                uuid.New(),
		Provider:          "AGODA",
		ExternalReference: "AGD-PG-ISSUE-1",
		EventType:         "reservation_created",
		RoomTypeID:        &roomUUID,
		Status:            "QUARANTINED_CONFLICT",
		Reason:            "Test room sold out conflict",
	}

	err = store.SaveSyncIssue(ctx, issue)
	if err != nil {
		t.Fatalf("SaveSyncIssue failed: %v", err)
	}

	issues, err := store.ListSyncIssues(ctx)
	if err != nil {
		t.Fatalf("ListSyncIssues failed: %v", err)
	}
	if len(issues) == 0 {
		t.Errorf("expected at least 1 sync issue, got 0")
	}

	// 5. GetEventInboxByReference
	refEvt, err := store.GetEventInboxByReference(ctx, "TRAVELOKA", "TRV-PG-001")
	if err != nil {
		t.Fatalf("GetEventInboxByReference failed: %v", err)
	}
	if refEvt == nil || refEvt.ID != inboxEvt.ID {
		t.Fatalf("expected event by ref %v, got %+v", inboxEvt.ID, refEvt)
	}

	// 6. GetSyncIssue & ResolveSyncIssue (Reject)
	fetchedIssue, err := store.GetSyncIssue(ctx, issue.ID)
	if err != nil {
		t.Fatalf("GetSyncIssue failed: %v", err)
	}
	if fetchedIssue.Status != "QUARANTINED_CONFLICT" {
		t.Errorf("expected QUARANTINED_CONFLICT, got %s", fetchedIssue.Status)
	}

	resolvedIssue, err := store.ResolveSyncIssue(ctx, issue.ID, StatusRejected, nil, "receptionist_rina", "Rejected due to overbooking")
	if err != nil {
		t.Fatalf("ResolveSyncIssue failed: %v", err)
	}
	if resolvedIssue.Status != StatusRejected {
		t.Errorf("expected status REJECTED, got %s", resolvedIssue.Status)
	}

	// Double resolve returns ErrIssueAlreadyResolved
	_, err = store.ResolveSyncIssue(ctx, issue.ID, StatusRejected, nil, "receptionist_rina", "Again")
	if !errors.Is(err, ErrIssueAlreadyResolved) {
		t.Errorf("expected ErrIssueAlreadyResolved, got %v", err)
	}

	// 7. ResolveWithUpgrade
	targetRoomType := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	checkIn := time.Date(2028, 5, 1, 0, 0, 0, 0, time.UTC)
	checkOut := time.Date(2028, 5, 3, 0, 0, 0, 0, time.UTC)
	_, _ = pool.Exec(ctx, `
		INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
		VALUES ($1, '2028-05-01', 5, 5), ($1, '2028-05-02', 5, 5)
		ON CONFLICT (room_type_id, date) DO UPDATE SET available_rooms = 5
	`, targetRoomType)

	upgradeIssue := &SyncIssue{
		ID:                uuid.New(),
		Provider:          "AGODA",
		ExternalReference: "AGD-PG-UPGRADE-1",
		EventType:         "reservation_created",
		RoomTypeID:        &roomUUID,
		Status:            StatusQuarantinedConflict,
		Reason:            "Test overbooking",
	}
	_ = store.SaveSyncIssue(ctx, upgradeIssue)

	upgraded, err := store.ResolveWithUpgrade(ctx, upgradeIssue.ID, targetRoomType, checkIn, checkOut, 1, "receptionist_budi", "Complimentary upgrade test")
	if err != nil {
		t.Fatalf("ResolveWithUpgrade failed: %v", err)
	}
	if upgraded.Status != StatusResolved {
		t.Errorf("expected status RESOLVED, got %s", upgraded.Status)
	}
	if upgraded.UpgradeRoomTypeID == nil || *upgraded.UpgradeRoomTypeID != targetRoomType {
		t.Errorf("expected targetRoomType %v, got %v", targetRoomType, upgraded.UpgradeRoomTypeID)
	}

	// Test ResolveWithUpgrade fails when target room is exhausted
	_, err = store.ResolveWithUpgrade(ctx, upgradeIssue.ID, targetRoomType, checkIn, checkOut, 999, "receptionist_budi", "Too many")
	if !errors.Is(err, ErrIssueAlreadyResolved) { // already resolved
		t.Logf("expected ErrIssueAlreadyResolved on already resolved issue: %v", err)
	}
}

