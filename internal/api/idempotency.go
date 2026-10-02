package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrIdempotencyNotFound = errors.New("idempotency: record not found")

// IdempotencyRecord menyimpan snapshot respons API untuk replay yang aman (BE-G09, IETF draft).
type IdempotencyRecord struct {
	Key          string    `json:"key"`
	RequestHash  string    `json:"request_hash"`
	ResponseCode int       `json:"response_code"`
	ResponseBody string    `json:"response_body"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IdempotencyStore adalah kontrak penyimpanan idempotency keys.
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (IdempotencyRecord, error)
	Save(ctx context.Context, rec IdempotencyRecord) error
}

// MemoryIdempotencyStore adalah implementasi in-memory thread-safe untuk unit testing dan caching cepat.
type MemoryIdempotencyStore struct {
	mu      sync.RWMutex
	records map[string]IdempotencyRecord
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		records: make(map[string]IdempotencyRecord),
	}
}

func (s *MemoryIdempotencyStore) Get(_ context.Context, key string) (IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[key]
	if !ok {
		return IdempotencyRecord{}, ErrIdempotencyNotFound
	}
	if time.Now().After(rec.ExpiresAt) {
		return IdempotencyRecord{}, ErrIdempotencyNotFound
	}
	return rec, nil
}

func (s *MemoryIdempotencyStore) Save(_ context.Context, rec IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.Key] = rec
	return nil
}

// PostgresIdempotencyStore adalah implementasi persistent berbasis PostgreSQL (BE-G09).
type PostgresIdempotencyStore struct {
	pool *pgxpool.Pool
}

func NewPostgresIdempotencyStore(pool *pgxpool.Pool) *PostgresIdempotencyStore {
	return &PostgresIdempotencyStore{pool: pool}
}

func (s *PostgresIdempotencyStore) Get(ctx context.Context, key string) (IdempotencyRecord, error) {
	if s.pool == nil {
		return IdempotencyRecord{}, ErrIdempotencyNotFound
	}
	var rec IdempotencyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT key, request_hash, response_code, response_body, created_at, expires_at
		FROM idempotency_keys
		WHERE key = $1 AND expires_at > NOW()`, key,
	).Scan(&rec.Key, &rec.RequestHash, &rec.ResponseCode, &rec.ResponseBody, &rec.CreatedAt, &rec.ExpiresAt)
	if err != nil {
		return IdempotencyRecord{}, ErrIdempotencyNotFound
	}
	return rec, nil
}

func (s *PostgresIdempotencyStore) Save(ctx context.Context, rec IdempotencyRecord) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys
			(key, request_hash, response_code, response_body, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO UPDATE
			SET response_code = EXCLUDED.response_code,
			    response_body = EXCLUDED.response_body,
			    expires_at = EXCLUDED.expires_at`,
		rec.Key, rec.RequestHash, rec.ResponseCode, rec.ResponseBody, rec.CreatedAt, rec.ExpiresAt,
	)
	return err
}
