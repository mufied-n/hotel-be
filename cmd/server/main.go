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
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/housekeeping"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/stay"
	"github.com/example/hotel-booking/internal/workers"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := platform.NewLogger()
	cfg := platform.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Error("config invalid", "err", err)
		os.Exit(1)
	}

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
	catalogStore := catalog.NewPostgresStore(pool)

	// Ensure 365-day rolling inventory horizon (BE-G18)
	if err := invStore.EnsureHorizon(ctx, 365); err != nil {
		log.Error("inventory.ensure_horizon.warning", "err", err)
	}

	// Base rate per tipe kamar (7 varian Pulang ke Uttara — BE-G01).
	baseRates := map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000,   // Superior King
		"01900000-0000-7000-8000-000000000002": 550_000,   // Superior Twin
		"01900000-0000-7000-8000-000000000003": 750_000,   // Deluxe King
		"01900000-0000-7000-8000-000000000004": 750_000,   // Deluxe Twin
		"01900000-0000-7000-8000-000000000005": 1_100_000, // Executive King
		"01900000-0000-7000-8000-000000000006": 1_650_000, // Junior Suite
		"01900000-0000-7000-8000-000000000007": 3_500_000, // Presidential Suite
	}
	rateEngine := rates.NewEngine(baseRates, 1.25) // weekend +25%

	var payGateway booking.PaymentGateway
	var xenditGw *payment.XenditGateway
	if cfg.XenditSecretKey != "" {
		xenditGw = payment.NewXendit(cfg.XenditBaseURL, cfg.XenditSecretKey, cfg.XenditWebhookToken, cfg.AppBaseURL, log)
		payGateway = xenditGw
		log.Info("payment.gateway.xendit.active", "base_url", cfg.XenditBaseURL)
	} else {
		payGateway = payment.NewFake()
		log.Info("payment.gateway.fake.active", "mode", "dev_fallback")
	}

	var notifierSvc booking.Notifier
	var otpNotifier guest.OTPNotifier
	if cfg.ResendAPIKey != "" {
		resendNotifier := notifier.NewResend(cfg.ResendBaseURL, cfg.ResendAPIKey, cfg.ResendFromEmail, log)
		notifierSvc = resendNotifier
		otpNotifier = resendNotifier
		log.Info("notifier.resend.active", "from", cfg.ResendFromEmail)
	} else {
		logNotifier := notifier.NewLog(log)
		notifierSvc = logNotifier
		otpNotifier = logNotifier
		log.Info("notifier.log.active", "mode", "dev_fallback")
	}

	bkSvc := booking.NewService(bkRunner, invStore, rateEngine, payGateway, notifierSvc, bkReader, cfg.HoldTimeout, log)
	bkSvc.SetPaymentAttemptStore(booking.NewPostgresPaymentAttemptStore(pool))

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
		return notifierSvc.SendBookingConfirmed(ctx, b)
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

	// ---- Guest Auth & My Bookings Service (F02 & F03) ----
	guestStore := guest.NewPostgresStore(pool)
	guestSvc := guest.NewService(guestStore, otpNotifier, log)

	// ---- Finance Reconciliation & Refund Service (F14) ----
	financeStore := finance.NewPostgresStore(pool)
	financeSvc := finance.NewService(financeStore, xenditGw, log)

	// ---- Housekeeping Room Status & Readiness Service (Proposed 01) ----
	housekeepingStore := housekeeping.NewPostgresStore(pool)
	housekeepingSvc := housekeeping.NewService(housekeepingStore, log)

	// ---- Front Desk Daily Operations Roster & Shift Handover Service (Proposed 02) ----
	frontdeskStore := frontdesk.NewPostgresStore(pool)
	frontdeskSvc := frontdesk.NewService(frontdeskStore, log)

	// ---- Stay Modification & Room Move Service (Proposed 03) ----
	stayStore := stay.NewPostgresStore(pool)
	staySvc := stay.NewService(stayStore, rateEngine, log)

	// ---- Feature Flags Engine ----
	ffManager, err := featureflag.NewPostgresManager(ctx, pool, redisClient, 30*time.Second, log)
	if err != nil {
		log.Warn("featureflag.init_fallback", "err", err)
	}
	defer func() { _ = ffManager.Close() }()

	// ---- HTTP ----
	handler := api.NewRouter(api.Deps{
		BookingSvc:       bkSvc,
		InvStore:         invStore,
		RateSvc:          rateEngine,
		RateEngine:       rateEngine,
		QuoteStore:       rateEngine.QuoteStore(),
		CatalogStore:     catalogStore,
		Enqueuer:         enqueuer,
		Enforcer:         enforcer,
		IdempotencyStore: api.NewPostgresIdempotencyStore(pool),
		IsDevelopment:    cfg.IsDevelopment(),
		RateLimiter:      api.NewRateLimiter(20, 40), // 20 req/s, burst 40
		XenditGateway:    xenditGw,
		GuestSvc:         guestSvc,
		FinanceSvc:       financeSvc,
		HousekeepingSvc:  housekeepingSvc,
		FrontDeskSvc:     frontdeskSvc,
		StaySvc:          staySvc,
		FeatureFlag:      ffManager,
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
				if errors.Is(err, booking.ErrHoldExpired) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"error":"hold has expired, room availability was released","code":"HOLD_EXPIRED"}`))
					return
				}
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
			stop() // Fail-fast: batalkan context jika HTTP server gagal listen
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	asynqSrv.Shutdown()
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
