package featureflag

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	ValkeyChannelUpdated = "pku:feature_flags:updated"
)

// PostgresManager mengimplementasikan Manager dengan persistensi PostgreSQL dan event sinkronisasi Valkey.
type PostgresManager struct {
	pool     *pgxpool.Pool
	valkey   *redis.Client
	snapshot atomic.Pointer[Snapshot]
	cancel   context.CancelFunc
	log      *slog.Logger
}

// NewPostgresManager membuat dan menginisialisasi PostgresManager secara synchronous pada startup.
func NewPostgresManager(ctx context.Context, pool *pgxpool.Pool, valkey *redis.Client, syncInterval time.Duration, log *slog.Logger) (*PostgresManager, error) {
	if log == nil {
		log = slog.Default()
	}

	subCtx, cancel := context.WithCancel(context.Background())
	pm := &PostgresManager{
		pool:   pool,
		valkey: valkey,
		cancel: cancel,
		log:    log,
	}

	// 1. Initial synchronous load
	if err := pm.Reload(ctx); err != nil {
		log.Warn("featureflag: initial db load failed, fallback to defaults", "err", err)
		defaults := DefaultFlags()
		snap := Snapshot(defaults)
		pm.snapshot.Store(&snap)
	}

	// 2. Jalankan background sync polling jika syncInterval > 0
	if syncInterval > 0 {
		go pm.startPolling(subCtx, syncInterval)
	}

	// 3. Jalankan listener Valkey Pub/Sub jika client tersedia
	if valkey != nil {
		go pm.startSubscriber(subCtx)
	}

	return pm, nil
}

func (pm *PostgresManager) startPolling(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pm.Reload(ctx); err != nil && !errors.Is(err, context.Canceled) {
				pm.log.Warn("featureflag.poll_reload_failed", "err", err)
			}
		}
	}
}

func (pm *PostgresManager) startSubscriber(ctx context.Context) {
	pubsub := pm.valkey.Subscribe(ctx, ValkeyChannelUpdated)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			pm.log.Debug("featureflag.event_received", "channel", msg.Channel, "payload", msg.Payload)
			_ = pm.Reload(ctx)
		}
	}
}

// IsEnabled mengevaluasi status feature flag dari memory snapshot.
func (pm *PostgresManager) IsEnabled(ctx context.Context, key string) bool {
	snapPtr := pm.snapshot.Load()
	if snapPtr == nil {
		return false
	}
	snap := *snapPtr
	f, ok := snap[key]
	if !ok {
		return false
	}

	if !f.Enabled {
		return false
	}

	if len(f.AllowedRoles) == 0 {
		return true
	}

	callerRole := RoleFromContext(ctx)
	if callerRole == "" {
		return false
	}
	for _, r := range f.AllowedRoles {
		if r == callerRole {
			return true
		}
	}
	return false
}

// Get mengambil detail konfigurasi satu flag dari snapshot.
func (pm *PostgresManager) Get(_ context.Context, key string) (Flag, bool) {
	snapPtr := pm.snapshot.Load()
	if snapPtr == nil {
		return Flag{}, false
	}
	snap := *snapPtr
	f, ok := snap[key]
	return f, ok
}

// List mengambil seluruh daftar konfigurasi flag dari snapshot.
func (pm *PostgresManager) List(_ context.Context) []Flag {
	snapPtr := pm.snapshot.Load()
	if snapPtr == nil {
		return nil
	}
	snap := *snapPtr
	res := make([]Flag, 0, len(snap))
	for _, f := range snap {
		res = append(res, f)
	}
	return res
}

// Update memperbarui konfigurasi flag di PostgreSQL, reload snapshot lokal, dan mempublikasikan event ke Valkey.
func (pm *PostgresManager) Update(ctx context.Context, key string, enabled bool, allowedRoles []string, updatedBy string) (Flag, error) {
	if allowedRoles == nil {
		allowedRoles = []string{}
	}

	query := `
		UPDATE feature_flags
		SET enabled = $1, allowed_roles = $2, updated_by = $3, updated_at = NOW()
		WHERE key = $4
		RETURNING key, name, description, enabled, allowed_roles, updated_by, updated_at
	`
	var f Flag
	err := pm.pool.QueryRow(ctx, query, enabled, allowedRoles, updatedBy, key).Scan(
		&f.Key, &f.Name, &f.Description, &f.Enabled, &f.AllowedRoles, &f.UpdatedBy, &f.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Flag{}, fmt.Errorf("%w: %s", ErrFlagNotFound, key)
	}
	if err != nil {
		return Flag{}, fmt.Errorf("failed to update feature flag %s: %w", key, err)
	}

	// Reload snapshot lokal
	_ = pm.Reload(ctx)

	// Broadcast perubahan ke instance lain melalui Valkey
	if pm.valkey != nil {
		_ = pm.valkey.Publish(ctx, ValkeyChannelUpdated, key).Err()
	}

	return f, nil
}

// Reload memuat seluruh record dari tabel feature_flags ke memory snapshot.
func (pm *PostgresManager) Reload(ctx context.Context) error {
	query := `
		SELECT key, name, description, enabled, allowed_roles, updated_by, updated_at
		FROM feature_flags
	`
	rows, err := pm.pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query feature_flags: %w", err)
	}
	defer rows.Close()

	newSnap := make(Snapshot)
	for rows.Next() {
		var f Flag
		if err := rows.Scan(&f.Key, &f.Name, &f.Description, &f.Enabled, &f.AllowedRoles, &f.UpdatedBy, &f.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan feature_flags row: %w", err)
		}
		newSnap[f.Key] = f
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("feature_flags iteration error: %w", err)
	}

	// Jika tabel kosong, fallback ke defaults
	if len(newSnap) == 0 {
		defaults := DefaultFlags()
		newSnap = Snapshot(defaults)
	}

	pm.snapshot.Store(&newSnap)
	return nil
}

// Close menghentikan background worker subscriber dan polling.
func (pm *PostgresManager) Close() error {
	if pm.cancel != nil {
		pm.cancel()
	}
	return nil
}
