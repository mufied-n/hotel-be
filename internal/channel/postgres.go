package channel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan Store menggunakan PostgreSQL pgxpool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) GetPartnerByCode(ctx context.Context, code string) (*Partner, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, provider_code, name, webhook_secret, COALESCE(safety_buffer, 1), is_active, created_at, updated_at
		FROM channel_partners
		WHERE provider_code = $1 AND is_active = true
	`, code)

	var p Partner
	err := row.Scan(&p.ID, &p.ProviderCode, &p.Name, &p.WebhookSecret, &p.SafetyBuffer, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPartnerNotFound
		}
		return nil, fmt.Errorf("scan channel_partner: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) SaveEventInbox(ctx context.Context, evt *EventInbox) error {
	if evt.ID == uuid.Nil {
		evt.ID = uuid.New()
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_event_inbox (id, provider, event_id, event_type, external_reference, payload, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, evt.ID, evt.Provider, evt.EventID, evt.EventType, evt.ExternalReference, evt.Payload, evt.Status, evt.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique violation
			return ErrDuplicateEvent
		}
		return fmt.Errorf("insert channel_event_inbox: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetEventInbox(ctx context.Context, provider, eventID string) (*EventInbox, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, provider, event_id, event_type, external_reference, payload, status, COALESCE(error_reason, ''), created_at, processed_at
		FROM channel_event_inbox
		WHERE provider = $1 AND event_id = $2
	`, provider, eventID)

	var evt EventInbox
	err := row.Scan(&evt.ID, &evt.Provider, &evt.EventID, &evt.EventType, &evt.ExternalReference, &evt.Payload, &evt.Status, &evt.ErrorReason, &evt.CreatedAt, &evt.ProcessedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan channel_event_inbox: %w", err)
	}
	return &evt, nil
}

func (s *PostgresStore) GetEventInboxByReference(ctx context.Context, provider, externalRef string) (*EventInbox, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, provider, event_id, event_type, external_reference, payload, status, COALESCE(error_reason, ''), created_at, processed_at
		FROM channel_event_inbox
		WHERE provider = $1 AND external_reference = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, provider, externalRef)

	var evt EventInbox
	err := row.Scan(&evt.ID, &evt.Provider, &evt.EventID, &evt.EventType, &evt.ExternalReference, &evt.Payload, &evt.Status, &evt.ErrorReason, &evt.CreatedAt, &evt.ProcessedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan channel_event_inbox by ref: %w", err)
	}
	return &evt, nil
}

func (s *PostgresStore) UpdateEventInboxStatus(ctx context.Context, id uuid.UUID, status, reason string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE channel_event_inbox
		SET status = $1, error_reason = $2, processed_at = $3
		WHERE id = $4
	`, status, reason, now, id)
	if err != nil {
		return fmt.Errorf("update channel_event_inbox: %w", err)
	}
	return nil
}

func (s *PostgresStore) SaveSyncIssue(ctx context.Context, issue *SyncIssue) error {
	if issue.ID == uuid.Nil {
		issue.ID = uuid.New()
	}
	if issue.CreatedAt.IsZero() {
		issue.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_sync_issues (id, provider, external_reference, event_type, room_type_id, status, reason, upgrade_room_type_id, notes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, issue.ID, issue.Provider, issue.ExternalReference, issue.EventType, issue.RoomTypeID, issue.Status, issue.Reason, issue.UpgradeRoomTypeID, issue.Notes, issue.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert channel_sync_issues: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSyncIssue(ctx context.Context, id uuid.UUID) (*SyncIssue, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, provider, external_reference, event_type, room_type_id, status, reason, upgrade_room_type_id, COALESCE(notes, ''), COALESCE(resolved_by, ''), resolved_at, created_at
		FROM channel_sync_issues
		WHERE id = $1
	`, id)

	var i SyncIssue
	err := row.Scan(&i.ID, &i.Provider, &i.ExternalReference, &i.EventType, &i.RoomTypeID, &i.Status, &i.Reason, &i.UpgradeRoomTypeID, &i.Notes, &i.ResolvedBy, &i.ResolvedAt, &i.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIssueNotFound
		}
		return nil, fmt.Errorf("scan channel_sync_issue: %w", err)
	}
	return &i, nil
}

func (s *PostgresStore) ListSyncIssues(ctx context.Context) ([]*SyncIssue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, provider, external_reference, event_type, room_type_id, status, reason, upgrade_room_type_id, COALESCE(notes, ''), COALESCE(resolved_by, ''), resolved_at, created_at
		FROM channel_sync_issues
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("query channel_sync_issues: %w", err)
	}
	defer rows.Close()

	var issues []*SyncIssue
	for rows.Next() {
		var i SyncIssue
		if err := rows.Scan(&i.ID, &i.Provider, &i.ExternalReference, &i.EventType, &i.RoomTypeID, &i.Status, &i.Reason, &i.UpgradeRoomTypeID, &i.Notes, &i.ResolvedBy, &i.ResolvedAt, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan channel_sync_issue: %w", err)
		}
		issues = append(issues, &i)
	}
	return issues, nil
}

func (s *PostgresStore) ResolveSyncIssue(ctx context.Context, id uuid.UUID, status string, upgradeRoomTypeID *uuid.UUID, resolvedBy, notes string) (*SyncIssue, error) {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE channel_sync_issues
		SET status = $1, upgrade_room_type_id = $2, resolved_by = $3, resolved_at = $4, notes = $5
		WHERE id = $6 AND status = 'QUARANTINED_CONFLICT'
	`, status, upgradeRoomTypeID, resolvedBy, now, notes, id)
	if err != nil {
		return nil, fmt.Errorf("update channel_sync_issues: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var currStatus string
		checkErr := s.pool.QueryRow(ctx, `SELECT status FROM channel_sync_issues WHERE id = $1`, id).Scan(&currStatus)
		if checkErr != nil {
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return nil, ErrIssueNotFound
			}
			return nil, fmt.Errorf("check issue: %w", checkErr)
		}
		return nil, ErrIssueAlreadyResolved
	}
	return s.GetSyncIssue(ctx, id)
}

func (s *PostgresStore) ResolveWithUpgrade(ctx context.Context, id uuid.UUID, targetRoomTypeID uuid.UUID, checkIn, checkOut time.Time, rooms int, resolvedBy, notes string) (*SyncIssue, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Verifikasi status isu saat ini dan kunci baris
	var currStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM channel_sync_issues WHERE id = $1 FOR UPDATE`, id).Scan(&currStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIssueNotFound
		}
		return nil, fmt.Errorf("lock issue: %w", err)
	}
	if currStatus != StatusQuarantinedConflict {
		return nil, ErrIssueAlreadyResolved
	}

	// 2. Kunci dan potong stok kamar target secara atomik
	fromUTC := time.Date(checkIn.Year(), checkIn.Month(), checkIn.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(checkOut.Year(), checkOut.Month(), checkOut.Day(), 0, 0, 0, 0, time.UTC)
	nights := int(toUTC.Sub(fromUTC) / (24 * time.Hour))
	if nights <= 0 {
		nights = 1
	}
	if rooms <= 0 {
		rooms = 1
	}

	tag, err := tx.Exec(ctx, `
		WITH locked AS (
			SELECT date FROM inventory
			WHERE room_type_id = $1 AND date >= $2 AND date < $3
			ORDER BY date ASC
			FOR UPDATE
		)
		UPDATE inventory i
		SET available_rooms = i.available_rooms - $4,
		    version = i.version + 1
		FROM locked l
		WHERE i.room_type_id = $1 AND i.date = l.date
		  AND i.available_rooms >= $4`,
		targetRoomTypeID.String(), fromUTC, toUTC, rooms)
	if err != nil {
		return nil, fmt.Errorf("lock target inventory: %w", err)
	}
	if tag.RowsAffected() != int64(nights) {
		return nil, ErrTargetRoomUnavailable
	}

	// 3. Perbarui status isu karantina menjadi RESOLVED
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		UPDATE channel_sync_issues
		SET status = $1, upgrade_room_type_id = $2, resolved_by = $3, resolved_at = $4, notes = $5
		WHERE id = $6`,
		StatusResolved, targetRoomTypeID, resolvedBy, now, notes, id)
	if err != nil {
		return nil, fmt.Errorf("resolve issue: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit resolve upgrade: %w", err)
	}

	return s.GetSyncIssue(ctx, id)
}
