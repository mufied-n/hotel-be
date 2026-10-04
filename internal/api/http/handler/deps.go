package handler

import (
	"context"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/assistance"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/housekeeping"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/stay"
	"github.com/example/hotel-booking/internal/workers"
)

// Deps adalah dependensi transport layer — seluruh dependensi domain/platform.
type Deps struct {
	BookingSvc          *booking.Service
	InvStore            inventory.AvailabilityStore
	RateSvc             rates.RateProvider
	RateEngine          *rates.Engine
	QuoteStore          rates.QuoteStore
	CatalogStore        catalog.Store
	Enqueuer            *workers.Enqueuer
	IdempotencyStore    middleware.IdempotencyStore
	StaffAuth           middleware.StaffAuthService
	ReadyCheck          func(ctx context.Context) error
	FakePay             gin.HandlerFunc
	Enforcer            *casbin.SyncedEnforcer
	IsDevelopment       bool
	RateLimiter         *middleware.RateLimiter
	XenditGateway       *payment.XenditGateway
	GuestSvc            guest.Service
	FinanceSvc          finance.Service
	HousekeepingSvc     housekeeping.Service
	FrontDeskSvc        frontdesk.Service
	StaySvc             stay.Service
	AssistanceSvc       assistance.Service
	FeatureFlag         featureflag.Manager
	CalendarStore       rates.RateCalendarStore
	PromoStore          rates.PromoStore
	NotifierMode        string
	AuthRateLimiter     *middleware.RateLimiter
	TrustedProxies      []string
	CORSOrigins         []string
	RequestTimeout      time.Duration
	SkipRateLimitRoutes []string
}

type latePaymentAdapter struct {
	svc finance.Service
}

func (a latePaymentAdapter) CreateLatePaymentCase(ctx context.Context, bookingID, providerRef string, amountMinor int64, notes string) error {
	_, err := a.svc.CreateLatePaymentCase(ctx, bookingID, providerRef, amountMinor, notes)
	return err
}

// WithDefaults mengisi nilai default yang aman untuk dependensi transport (FR-01).
func (d Deps) WithDefaults() Deps {
	if d.CatalogStore == nil {
		d.CatalogStore = catalog.NewMemoryStore(catalog.DefaultVariants())
	}
	if d.RateSvc == nil && d.RateEngine != nil {
		d.RateSvc = d.RateEngine
	}
	if d.QuoteStore == nil && d.RateEngine != nil {
		d.QuoteStore = d.RateEngine.QuoteStore()
	}
	if d.BookingSvc != nil && d.FinanceSvc != nil {
		d.BookingSvc.SetLatePaymentRecorder(latePaymentAdapter{svc: d.FinanceSvc})
	}
	if d.RequestTimeout <= 0 {
		d.RequestTimeout = 30 * time.Second
	}
	return d
}
