package http

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/hotel-booking/internal/api/http/middleware"
)

const (
	// idempotencyReserveTTL membatasi umur reservasi in-progress; reservasi yang ditinggal
	// proses mati dapat diambil alih setelah lewat TTL ini (BE-R08).
	idempotencyReserveTTL = 2 * time.Minute
	// idempotencyResultTTL adalah masa replay respons yang sudah selesai.
	idempotencyResultTTL = 24 * time.Hour
)

type IdempotencyRecord = middleware.IdempotencyRecord
type IdempotencyStore = middleware.IdempotencyStore

var (
	ScopeIdempotencyKey = middleware.ScopeIdempotencyKey
	HashBody            = middleware.HashBody
)

func hashBody(b []byte) string {
	return middleware.HashBody(b)
}

// MemoryIdempotencyStore adalah implementasi in-memory thread-safe untuk unit testing dan development.
type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]IdempotencyRecord
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{records: make(map[string]IdempotencyRecord)}
}

func (s *MemoryIdempotencyStore) Reserve(_ context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if rec, ok := s.records[key]; ok && now.Before(rec.ExpiresAt) {
		return rec, false, nil
	}
	rec := IdempotencyRecord{Key: key, RequestHash: requestHash, CreatedAt: now, ExpiresAt: now.Add(idempotencyReserveTTL)}
	s.records[key] = rec
	return rec, true, nil
}

func (s *MemoryIdempotencyStore) Complete(_ context.Context, rec IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.Key] = rec
	return nil
}

func (s *MemoryIdempotencyStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

// PostgresIdempotencyStore adalah implementasi persistent berbasis PostgreSQL (BE-G09, BE-R08).
type PostgresIdempotencyStore struct {
	pool *pgxpool.Pool
}

func NewPostgresIdempotencyStore(pool *pgxpool.Pool) *PostgresIdempotencyStore {
	return &PostgresIdempotencyStore{pool: pool}
}

// Reserve memakai satu pernyataan INSERT ... ON CONFLICT: baris baru atau baris kedaluwarsa
// diambil alih secara atomik; baris aktif dibiarkan dan dibaca kembali.
func (s *PostgresIdempotencyStore) Reserve(ctx context.Context, key, requestHash string) (IdempotencyRecord, bool, error) {
	// Maksimal 3 percobaan: baris aktif bisa dilepas (Release) di antara INSERT dan SELECT.
	for range 3 {
		var rec IdempotencyRecord
		err := s.pool.QueryRow(ctx, `
			INSERT INTO idempotency_keys (key, request_hash, response_code, response_body, created_at, expires_at)
			VALUES ($1, $2, 0, '', NOW(), NOW() + $3::interval)
			ON CONFLICT (key) DO UPDATE
				SET request_hash = EXCLUDED.request_hash, response_code = 0, response_body = '',
				    created_at = NOW(), expires_at = EXCLUDED.expires_at
				WHERE idempotency_keys.expires_at <= NOW()
			RETURNING key, request_hash, response_code, response_body, created_at, expires_at`,
			key, requestHash, idempotencyReserveTTL.String(),
		).Scan(&rec.Key, &rec.RequestHash, &rec.ResponseCode, &rec.ResponseBody, &rec.CreatedAt, &rec.ExpiresAt)
		if err == nil {
			return rec, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return IdempotencyRecord{}, false, err
		}
		// Key aktif milik request lain: baca kondisi terkini.
		err = s.pool.QueryRow(ctx, `
			SELECT key, request_hash, response_code, response_body, created_at, expires_at
			FROM idempotency_keys WHERE key = $1`, key,
		).Scan(&rec.Key, &rec.RequestHash, &rec.ResponseCode, &rec.ResponseBody, &rec.CreatedAt, &rec.ExpiresAt)
		if err == nil {
			return rec, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return IdempotencyRecord{}, false, err
		}
	}
	return IdempotencyRecord{}, false, errors.New("idempotency: reserve contention")
}

func (s *PostgresIdempotencyStore) Complete(ctx context.Context, rec IdempotencyRecord) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE idempotency_keys
		SET response_code = $2, response_body = $3, expires_at = $4
		WHERE key = $1 AND request_hash = $5`,
		rec.Key, rec.ResponseCode, rec.ResponseBody, rec.ExpiresAt, rec.RequestHash)
	return err
}

func (s *PostgresIdempotencyStore) Release(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE key = $1 AND response_code = 0`, key)
	return err
}
