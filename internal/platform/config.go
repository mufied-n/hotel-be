// Package platform menyediakan infrastruktur lintas-modul: config, koneksi DB,
// koneksi Valkey, dan helper logging. Tidak ada logika domain di sini.
package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Config membaca seluruh setting dari environment variables.
type Config struct {
	Port        string
	DatabaseDSN string
	ValkeyAddr  string
	// HoldTimeout durasi hold kamar sebelum dirilis otomatis (desain §12: 30 menit).
	HoldTimeout time.Duration
	// OutboxInterval interval polling relay outbox.
	OutboxInterval time.Duration
}

func LoadConfig() Config {
	return Config{
		Port:           getenv("APP_PORT", "8080"),
		DatabaseDSN:    getenv("DATABASE_URL", "postgres://postgres:dev@localhost:5432/booking?sslmode=disable"),
		ValkeyAddr:     getenv("VALKEY_ADDR", "localhost:6379"),
		HoldTimeout:    getDuration("HOLD_TIMEOUT", 30*time.Minute),
		OutboxInterval: getDuration("OUTBOX_INTERVAL", 2*time.Second),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// NewDB membuat connection pool pgx.
func NewDB(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// NewValkey membuat klien Redis/Valkey (RESP-compatible — desain §6).
func NewValkey(cfg Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.ValkeyAddr,
	})
}

// NewLogger membuat structured logger (NFR §15: slog).
func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
