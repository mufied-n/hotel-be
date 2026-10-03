package staffauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan Store di atas staff_users dan staff_sessions.
type PostgresStore struct{ Pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{Pool: pool} }

func (s *PostgresStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, username, role, full_name, COALESCE(password_hash, ''), is_active, failed_attempts, locked_until
		FROM staff_users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Role, &u.FullName, &u.PasswordHash, &u.Active, &u.FailedAttempts, &u.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PostgresStore) RecordFailure(ctx context.Context, id string, failedAttempts int, lockedUntil *time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE staff_users SET failed_attempts = $2, locked_until = $3 WHERE id = $1`, id, failedAttempts, lockedUntil)
	return err
}

func (s *PostgresStore) RecordSuccess(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE staff_users SET failed_attempts = 0, locked_until = NULL WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) CreateSession(ctx context.Context, staffID, tokenHash string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO staff_sessions (staff_id, token_hash, expires_at) VALUES ($1, $2, $3)`, staffID, tokenHash, expiresAt)
	return err
}

func (s *PostgresStore) GetSessionPrincipal(ctx context.Context, tokenHash string, now time.Time) (*Principal, error) {
	var p Principal
	err := s.Pool.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.role, u.full_name
		FROM staff_sessions s JOIN staff_users u ON u.id = s.staff_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2 AND u.is_active`,
		tokenHash, now,
	).Scan(&p.StaffID, &p.Username, &p.Role, &p.FullName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PostgresStore) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE staff_sessions SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return err
}

// SetPassword mengganti hash, mereset lockout, dan mencabut seluruh sesi aktif user dalam satu pernyataan.
func (s *PostgresStore) SetPassword(ctx context.Context, username, passwordHash string) error {
	var id string
	err := s.Pool.QueryRow(ctx, `
		WITH upd AS (
			UPDATE staff_users SET password_hash = $2, failed_attempts = 0, locked_until = NULL
			WHERE username = $1 RETURNING id
		), rev AS (
			UPDATE staff_sessions SET revoked_at = NOW()
			WHERE revoked_at IS NULL AND staff_id IN (SELECT id FROM upd)
		)
		SELECT id::text FROM upd`, username, passwordHash,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("staffauth: set password: %w", err)
	}
	return nil
}
