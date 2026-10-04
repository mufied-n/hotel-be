// Package rates menyediakan port dan adapter untuk kuotasi harga kamar terkunci (BE-G04, BE-G06, BE-R11).
package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultQuotePrefix prefiks standar kunci Redis/Valkey untuk kuotasi harga.
	DefaultQuotePrefix = "hotel:quote:"
	// DefaultQuoteTTL masa berlaku kuotasi kamar 15 menit (BE-G06).
	DefaultQuoteTTL = 15 * time.Minute
)

// ValkeyQuoteStore mengimplementasikan QuoteStore menggunakan Redis/Valkey untuk
// ketahanan kuotasi lintas instance dan container restart (BE-R11).
type ValkeyQuoteStore struct {
	client redis.Cmdable
	ttl    time.Duration
	prefix string
}

// NewValkeyQuoteStore membuat instance adapter ValkeyQuoteStore baru.
func NewValkeyQuoteStore(client redis.Cmdable, ttl time.Duration) *ValkeyQuoteStore {
	if ttl <= 0 {
		ttl = DefaultQuoteTTL
	}
	return &ValkeyQuoteStore{
		client: client,
		ttl:    ttl,
		prefix: DefaultQuotePrefix,
	}
}

// SaveQuote menyimpan kuotasi harga terkunci ke Valkey dengan TTL native.
func (s *ValkeyQuoteStore) SaveQuote(ctx context.Context, q LockedQuote) error {
	if s == nil || s.client == nil {
		return errors.New("rates: valkey client not configured")
	}
	if q.ID == "" {
		return errors.New("rates: cannot save quote with empty id")
	}

	ttl := s.ttl
	if !q.ExpiresAt.IsZero() {
		rem := time.Until(q.ExpiresAt)
		if rem > 0 {
			ttl = rem
		}
	}

	payload, err := json.Marshal(q)
	if err != nil {
		return fmt.Errorf("rates: marshal quote: %w", err)
	}

	key := s.prefix + q.ID
	if err := s.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("rates: valkey set quote: %w", err)
	}

	return nil
}

// GetQuote mengambil dan memverifikasi kuotasi harga terkunci dari Valkey.
func (s *ValkeyQuoteStore) GetQuote(ctx context.Context, id string) (LockedQuote, error) {
	if s == nil || s.client == nil {
		return LockedQuote{}, errors.New("rates: valkey client not configured")
	}
	if id == "" {
		return LockedQuote{}, ErrQuoteNotFound
	}

	key := s.prefix + id
	val, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return LockedQuote{}, ErrQuoteNotFound
		}
		return LockedQuote{}, fmt.Errorf("rates: valkey get quote: %w", err)
	}

	var q LockedQuote
	if err := json.Unmarshal([]byte(val), &q); err != nil {
		return LockedQuote{}, fmt.Errorf("rates: unmarshal quote: %w", err)
	}

	if !q.ExpiresAt.IsZero() && time.Now().After(q.ExpiresAt) {
		return LockedQuote{}, ErrQuoteExpired
	}

	return q, nil
}
