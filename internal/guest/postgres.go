package guest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan guest.Store menggunakan connection pool pgxpool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateChallenge(ctx context.Context, c *Challenge) error {
	query := `
		INSERT INTO guest_auth_challenges (
			email, code_hash, attempts, max_attempts, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		c.Email, c.CodeHash, c.Attempts, c.MaxAttempts, c.ExpiresAt, c.CreatedAt,
	).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("guest_store.create_challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetLatestActiveChallenge(ctx context.Context, email string) (*Challenge, error) {
	query := `
		SELECT id, email, code_hash, attempts, max_attempts, expires_at, verified_at, created_at
		FROM guest_auth_challenges
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var c Challenge
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&c.ID, &c.Email, &c.CodeHash, &c.Attempts, &c.MaxAttempts, &c.ExpiresAt, &c.VerifiedAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("guest_store.get_latest_challenge: %w", err)
	}
	return &c, nil
}

func (s *PostgresStore) UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error {
	query := `UPDATE guest_auth_challenges SET attempts = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, attempts)
	if err != nil {
		return fmt.Errorf("guest_store.update_challenge_attempts: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkChallengeVerified(ctx context.Context, id string, verifiedAt time.Time) error {
	query := `UPDATE guest_auth_challenges SET verified_at = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, verifiedAt)
	if err != nil {
		return fmt.Errorf("guest_store.mark_challenge_verified: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, sess *GuestSession) error {
	query := `
		INSERT INTO guest_sessions (
			guest_email, token_hash, expires_at, last_active_at, created_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		sess.GuestEmail, sess.TokenHash, sess.ExpiresAt, sess.LastActiveAt, sess.CreatedAt,
	).Scan(&sess.ID)
	if err != nil {
		return fmt.Errorf("guest_store.create_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*GuestSession, error) {
	query := `
		SELECT id, guest_email, token_hash, expires_at, last_active_at, created_at
		FROM guest_sessions
		WHERE token_hash = $1
	`
	var sess GuestSession
	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&sess.ID, &sess.GuestEmail, &sess.TokenHash, &sess.ExpiresAt, &sess.LastActiveAt, &sess.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("guest_store.get_session: %w", err)
	}
	return &sess, nil
}

func (s *PostgresStore) TouchSession(ctx context.Context, id string, lastActiveAt, expiresAt time.Time) error {
	query := `UPDATE guest_sessions SET last_active_at = $2, expires_at = $3 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, lastActiveAt, expiresAt)
	if err != nil {
		return fmt.Errorf("guest_store.touch_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM guest_sessions WHERE token_hash = $1`
	_, err := s.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("guest_store.delete_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountActiveBookingsByEmail(ctx context.Context, email string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM bookings
		WHERE guest_email = $1 AND status IN ('pending', 'confirmed', 'checked_in')
	`
	var count int
	err := s.pool.QueryRow(ctx, query, email).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("guest_store.count_active_bookings: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]BookingSummary, error) {
	baseQuery := `
		SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
		       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
		       b.num_rooms, b.num_guests, b.status, b.total_price_minor, b.currency, b.created_at
		FROM bookings b
		LEFT JOIN room_types r ON r.id = b.room_type_id
		WHERE b.guest_email = $1
	`
	var rows pgx.Rows
	var err error

	switch status {
	case "upcoming":
		q := baseQuery + ` AND b.status IN ('pending', 'confirmed') AND b.check_out >= CURRENT_DATE ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	case "completed":
		q := baseQuery + ` AND (b.status = 'checked_out' OR (b.status = 'confirmed' AND b.check_out < CURRENT_DATE)) ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	case "cancelled":
		q := baseQuery + ` AND b.status IN ('cancelled', 'expired', 'failed', 'no_show') ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	default: // "all" atau kosong
		q := baseQuery + ` ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("guest_store.list_bookings: %w", err)
	}
	defer rows.Close()

	var results []BookingSummary
	for rows.Next() {
		var item BookingSummary
		if err := rows.Scan(
			&item.ID, &item.RoomTypeID, &item.RoomTypeName,
			&item.CheckIn, &item.CheckOut, &item.NumRooms, &item.NumGuests,
			&item.Status, &item.TotalPriceMinor, &item.Currency, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("guest_store.scan_booking: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("guest_store.rows_err: %w", err)
	}

	if results == nil {
		results = []BookingSummary{}
	}
	return results, nil
}

func (s *PostgresStore) GetBookingDetailByEmail(ctx context.Context, email, bookingID string) (*BookingDetail, error) {
	query := `
		SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
		       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
		       b.num_rooms, b.num_guests, b.status, b.total_price_minor, b.currency,
		       b.guest_name, b.guest_email, COALESCE(b.guest_phone, ''),
		       COALESCE(b.estimated_arrival_time, ''), COALESCE(b.special_requests, ''),
		       b.created_at
		FROM bookings b
		LEFT JOIN room_types r ON r.id = b.room_type_id
		WHERE b.id = $1 AND b.guest_email = $2
	`
	var d BookingDetail
	err := s.pool.QueryRow(ctx, query, bookingID, email).Scan(
		&d.ID, &d.RoomTypeID, &d.RoomTypeName,
		&d.CheckIn, &d.CheckOut, &d.NumRooms, &d.NumGuests,
		&d.Status, &d.TotalPriceMinor, &d.Currency,
		&d.GuestName, &d.GuestEmail, &d.GuestPhone,
		&d.EstimatedArrivalTime, &d.SpecialRequests, &d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil // IDOR-safe: mengembalikan nil agar service mengonversi ke ErrBookingNotFound
		}
		return nil, fmt.Errorf("guest_store.get_booking_detail: %w", err)
	}
	return &d, nil
}

var _ Store = (*PostgresStore)(nil)
