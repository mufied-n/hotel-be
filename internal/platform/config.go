// Package platform menyediakan infrastruktur lintas-modul: config, koneksi DB,
// koneksi Valkey, dan helper logging. Tidak ada logika domain di sini.
package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Config membaca seluruh setting dari environment variables.
type Config struct {
	Environment    string // "development", "staging", "production"
	Port           string
	DatabaseDSN    string
	ValkeyAddr     string
	// HoldTimeout durasi hold kamar sebelum dirilis otomatis (desain §12: 30 menit).
	HoldTimeout time.Duration
	// OutboxInterval interval polling relay outbox.
	OutboxInterval time.Duration

	// AppBaseURL URL frontend/web untuk redirect URL
	AppBaseURL string

	// Xendit Payment Gateway
	XenditBaseURL      string
	XenditSecretKey    string
	XenditWebhookToken string

	// Resend Notifier
	ResendBaseURL   string
	ResendAPIKey    string
	ResendFromEmail string

	// TrustedProxies daftar IP/CIDR load balancer yang dipercayai (FR-08)
	TrustedProxies []string
	// CORSAllowedOrigins daftar domain asal yang diizinkan untuk CORS (FR-22)
	CORSAllowedOrigins []string
}

func (c Config) IsDevelopment() bool {
	return c.Environment != "production"
}

func (c Config) IsProduction() bool {
	return c.Environment == "production"
}

// Validate memeriksa kelengkapan dan validitas konfigurasi runtime (BE-G21, BE-R16).
func (c Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("config: APP_PORT cannot be empty")
	}
	if c.DatabaseDSN == "" {
		return fmt.Errorf("config: DATABASE_URL cannot be empty")
	}
	if c.ValkeyAddr == "" {
		return fmt.Errorf("config: VALKEY_ADDR cannot be empty")
	}
	if c.HoldTimeout <= 0 {
		return fmt.Errorf("config: HOLD_TIMEOUT must be positive duration")
	}
	if c.OutboxInterval <= 0 {
		return fmt.Errorf("config: OUTBOX_INTERVAL must be positive duration")
	}
	for _, origin := range c.CORSAllowedOrigins {
		if origin == "*" {
			return fmt.Errorf("config: wildcard '*' in CORS_ALLOWED_ORIGINS is forbidden for security")
		}
	}
	if c.IsProduction() {
		if c.XenditSecretKey == "" {
			return fmt.Errorf("config: XENDIT_SECRET_KEY is required in production (fake payment gateway is forbidden)")
		}
		if c.XenditWebhookToken == "" {
			return fmt.Errorf("config: XENDIT_WEBHOOK_TOKEN is required in production")
		}
		if c.ResendAPIKey == "" {
			return fmt.Errorf("config: RESEND_API_KEY is required in production (log notifier is forbidden)")
		}
	}
	return nil
}

func LoadConfig() Config {
	holdTimeout := getDuration("HOLD_TIMEOUT", 30*time.Minute)
	if holdTimeout <= 0 {
		holdTimeout = 30 * time.Minute
	}
	outboxInterval := getDuration("OUTBOX_INTERVAL", 2*time.Second)
	if outboxInterval <= 0 {
		outboxInterval = 2 * time.Second
	}

	var trustedProxies []string
	if tp := os.Getenv("TRUSTED_PROXIES"); tp != "" {
		for _, p := range strings.Split(tp, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				trustedProxies = append(trustedProxies, trimmed)
			}
		}
	}

	var corsOrigins []string
	if co := os.Getenv("CORS_ALLOWED_ORIGINS"); co != "" {
		for _, o := range strings.Split(co, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
	}

	return Config{
		Environment:        getenv("APP_ENV", "development"),
		Port:               getenv("APP_PORT", "8080"),
		DatabaseDSN:        getenv("DATABASE_URL", "postgres://postgres:dev@localhost:5432/booking?sslmode=disable"),
		ValkeyAddr:         getenv("VALKEY_ADDR", "localhost:6379"),
		HoldTimeout:        holdTimeout,
		OutboxInterval:     outboxInterval,
		AppBaseURL:         getenv("APP_BASE_URL", "http://localhost:3000"),
		XenditBaseURL:      getenv("XENDIT_BASE_URL", "https://api.xendit.co"),
		XenditSecretKey:    getenv("XENDIT_SECRET_KEY", ""),
		XenditWebhookToken: getenv("XENDIT_WEBHOOK_TOKEN", ""),
		ResendBaseURL:      getenv("RESEND_BASE_URL", "https://api.resend.com"),
		ResendAPIKey:       getenv("RESEND_API_KEY", ""),
		ResendFromEmail:    getenv("RESEND_FROM_EMAIL", "Pulang ke Uttara <reservations@pulangkeuttara.com>"),
		TrustedProxies:     trustedProxies,
		CORSAllowedOrigins: corsOrigins,
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
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
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
		pool.Close()
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
