// Package workers menyediakan background job asynq: release-hold kamar
// (desain §12.1 pendekatan 2+6) dan relay outbox → asynq (§5.4).
package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Nama task asynq.
const (
	TaskReleaseHold = "booking:release_hold"
	TaskNotifyEmail = "notify:booking_confirmed"
)

// ---------- Task payloads ----------

type ReleaseHoldPayload struct {
	BookingID string `json:"booking_id"`
}

type NotifyPayload struct {
	BookingID string `json:"booking_id"`
}

// ---------- Producer ----------

// Enqueuer membungkus asynq.Client agar modul lain tidak bergantung langsung
// ke asynq (port tipis — mudah diganti nanti, desain §5.4).
type Enqueuer struct{ Client *asynq.Client }

func (e *Enqueuer) EnqueueReleaseHold(ctx context.Context, bookingID string, delay time.Duration) error {
	payload, err := json.Marshal(ReleaseHoldPayload{BookingID: bookingID})
	if err != nil {
		return fmt.Errorf("workers: marshal: %w", err)
	}
	_, err = e.Client.EnqueueContext(ctx,
		asynq.NewTask(TaskReleaseHold, payload),
		asynq.ProcessIn(delay),
		asynq.MaxRetry(5),
	)
	return err
}

func (e *Enqueuer) EnqueueNotify(ctx context.Context, bookingID string) error {
	payload, err := json.Marshal(NotifyPayload{BookingID: bookingID})
	if err != nil {
		return fmt.Errorf("workers: marshal: %w", err)
	}
	_, err = e.Client.EnqueueContext(ctx, asynq.NewTask(TaskNotifyEmail, payload), asynq.MaxRetry(5))
	return err
}

// ---------- Handlers ----------

// NewMux memasang seluruh handler task.
func NewMux(pool *pgxpool.Pool, onRelease func(ctx context.Context, bookingID string) error, onNotify func(ctx context.Context, bookingID string) error, log *slog.Logger) *asynq.ServeMux {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TaskReleaseHold, func(ctx context.Context, t *asynq.Task) error {
		var p ReleaseHoldPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("workers: release_hold payload: %w", asynq.SkipRetry)
		}
		if err := onRelease(ctx, p.BookingID); err != nil {
			return fmt.Errorf("workers: release_hold %s: %w", p.BookingID, err)
		}
		log.InfoContext(ctx, "worker.release_hold.done", "booking_id", p.BookingID)
		return nil
	})

	mux.HandleFunc(TaskNotifyEmail, func(ctx context.Context, t *asynq.Task) error {
		var p NotifyPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("workers: notify payload: %w", asynq.SkipRetry)
		}
		if err := onNotify(ctx, p.BookingID); err != nil {
			return fmt.Errorf("workers: notify %s: %w", p.BookingID, err)
		}
		log.InfoContext(ctx, "worker.notify.done", "booking_id", p.BookingID)
		return nil
	})

	return mux
}

// SweepExpiredHolds jalur cadangan: merilis hold yang expired walau task
// asynq-nya hilang (defense-in-depth — AOF meminimalkan, sweep menjamin).
// Dipanggil periodik dari main via goroutine ticker.
func SweepExpiredHolds(ctx context.Context, pool *pgxpool.Pool, release func(ctx context.Context, bookingID string) error, log *slog.Logger) {
	rows, err := pool.Query(ctx, `
		SELECT b.id
		FROM bookings b
		JOIN holds h ON h.booking_id = b.id
		WHERE b.status = 'pending' AND h.expires_at <= now()`)
	if err != nil {
		log.ErrorContext(ctx, "sweep.query", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.ErrorContext(ctx, "sweep.scan", "err", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()

	for _, id := range ids {
		if err := release(ctx, id); err != nil {
			log.ErrorContext(ctx, "sweep.release", "booking_id", id, "err", err)
		} else {
			log.InfoContext(ctx, "sweep.released", "booking_id", id)
		}
	}
}
