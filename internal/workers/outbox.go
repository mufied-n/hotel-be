package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
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

// Run menjalankan loop polling sampai ctx dibatalkan (BE-G21: fail-safe interval).
func (r *OutboxRelay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
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
		_ = tx.Rollback(ctx)
		// ErrNoRows = antrian kosong (normal), jika error lain catat ke log
		if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, context.Canceled) {
			r.Log.ErrorContext(ctx, "outbox.query", "err", err)
		}
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
			if _, errExec := tx.Exec(ctx,
				`UPDATE outbox SET status='failed', attempts=$2 WHERE id=$1`, id, attempts); errExec != nil {
				r.Log.ErrorContext(ctx, "outbox.mark_dead_letter_failed", "err", errExec, "id", id)
			}
			r.Log.ErrorContext(ctx, "outbox.dead_letter", "topic", topic, "id", id, "attempts", attempts, "err", err)
		} else {
			delay := backoff(attempts)
			if _, errExec := tx.Exec(ctx,
				`UPDATE outbox SET attempts=$2, next_retry_at=now()+$3 WHERE id=$1`,
				id, attempts, delay); errExec != nil {
				r.Log.ErrorContext(ctx, "outbox.schedule_retry_failed", "err", errExec, "id", id)
			}
			r.Log.WarnContext(ctx, "outbox.retry_scheduled", "topic", topic, "id", id, "attempt", attempts, "delay", delay.String())
		}
		if errCommit := tx.Commit(ctx); errCommit != nil {
			r.Log.ErrorContext(ctx, "outbox.commit_error_state", "err", errCommit, "id", id)
		}
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

// OTPPayload mendefinisikan struktur payload event topik 'guest.otp_dispatch' (BE-R17).
type OTPPayload struct {
	ChallengeID string    `json:"challenge_id"`
	Email       string    `json:"email"`
	OTPCode     string    `json:"otp_code"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// OTPNotifier mendefinisikan port pengiriman kode OTP untuk worker outbox (BE-R17).
type OTPNotifier interface {
	SendGuestOTP(ctx context.Context, email, otpCode string, challengeID ...string) error
}

// NewGuestOTPDispatchHandler membuat handler outbox untuk pemrosesan topik 'guest.otp_dispatch' secara andal (BE-R17).
func NewGuestOTPDispatchHandler(pool *pgxpool.Pool, notifier OTPNotifier, log *slog.Logger) func(ctx context.Context, payload []byte) error {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, payload []byte) error {
		var p OTPPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			log.ErrorContext(ctx, "outbox.otp.unmarshal_failed", "err", err)
			return nil // Payload rusak dibatalkan agar tidak menyumbat antrian outbox
		}

		now := time.Now().UTC()
		// 1. Cek apakah OTP sudah kedaluwarsa (anti-stale delivery)
		if !p.ExpiresAt.IsZero() && now.After(p.ExpiresAt) {
			log.WarnContext(ctx, "outbox.otp.discarded_expired",
				"challenge_id", p.ChallengeID,
				"recipient", p.Email,
			)
			return nil // Tandai event selesai tanpa mengirim email kedaluwarsa
		}

		// 2. Cek apakah tantangan sudah pernah diverifikasi
		if pool != nil && p.ChallengeID != "" {
			var verifiedAt *time.Time
			err := pool.QueryRow(ctx,
				`SELECT verified_at FROM guest_auth_challenges WHERE id = $1`,
				p.ChallengeID,
			).Scan(&verifiedAt)
			if err == nil && verifiedAt != nil {
				log.InfoContext(ctx, "outbox.otp.discarded_already_verified",
					"challenge_id", p.ChallengeID,
				)
				return nil // Tamu sudah login, batalkan pengiriman ulang
			}
		}

		if notifier == nil {
			log.WarnContext(ctx, "outbox.otp.no_notifier", "challenge_id", p.ChallengeID)
			return nil
		}

		// 3. Eksekusi pengiriman via notifier dengan menyertakan challengeID untuk Idempotency-Key
		if err := notifier.SendGuestOTP(ctx, p.Email, p.OTPCode, p.ChallengeID); err != nil {
			log.ErrorContext(ctx, "outbox.otp.send_failed",
				"challenge_id", p.ChallengeID,
				"recipient", p.Email,
				"err", err,
			)
			return err // Return error agar OutboxRelay menjadwalkan retry dengan exponential backoff
		}

		log.InfoContext(ctx, "outbox.otp.dispatched",
			"challenge_id", p.ChallengeID,
			"recipient", p.Email,
		)
		return nil
	}
}

