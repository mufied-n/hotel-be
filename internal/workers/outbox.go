package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRelay memproses tabel outbox (desain §5.3): polling FOR UPDATE SKIP
// LOCKED (aman multi-worker), dispatch per topic, retry backoff eksponensial,
// dead-letter ke status 'failed' setelah maxAttempts.
type OutboxRelay struct {
	Pool        *pgxpool.Pool
	BatchSize   int
	MaxAttempts int
	Log         *slog.Logger
	// Handlers memetakan topic outbox → prosesor event.
	// Return error → event di-retry dengan backoff.
	Handlers map[string]func(ctx context.Context, payload []byte) error
}

// Run menjalankan loop polling sampai ctx dibatalkan.
func (r *OutboxRelay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

func (r *OutboxRelay) processBatch(ctx context.Context) {
	for i := 0; i < (r.BatchSize); i++ {
		ok := r.processOne(ctx)
		if !ok {
			return // tidak ada lagi job pending
		}
	}
}

func (r *OutboxRelay) processOne(ctx context.Context) bool {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		r.Log.ErrorContext(ctx, "outbox.begin", "err", err)
		return false
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		id       int64
		topic    string
		payload  []byte
		attempts int
	)
	// SKIP LOCKED: dua relay tidak akan menarik job yang sama (desain §5.3).
	err = tx.QueryRow(ctx, `
		SELECT id, topic, payload, attempts
		FROM outbox
		WHERE status = 'pending' AND next_retry_at <= now()
		ORDER BY id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(&id, &topic, &payload, &attempts)
	if err != nil {
		_ = tx.Rollback(ctx) // ErrNoRows = antrian kosong (normal)
		return false
	}

	handler, ok := r.Handlers[topic]
	if !ok {
		r.Log.ErrorContext(ctx, "outbox.unknown_topic", "topic", topic, "id", id)
		_, _ = tx.Exec(ctx, `UPDATE outbox SET status='failed' WHERE id=$1`, id)
		_ = tx.Commit(ctx)
		return true
	}

	if err := handler(ctx, payload); err != nil {
		attempts++
		if attempts >= r.MaxAttempts {
			_, _ = tx.Exec(ctx,
				`UPDATE outbox SET status='failed', attempts=$2 WHERE id=$1`, id, attempts)
			r.Log.ErrorContext(ctx, "outbox.dead_letter", "topic", topic, "id", id, "attempts", attempts, "err", err)
		} else {
			delay := backoff(attempts)
			_, _ = tx.Exec(ctx,
				`UPDATE outbox SET attempts=$2, next_retry_at=now()+$3 WHERE id=$1`,
				id, attempts, delay)
			r.Log.WarnContext(ctx, "outbox.retry_scheduled", "topic", topic, "id", id, "attempt", attempts, "delay", delay.String())
		}
		_ = tx.Commit(ctx)
		return true
	}

	if _, err := tx.Exec(ctx, `UPDATE outbox SET status='done' WHERE id=$1`, id); err != nil {
		r.Log.ErrorContext(ctx, "outbox.mark_done", "err", err)
		return false
	}
	if err := tx.Commit(ctx); err != nil {
		r.Log.ErrorContext(ctx, "outbox.commit", "err", err)
		return false
	}
	r.Log.InfoContext(ctx, "outbox.processed", "topic", topic, "id", id)
	return true
}

// backoff eksponensial: 5s, 10s, 20s, 40s ... cap 5 menit.
func backoff(attempt int) time.Duration {
	d := time.Duration(5*math.Pow(2, float64(attempt-1))) * time.Second
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

// ParsePayloadBookingID helper umum untuk handler event yang membawa booking_id.
func ParsePayloadBookingID(payload []byte) (string, error) {
	var p struct {
		BookingID string `json:"booking_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("outbox payload: %w", err)
	}
	if p.BookingID == "" {
		return "", fmt.Errorf("outbox payload: booking_id kosong")
	}
	return p.BookingID, nil
}
