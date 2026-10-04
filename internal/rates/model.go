package rates

import (
	"context"
	"errors"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
)

// Standar kode rate plan hotel
const (
	RatePlanRoomOnly     = "room_only"
	RatePlanBedBreakfast = "bed_and_breakfast"

	// Kebijakan Pembatalan Baku
	PolicyFlexible48h   = "flexible_48h"
	PolicyNonRefundable = "non_refundable"

	// Biaya sarapan resmi Pulang ke Uttara (BE-G04, BE-R10)
	BreakfastRatePerPersonPerNight = 100_000 // Dewasa (>=12 thn)
	BreakfastRateChildPerNight     = 50_000  // Anak (6-11 thn: diskon 50%)

	// Konstanta status ketersediaan (Availability Reasons)
	ReasonAvailable               = "AVAILABLE"
	ReasonStopSell                = "STOP_SELL"
	ReasonClosedToArrival         = "CLOSED_TO_ARRIVAL"
	ReasonClosedToDeparture       = "CLOSED_TO_DEPARTURE"
	ReasonMinLengthOfStayNotMet   = "MIN_LENGTH_OF_STAY_NOT_MET"
	ReasonMinLengthOfStayViolated = "MIN_LENGTH_OF_STAY_VIOLATED"
	ReasonMaxLengthOfStayExceeded = "MAX_LENGTH_OF_STAY_EXCEEDED"
	ReasonMaxLengthOfStayViolated = "MAX_LENGTH_OF_STAY_VIOLATED"
	ReasonRateUnavailable         = "RATE_UNAVAILABLE"
)

// Domain Errors Baku Modul Rates
var (
	// Kesalahan konfigurasi tarif & kuotasi dasar
	ErrUnknownRoomType  = errors.New("rates: unknown room type")
	ErrUnpricedRoomType = errors.New("rates: room type has no valid base price configured")
	ErrQuoteNotFound    = errors.New("rates: quote not found")
	ErrQuoteExpired     = errors.New("rates: quote has expired (>15m)")
	ErrInvalidRatePlan  = errors.New("rates: invalid rate plan code")
	ErrInvalidPromoCode = errors.New("rates: invalid or expired promo code")
	ErrSaveQuoteFailed  = errors.New("rates: failed to save quote")

	// Restriksi kalender tarif (Rate Calendar Restrictions)
	ErrStopSellApplied   = errors.New("rates: room type is closed for sale due to stop-sell restriction")
	ErrClosedToArrival   = errors.New("rates: check-in not allowed on this date (closed to arrival)")
	ErrClosedToDeparture = errors.New("rates: check-out not allowed on this date (closed to departure)")
	ErrMinLengthOfStay   = errors.New("rates: minimum length of stay requirement not met")
	ErrMaxLengthOfStay   = errors.New("rates: stay duration exceeds maximum length of stay")

	// Kampanye promo dinamis (Dynamic Promo Engine)
	ErrPromoNotFound         = errors.New("rates: promo campaign not found")
	ErrPromoExpired          = errors.New("rates: promo code is not active or expired")
	ErrPromoQuotaExhausted   = errors.New("rates: promo code quota has been exhausted")
	ErrPromoMinStayNotMet    = errors.New("rates: minimum stay nights requirement for promo not met")
	ErrPromoRoomNotApplicable = errors.New("rates: promo is not applicable to the selected room type")
)

// Money merepresentasikan besaran moneter baku tanpa floating-point (BE-G05, BE-G19).
type Money struct {
	Amount   int64  `json:"amount"`   // Satuan terkecil (1 Rupiah = 1)
	Currency string `json:"currency"` // Standar ISO 4217 ("IDR")
	Exponent int    `json:"exponent"` // 0 untuk IDR
}

// Quote adalah harga untuk satu malam kamar dasar (kompatibilitas historis).
type Quote struct {
	Date      time.Time `json:"date"`
	RateMinor int64     `json:"rate_minor"` // Satuan minor (IDR: Rp 1)
}

// PricingBreakdown merinci komponen harga secara transparan dan akurat (BE-G05).
type PricingBreakdown struct {
	RoomSubtotalMinor    int64  `json:"room_subtotal_minor"`
	BreakfastChargeMinor int64  `json:"breakfast_charge_minor"`
	DiscountMinor        int64  `json:"discount_minor"`
	TaxMinor             int64  `json:"tax_minor"`
	TotalPriceMinor      int64  `json:"total_price_minor"`
	Currency             string `json:"currency"`
}

// LockedQuote adalah penawaran harga terkunci dengan TTL 15 menit dari search ke checkout (BE-G06).
type LockedQuote struct {
	ID               string           `json:"quote_id"`
	CreatedAt        time.Time        `json:"created_at"`
	ExpiresAt        time.Time        `json:"expires_at"`
	RoomTypeID       string           `json:"room_type_id"`
	RatePlanCode     string           `json:"rate_plan_code"`
	RatePlanName     string           `json:"rate_plan_name"`
	CancellationCode string           `json:"cancellation_policy"`
	CancellationDesc string           `json:"cancellation_description"`
	CheckIn          time.Time        `json:"check_in"`
	CheckOut         time.Time        `json:"check_out"`
	NumRooms         int              `json:"num_rooms"`
	NumGuests        int              `json:"num_guests"`
	Adults           int              `json:"adults,omitempty"`
	Children         int              `json:"children,omitempty"`
	ChildAges        []int            `json:"child_ages,omitempty"`
	NightlyRates     []Quote          `json:"nightly_rates"`
	Pricing          PricingBreakdown `json:"pricing"`
}

// QuoteRequest adalah parameter input untuk mengunci penawaran harga.
type QuoteRequest struct {
	RoomTypeID   string    `json:"room_type_id"`
	RatePlanCode string    `json:"rate_plan_code"`
	CheckIn      time.Time `json:"check_in"`
	CheckOut     time.Time `json:"check_out"`
	NumRooms     int       `json:"num_rooms"`
	NumGuests    int       `json:"num_guests"`
	Adults       int       `json:"adults,omitempty"`
	Children     int       `json:"children,omitempty"`
	ChildAges    []int     `json:"child_ages,omitempty"`
	PromoCode    string    `json:"promo_code"`
}

// CalendarOverride mewakili tarif harian dinamis dan restriksi kamar.
type CalendarOverride struct {
	ID               string    `json:"id,omitempty"`
	RoomTypeID       string    `json:"room_type_id"`
	RoomTypeName     string    `json:"room_type_name,omitempty"`
	RatePlanCode     string    `json:"rate_plan_code"`
	Date             time.Time `json:"date"`
	BasePriceIDR     int64     `json:"base_price_idr,omitempty"`
	PriceOverrideIDR *int64    `json:"price_override_idr"`
	EffectivePrice   int64     `json:"effective_price_idr"`
	IsStopSell       bool      `json:"is_stop_sell"`
	IsCTA            bool      `json:"is_cta"`
	IsCTD            bool      `json:"is_ctd"`
	MinLOS           int       `json:"min_los"`
	MaxLOS           int       `json:"max_los"`
	AllotmentLimit   *int      `json:"allotment_limit,omitempty"`
}

// BulkCalendarUpdateRequest adalah parameter input Revenue Manager untuk mengubah banyak tanggal sekaligus.
type BulkCalendarUpdateRequest struct {
	RoomTypeIDs      []string `json:"room_type_ids"`
	StartDate        string   `json:"start_date"` // YYYY-MM-DD
	EndDate          string   `json:"end_date"`   // YYYY-MM-DD
	RatePlanCode     string   `json:"rate_plan_code"`
	PriceOverrideIDR *int64   `json:"price_override_idr"`
	IsStopSell       *bool    `json:"is_stop_sell"`
	IsCTA            *bool    `json:"is_cta"`
	IsCTD            *bool    `json:"is_ctd"`
	MinLOS           *int     `json:"min_los"`
	MaxLOS           *int     `json:"max_los"`
	AllotmentLimit   *int     `json:"allotment_limit"`
}

// PromoCampaign mewakili kampanye kode promo dinamis.
type PromoCampaign struct {
	ID                  string    `json:"id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	DiscountType        string    `json:"discount_type"` // "PERCENT" atau "FIXED"
	DiscountValue       int       `json:"discount_value"`
	MaxDiscountIDR      *int64    `json:"max_discount_idr"`
	MinStayNights       int       `json:"min_stay_nights"`
	QuotaTotal          int       `json:"quota_total"`
	QuotaUsed           int       `json:"quota_used"`
	ValidFrom           time.Time `json:"valid_from"`
	ValidTo             time.Time `json:"valid_to"`
	ApplicableRoomTypes []string  `json:"applicable_room_types,omitempty"`
	IsActive            bool      `json:"is_active"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

// BaseRateSource adalah port opsional untuk mengambil tarif dasar kamar secara dinamis dari katalog (BE-R09).
type BaseRateSource interface {
	GetVariant(ctx context.Context, idOrCode string) (catalog.RoomVariant, error)
}

// RateProvider adalah port yang dikonsumsi modul booking untuk menghitung harga.
type RateProvider interface {
	Quote(ctx context.Context, roomTypeID string, from, to time.Time) ([]Quote, error)
}

// QuoteStore adalah port penyimpanan quote in-memory / cache dengan validasi TTL (BE-G06).
type QuoteStore interface {
	SaveQuote(ctx context.Context, q LockedQuote) error
	GetQuote(ctx context.Context, id string) (LockedQuote, error)
}

// RateCalendarStore mendefinisikan kontrak persistensi untuk kalender tarif.
type RateCalendarStore interface {
	GetCalendar(ctx context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]CalendarOverride, error)
	BulkUpsertOverrides(ctx context.Context, req BulkCalendarUpdateRequest) (int64, error)
}

// PromoStore mendefinisikan kontrak persistensi untuk kampanye promo.
type PromoStore interface {
	GetByCode(ctx context.Context, code string) (PromoCampaign, error)
	ListCampaigns(ctx context.Context) ([]PromoCampaign, error)
	CreateCampaign(ctx context.Context, promo PromoCampaign) (string, error)
	UpdateCampaign(ctx context.Context, id string, isActive bool, quotaTotal int) error
	ReserveQuotaAtomic(ctx context.Context, code string) error
	ReleaseQuotaAtomic(ctx context.Context, code string) error
}
