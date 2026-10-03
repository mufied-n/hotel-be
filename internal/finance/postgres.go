package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan finance.Store dengan koneksi pgxpool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) GetRefundableBalance(ctx context.Context, bookingID string) (capturedMinor, refundedMinor, remainingMinor int64, bookingStatus string, invoiceID string, err error) {
	query := `
		SELECT b.status, b.total_price_minor,
		       COALESCE((SELECT SUM(r.amount_minor) FROM payment_refunds r WHERE r.booking_id = b.id AND r.status IN ('pending', 'succeeded')), 0) as total_refunded,
		       COALESCE((SELECT provider_reference FROM payment_attempts pa WHERE pa.booking_id = b.id AND pa.status IN ('success', 'paid') ORDER BY created_at DESC LIMIT 1), '') as invoice_id
		FROM bookings b
		WHERE b.id = $1
		FOR UPDATE OF b;
	`
	var (
		status    string
		total     int64
		refunded  int64
		invID     string
	)
	err = s.pool.QueryRow(ctx, query, bookingID).Scan(&status, &total, &refunded, &invID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, "", "", ErrBookingNotFound
		}
		return 0, 0, 0, "", "", fmt.Errorf("finance_store.get_balance: %w", err)
	}

	remaining := total - refunded
	if remaining < 0 {
		remaining = 0
	}

	return total, refunded, remaining, status, invID, nil
}

func (s *PostgresStore) CreateRefund(ctx context.Context, r *PaymentRefund) error {
	query := `
		INSERT INTO payment_refunds (
			booking_id, reference_id, amount_minor, currency, reason, status,
			provider, provider_refund_id, actor_id, actor_role, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		r.BookingID, r.ReferenceID, r.AmountMinor, r.Currency, r.Reason, r.Status,
		r.Provider, r.ProviderRefundID, r.ActorID, r.ActorRole, r.CreatedAt, r.UpdatedAt,
	).Scan(&r.ID)
	if err != nil {
		return fmt.Errorf("finance_store.create_refund: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateRefundStatus(ctx context.Context, refundID, status, providerRefundID string) error {
	query := `
		UPDATE payment_refunds
		SET status = $2, provider_refund_id = $3, updated_at = NOW()
		WHERE id = $1
	`
	res, err := s.pool.Exec(ctx, query, refundID, status, providerRefundID)
	if err != nil {
		return fmt.Errorf("finance_store.update_refund: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrRefundNotFound
	}
	return nil
}

func (s *PostgresStore) ListRefundsByBookingID(ctx context.Context, bookingID string) ([]PaymentRefund, error) {
	query := `
		SELECT id, booking_id, COALESCE(payment_attempt_id::text, ''), reference_id,
		       amount_minor, currency, reason, status, provider,
		       COALESCE(provider_refund_id, ''), actor_id, actor_role, created_at, updated_at
		FROM payment_refunds
		WHERE booking_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, bookingID)
	if err != nil {
		return nil, fmt.Errorf("finance_store.list_refunds: %w", err)
	}
	defer rows.Close()

	var results []PaymentRefund
	for rows.Next() {
		var r PaymentRefund
		var paID string
		if err := rows.Scan(
			&r.ID, &r.BookingID, &paID, &r.ReferenceID,
			&r.AmountMinor, &r.Currency, &r.Reason, &r.Status, &r.Provider,
			&r.ProviderRefundID, &r.ActorID, &r.ActorRole, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("finance_store.scan_refund: %w", err)
		}
		if paID != "" {
			r.PaymentAttemptID = &paID
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finance_store.rows_err: %w", err)
	}
	if results == nil {
		results = []PaymentRefund{}
	}
	return results, nil
}

func (s *PostgresStore) CreatePaymentCase(ctx context.Context, pc *PaymentCase) error {
	query := `
		INSERT INTO payment_cases (
			booking_id, case_type, status, amount_minor, currency,
			provider_reference, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		pc.BookingID, pc.CaseType, pc.Status, pc.AmountMinor, pc.Currency,
		pc.ProviderReference, pc.Notes, pc.CreatedAt, pc.UpdatedAt,
	).Scan(&pc.ID)
	if err != nil {
		return fmt.Errorf("finance_store.create_case: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetPaymentCaseByID(ctx context.Context, caseID string) (*PaymentCase, error) {
	query := `
		SELECT id, COALESCE(booking_id::text, ''), case_type, status,
		       amount_minor, currency, provider_reference, notes,
		       COALESCE(resolved_by, ''), resolved_at, COALESCE(resolution_action, ''),
		       created_at, updated_at
		FROM payment_cases
		WHERE id = $1
	`
	var pc PaymentCase
	err := s.pool.QueryRow(ctx, query, caseID).Scan(
		&pc.ID, &pc.BookingID, &pc.CaseType, &pc.Status,
		&pc.AmountMinor, &pc.Currency, &pc.ProviderReference, &pc.Notes,
		&pc.ResolvedBy, &pc.ResolvedAt, &pc.ResolutionAction,
		&pc.CreatedAt, &pc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCaseNotFound
		}
		return nil, fmt.Errorf("finance_store.get_case: %w", err)
	}
	return &pc, nil
}

func (s *PostgresStore) ListPaymentCases(ctx context.Context, status string, limit int) ([]PaymentCase, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT id, COALESCE(booking_id::text, ''), case_type, status,
		       amount_minor, currency, provider_reference, notes,
		       COALESCE(resolved_by, ''), resolved_at, COALESCE(resolution_action, ''),
		       created_at, updated_at
		FROM payment_cases
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("finance_store.list_cases: %w", err)
	}
	defer rows.Close()

	var results []PaymentCase
	for rows.Next() {
		var pc PaymentCase
		if err := rows.Scan(
			&pc.ID, &pc.BookingID, &pc.CaseType, &pc.Status,
			&pc.AmountMinor, &pc.Currency, &pc.ProviderReference, &pc.Notes,
			&pc.ResolvedBy, &pc.ResolvedAt, &pc.ResolutionAction,
			&pc.CreatedAt, &pc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("finance_store.scan_case: %w", err)
		}
		results = append(results, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finance_store.cases_rows_err: %w", err)
	}
	if results == nil {
		results = []PaymentCase{}
	}
	return results, nil
}

func (s *PostgresStore) ResolvePaymentCase(ctx context.Context, caseID, action, notes, resolvedBy string) error {
	query := `
		UPDATE payment_cases
		SET status = 'resolved', resolution_action = $2,
		    notes = CASE WHEN notes = '' THEN $3 ELSE notes || ' | ' || $3 END,
		    resolved_by = $4, resolved_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status != 'resolved'
	`
	res, err := s.pool.Exec(ctx, query, caseID, action, notes, resolvedBy)
	if err != nil {
		return fmt.Errorf("finance_store.resolve_case: %w", err)
	}
	if res.RowsAffected() == 0 {
		// Cek apakah tidak ditemukan atau sudah resolved
		existing, err := s.GetPaymentCaseByID(ctx, caseID)
		if err != nil {
			return err
		}
		if existing.Status == "resolved" {
			return ErrCaseAlreadyResolved
		}
		return ErrCaseNotFound
	}
	return nil
}

func (s *PostgresStore) GetReconciliationSummary(ctx context.Context) (*ReconciliationSummary, error) {
	query := `
		SELECT
			COALESCE((SELECT SUM(amount_minor) FROM payment_attempts WHERE status IN ('success', 'paid')), 0) as total_settled,
			COALESCE((SELECT SUM(amount_minor) FROM payment_refunds WHERE status = 'succeeded'), 0) as total_refunded,
			COALESCE((SELECT COUNT(*) FROM payment_cases WHERE status = 'open'), 0) as open_cases,
			COALESCE((SELECT COUNT(*) FROM payment_refunds WHERE status = 'succeeded'), 0) as total_refunds
	`
	var (
		settled  int64
		refunded int64
		cases    int
		refunds  int
	)
	err := s.pool.QueryRow(ctx, query).Scan(&settled, &refunded, &cases, &refunds)
	if err != nil {
		return nil, fmt.Errorf("finance_store.get_summary: %w", err)
	}

	return &ReconciliationSummary{
		TotalSettledMinor:  settled,
		TotalRefundedMinor: refunded,
		NetCapturedMinor:   settled - refunded,
		OpenCasesCount:     cases,
		TotalRefundsCount:  refunds,
	}, nil
}

func (s *PostgresStore) VerifyBookingOwnership(ctx context.Context, bookingID, email string) (bool, error) {
	query := `SELECT COUNT(*) FROM bookings WHERE id = $1 AND guest_email = $2`
	var count int
	err := s.pool.QueryRow(ctx, query, bookingID, email).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("finance_store.verify_owner: %w", err)
	}
	return count > 0, nil
}

var _ Store = (*PostgresStore)(nil)
