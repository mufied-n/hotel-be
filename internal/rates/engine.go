// Package rates adalah rate engine hotel Pulang ke Uttara (desain §3.2, BE-G04, BE-G05, BE-G06).
// Mengelola harga dinamis per malam, paket rate plan (Room Only vs Bed & Breakfast),
// kalkulasi promo code, breakdown pajak 10% PB1, tipe data Money, dan 15-minute Quote Lock Engine.
package rates

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/example/hotel-booking/internal/catalog"
)

// Standar kode rate plan hotel
const (
	RatePlanRoomOnly     = "room_only"
	RatePlanBedBreakfast = "bed_and_breakfast"

	// Kebijakan Pembatalan Baku
	PolicyFlexible48h   = "flexible_48h"
	PolicyNonRefundable = "non_refundable"

	// Biaya sarapan resmi Pulang ke Uttara (Rp 100.000 / orang dewasa / malam)
	BreakfastRatePerPersonPerNight = 100_000
)

var (
	ErrUnknownRoomType  = errors.New("rates: unknown room type")
	ErrUnpricedRoomType = errors.New("rates: room type has no valid base price configured")
	ErrQuoteNotFound    = errors.New("rates: quote not found")
	ErrQuoteExpired     = errors.New("rates: quote has expired (>15m)")
	ErrInvalidRatePlan  = errors.New("rates: invalid rate plan code")
	ErrInvalidPromoCode = errors.New("rates: invalid or expired promo code")
)

// BaseRateSource adalah port opsional untuk mengambil tarif dasar kamar secara dinamis dari katalog (BE-R09).
type BaseRateSource interface {
	GetVariant(ctx context.Context, idOrCode string) (catalog.RoomVariant, error)
}

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
	PromoCode    string    `json:"promo_code"`
}

// QuoteStore adalah port penyimpanan quote in-memory / cache dengan validasi TTL (BE-G06).
type QuoteStore interface {
	SaveQuote(ctx context.Context, q LockedQuote) error
	GetQuote(ctx context.Context, id string) (LockedQuote, error)
}

// MemoryQuoteStore implementasi thread-safe in-memory quote repository dengan masa berlaku 15 menit.
type MemoryQuoteStore struct {
	mu     sync.RWMutex
	quotes map[string]LockedQuote
	ttl    time.Duration
}

func NewMemoryQuoteStore(ttl time.Duration) *MemoryQuoteStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &MemoryQuoteStore{
		quotes: make(map[string]LockedQuote),
		ttl:    ttl,
	}
}

func (s *MemoryQuoteStore) SaveQuote(_ context.Context, q LockedQuote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotes[q.ID] = q
	return nil
}

func (s *MemoryQuoteStore) GetQuote(_ context.Context, id string) (LockedQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.quotes[id]
	if !ok {
		return LockedQuote{}, ErrQuoteNotFound
	}
	if time.Now().After(q.ExpiresAt) {
		return LockedQuote{}, ErrQuoteExpired
	}
	return q, nil
}

// RateProvider adalah port yang dikonsumsi modul booking untuk menghitung harga.
type RateProvider interface {
	Quote(ctx context.Context, roomTypeID string, from, to time.Time) ([]Quote, error)
}

// Engine implementasi tarif kamar dan generator quote terkunci.
type Engine struct {
	mu            sync.RWMutex
	base          map[string]int64
	baseSource    BaseRateSource
	weekendFactor float64
	quoteStore    QuoteStore
}

func NewEngine(base map[string]int64, weekendFactor float64) *Engine {
	baseCopy := make(map[string]int64, len(base))
	for k, v := range base {
		baseCopy[k] = v
	}
	return &Engine{
		base:          baseCopy,
		weekendFactor: weekendFactor,
		quoteStore:    NewMemoryQuoteStore(15 * time.Minute),
	}
}

func NewEngineWithQuoteStore(base map[string]int64, weekendFactor float64, store QuoteStore) *Engine {
	if store == nil {
		store = NewMemoryQuoteStore(15 * time.Minute)
	}
	baseCopy := make(map[string]int64, len(base))
	for k, v := range base {
		baseCopy[k] = v
	}
	return &Engine{
		base:          baseCopy,
		weekendFactor: weekendFactor,
		quoteStore:    store,
	}
}

// SetBaseRateSource menghubungkan sumber tarif dinamis dari katalog ke rate engine (BE-R09).
func (e *Engine) SetBaseRateSource(src BaseRateSource) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.baseSource = src
}

// SetBaseRate mendaftarkan atau memperbarui tarif dasar kamar pada in-memory fallback map (BE-R09).
func (e *Engine) SetBaseRate(roomTypeID string, rateMinor int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.base == nil {
		e.base = make(map[string]int64)
	}
	e.base[roomTypeID] = rateMinor
}

// QuoteStore mengembalikan instance quote store yang aktif pada engine.
func (e *Engine) QuoteStore() QuoteStore {
	return e.quoteStore
}

// Quote menghitung harga per malam untuk rentang half-open [from, to).
func (e *Engine) Quote(ctx context.Context, roomTypeID string, from, to time.Time) ([]Quote, error) {
	var base int64
	var found bool

	e.mu.RLock()
	src := e.baseSource
	staticBase, staticOk := e.base[roomTypeID]
	e.mu.RUnlock()

	if src != nil {
		v, err := src.GetVariant(ctx, roomTypeID)
		if err == nil {
			if v.BasePriceMinor <= 0 {
				return nil, ErrUnpricedRoomType
			}
			base = v.BasePriceMinor
			found = true
		} else if !errors.Is(err, catalog.ErrVariantNotFound) {
			return nil, err
		}
	}

	if !found {
		if staticOk {
			if staticBase <= 0 {
				return nil, ErrUnpricedRoomType
			}
			base = staticBase
			found = true
		}
	}

	if !found {
		return nil, ErrUnknownRoomType
	}

	var out []Quote
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		rate := base
		switch d.Weekday() {
		case time.Friday, time.Saturday:
			rate = int64(float64(base) * e.weekendFactor)
		}
		out = append(out, Quote{Date: d, RateMinor: rate})
	}
	return out, nil
}

// CalculateLockedQuote membuat penawaran harga terkunci lengkap dengan breakdown dan TTL 15 menit (BE-G04, BE-G05, BE-G06).
func (e *Engine) CalculateLockedQuote(ctx context.Context, req QuoteRequest) (LockedQuote, error) {
	// Normalisasi & validasi rate plan
	planCode := strings.ToLower(strings.TrimSpace(req.RatePlanCode))
	if planCode == "" {
		planCode = RatePlanRoomOnly
	}
	var planName string
	switch planCode {
	case RatePlanRoomOnly:
		planName = "Room Only"
	case RatePlanBedBreakfast:
		planName = "Bed and Breakfast"
	default:
		return LockedQuote{}, ErrInvalidRatePlan
	}

	numRooms := req.NumRooms
	if numRooms < 1 {
		numRooms = 1
	}
	numGuests := req.NumGuests
	if numGuests < 1 {
		numGuests = 2
	}

	// Ambil tarif per malam kamar dasar
	nightly, err := e.Quote(ctx, req.RoomTypeID, req.CheckIn, req.CheckOut)
	if err != nil {
		return LockedQuote{}, err
	}
	numNights := len(nightly)
	if numNights == 0 {
		return LockedQuote{}, errors.New("rates: check-out must be after check-in")
	}

	var sumNightlyBase int64
	for _, n := range nightly {
		sumNightlyBase += n.RateMinor
	}
	roomSubtotal := sumNightlyBase * int64(numRooms)

	// Biaya sarapan jika paket bed_and_breakfast
	var breakfastCharge int64
	if planCode == RatePlanBedBreakfast {
		breakfastCharge = int64(BreakfastRatePerPersonPerNight) * int64(numGuests) * int64(numNights) * int64(numRooms)
	}

	// Kebijakan pembatalan default
	cancelPolicy := PolicyFlexible48h
	cancelDesc := "Pembatalan gratis hingga 48 jam sebelum jam 14:00 WIB pada tanggal check-in. Pembatalan setelah batas waktu dikenakan biaya 100%."

	// Evaluasi promo code
	var discount int64
	promo := strings.ToUpper(strings.TrimSpace(req.PromoCode))
	if promo != "" {
		if promo == "OCTOBREAK" {
			// Diskon 15% dari subtotal kamar, kebijakan menjadi non-refundable
			discount = (roomSubtotal * 15) / 100
			cancelPolicy = PolicyNonRefundable
			cancelDesc = "Tarif promo hemat OCTOBREAK tidak dapat dibatalkan atau di-refund (100% biaya pembatalan)."
		} else {
			return LockedQuote{}, ErrInvalidPromoCode
		}
	}

	// Pajak PB1 (10% dari nilai kena pajak)
	taxable := roomSubtotal + breakfastCharge - discount
	if taxable < 0 {
		taxable = 0
	}
	tax := (taxable * 10) / 100
	total := taxable + tax

	now := time.Now()
	lq := LockedQuote{
		ID:               uuid.NewV7().String(),
		CreatedAt:        now,
		ExpiresAt:        now.Add(15 * time.Minute),
		RoomTypeID:       req.RoomTypeID,
		RatePlanCode:     planCode,
		RatePlanName:     planName,
		CancellationCode: cancelPolicy,
		CancellationDesc: cancelDesc,
		CheckIn:          req.CheckIn,
		CheckOut:         req.CheckOut,
		NumRooms:         numRooms,
		NumGuests:        numGuests,
		NightlyRates:     nightly,
		Pricing: PricingBreakdown{
			RoomSubtotalMinor:    roomSubtotal,
			BreakfastChargeMinor: breakfastCharge,
			DiscountMinor:        discount,
			TaxMinor:             tax,
			TotalPriceMinor:      total,
			Currency:             "IDR",
		},
	}

	if e.quoteStore != nil {
		_ = e.quoteStore.SaveQuote(ctx, lq)
	}

	return lq, nil
}
