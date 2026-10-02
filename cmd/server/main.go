// Command server menjalankan seluruh aplikasi dalam satu binary:
// HTTP API, migrasi, asynq worker, dan outbox relay (modular monolith — §4.3).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/hotel-booking/internal/adapter/notifier"
	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := platform.NewLogger()
	cfg := platform.LoadConfig()

	// ---- Infrastruktur ----
	pool, err := platform.NewDB(ctx, cfg)
	if err != nil {
		log.Error("db init", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient := platform.NewValkey(cfg)
	defer func() { _ = redisClient.Close() }()

	// ---- Wiring domain & adapter (composition root — §8.2) ----
	invStore := &inventory.PostgresStore{Pool: pool}
	bkRunner := &booking.PostgresTxRunner{Pool: pool}
	bkReader := &booking.PostgresReader{Pool: pool}

	// Base rate per tipe kamar (contoh IDR, satuan minor). Di produksi:
	// taruh di tabel rates atau config admin.
	baseRates := map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000,   // Standard
		"01900000-0000-7000-8000-000000000002": 750_000,   // Superior
		"01900000-0000-7000-8000-000000000003": 1_100_000, // Deluxe
		"01900000-0000-7000-8000-000000000004": 1_450_000, // Family
		"01900000-0000-7000-8000-000000000005": 2_200_000, // Suite
	}
	rateEngine := rates.NewEngine(baseRates, 1.25) // weekend +25%

	payGateway := payment.NewFake() // ganti: midtrans.New(cfg) / stripe.New(cfg)
	notifier := notifier.NewLog(log)

	bkSvc := booking.NewService(bkRunner, invStore, rateEngine, payGateway, notifier, bkReader, cfg.HoldTimeout, log)

	// ---- asynq (job queue di Valkey — §5.4, §6) ----
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.ValkeyAddr})
	defer func() { _ = asynqClient.Close() }()
	enqueuer := &workers.Enqueuer{Client: asynqClient}

	// ---- Handler event domain (outbox → efek samping) ----
	onConfirmed := func(ctx context.Context, payload []byte) error {
		id, err := workers.ParsePayloadBookingID(payload)
		if err != nil {
			return err
		}
		b, err := bkSvc.Get(ctx, id)
		if err != nil {
			return err
		}
		return notifier.SendBookingConfirmed(ctx, b)
	}
	onRelease := func(ctx context.Context, bookingID string) error {
		return releaseHold(ctx, pool, bkSvc, bookingID)
	}

	relay := &workers.OutboxRelay{
		Pool:        pool,
		BatchSize:   50,
		MaxAttempts: 8,
		Log:         log,
		Handlers: map[string]func(ctx context.Context, payload []byte) error{
			"booking.created": func(ctx context.Context, payload []byte) error {
				// Saat ini: hanya log. PMS sync / analytics masuk di sini nanti.
				var ev map[string]any
				_ = json.Unmarshal(payload, &ev)
				log.InfoContext(ctx, "event.booking.created", "payload", ev)
				return nil
			},
			"booking.confirmed": onConfirmed,
			"booking.cancelled": func(ctx context.Context, payload []byte) error {
				id, err := workers.ParsePayloadBookingID(payload)
				if err != nil {
					return err
				}
				log.InfoContext(ctx, "event.booking.cancelled", "booking_id", id)
				return nil
			},
			"booking.expired": func(ctx context.Context, payload []byte) error {
				id, err := workers.ParsePayloadBookingID(payload)
				if err != nil {
					return err
				}
				log.InfoContext(ctx, "event.booking.expired", "booking_id", id)
				return nil
			},
			"booking.checked_in": func(ctx context.Context, payload []byte) error {
				// PMS sync / housekeeping notification masuk di sini nanti.
				var ev map[string]any
				_ = json.Unmarshal(payload, &ev)
				log.InfoContext(ctx, "event.booking.checked_in", "payload", ev)
				return nil
			},
			"booking.checked_out": func(ctx context.Context, payload []byte) error {
				id, err := workers.ParsePayloadBookingID(payload)
				if err != nil {
					return err
				}
				log.InfoContext(ctx, "event.booking.checked_out", "booking_id", id)
				return nil
			},
			"booking.no_show": func(ctx context.Context, payload []byte) error {
				id, err := workers.ParsePayloadBookingID(payload)
				if err != nil {
					return err
				}
				log.InfoContext(ctx, "event.booking.no_show", "booking_id", id)
				return nil
			},
		},
	}
	go relay.Run(ctx, cfg.OutboxInterval)

	// ---- asynq server (worker) ----
	mux := workers.NewMux(pool, onRelease,
		func(ctx context.Context, bookingID string) error {
			payload, _ := json.Marshal(map[string]string{"booking_id": bookingID})
			return onConfirmed(ctx, payload)
		}, log)
	asynqSrv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.ValkeyAddr},
		asynq.Config{Concurrency: 4},
	)
	go func() {
		if err := asynqSrv.Run(mux); err != nil {
			log.Error("asynq server", "err", err)
		}
	}()

	// ---- Sweep cadangan hold expired tiap menit (defense-in-depth) ----
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				workers.SweepExpiredHolds(ctx, pool, onRelease, log)
			}
		}
	}()

	// ---- Casbin RBAC Enforcer (fail-closed: BE-G14) ----
	enforcer, err := auth.NewEnforcer(pool, "config/rbac_model.conf")
	if err != nil {
		if cfg.IsProduction() {
			log.Error("casbin.enforcer.fatal_production", "err", err)
			os.Exit(1)
		}
		log.Error("casbin.enforcer.init_failed", "err", err)
	} else {
		log.Info("casbin.enforcer.ready")
	}

	// ---- HTTP ----
	handler := api.NewRouter(api.Deps{
		BookingSvc:    bkSvc,
		InvStore:      invStore,
		RateSvc:       rateEngine,
		Enqueuer:      enqueuer,
		Enforcer:      enforcer,
		IsDevelopment: cfg.IsDevelopment(),
		RateLimiter:   api.NewRateLimiter(20, 40), // 20 req/s, burst 40
		ReadyCheck: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				return fmt.Errorf("postgres ping: %w", err)
			}
			if err := redisClient.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("valkey ping: %w", err)
			}
			return nil
		},
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			// Dev-only: langsung panggil use case Confirm yang sama dengan webhook (BE-G10).
			bookingID := r.URL.Query().Get("booking_id")
			if bookingID == "" {
				bookingID = chi.URLParam(r, "ref")
			}
			if bookingID == "" {
				http.Error(w, "booking_id or ref required", http.StatusBadRequest)
				return
			}
			if err := bkSvc.Confirm(r.Context(), bookingID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"confirmed"}`))
		},
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info("http.listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// releaseHold: expiring pending booking → kembalikan inventory + status expired.
// Idempotent: booking yang sudah bukan pending dilewati.
func releaseHold(ctx context.Context, pool *pgxpool.Pool, bkSvc *booking.Service, bookingID string) error {
	runner := &booking.PostgresTxRunner{Pool: pool}
	return runner.InTx(ctx, func(tx booking.InventoryTx, events booking.EventPublisher) error {
		b, err := tx.GetForUpdate(ctx, bookingID)
		if errors.Is(err, booking.ErrNotFound) {
			return nil // sudah tidak ada — idempotent
		}
		if err != nil {
			return err
		}
		if b.Status != booking.StatusPending {
			return nil // sudah confirmed/cancelled — idempotent
		}
		if err := tx.UpdateStatus(ctx, bookingID, booking.StatusExpired); err != nil {
			return err
		}
		if err := tx.Increment(ctx, b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"event": "booking.expired", "booking_id": bookingID})
		return events.PublishTx(ctx, "booking.expired", payload)
	})
}
