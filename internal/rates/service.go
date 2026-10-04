// Package rates adalah rate engine hotel Pulang ke Uttara (desain §3.2, BE-G04, BE-G05, BE-G06).
// Mengelola harga dinamis per malam, paket rate plan (Room Only vs Bed & Breakfast),
// kalkulasi promo code, breakdown pajak 10% PB1, tipe data Money, dan 15-minute Quote Lock Engine.
package rates

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/example/hotel-booking/internal/catalog"
)

// Engine implementasi tarif kamar dan generator quote terkunci (Domain Service).
type Engine struct {
	mu            sync.RWMutex
	base          map[string]int64
	baseSource    BaseRateSource
	calendarStore RateCalendarStore
	promoStore    PromoStore
	weekendFactor float64
	quoteStore    QuoteStore
}

// Service adalah alias untuk Engine untuk keseragaman pola arsitektur seluruh modul.
type Service = Engine

// NewEngine membuat instance Engine baru dengan fallback quote store in-memory.
func NewEngine(base map[string]int64, weekendFactor float64) *Engine {
	return NewEngineWithQuoteStore(base, weekendFactor, nil)
}

// NewService membuat instance Service baru (identik dengan NewEngine).
func NewService(base map[string]int64, weekendFactor float64) *Service {
	return NewEngine(base, weekendFactor)
}

// NewEngineWithQuoteStore membuat instance Engine dengan quote store kustom (misal: Redis/Valkey).
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
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.quoteStore
}

// SetQuoteStore mengganti quote store aktif pada engine (BE-R11).
func (e *Engine) SetQuoteStore(qs QuoteStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if qs != nil {
		e.quoteStore = qs
	}
}

// SetCalendarStore menghubungkan repository kalender tarif dinamis ke engine.
func (e *Engine) SetCalendarStore(cs RateCalendarStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calendarStore = cs
}

// CalendarStore mengembalikan instance kalender tarif aktif.
func (e *Engine) CalendarStore() RateCalendarStore {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.calendarStore
}

// SetPromoStore menghubungkan repository kampanye promo dinamis ke engine.
func (e *Engine) SetPromoStore(ps PromoStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.promoStore = ps
}

// PromoStore mengembalikan instance promo store aktif.
func (e *Engine) PromoStore() PromoStore {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.promoStore
}

// dailyBaseRate menghitung tarif kamar per malam dengan faktor pengali akhir pekan (weekendFactor).
func (e *Engine) dailyBaseRate(base int64, d time.Time) int64 {
	switch d.Weekday() {
	case time.Friday, time.Saturday:
		return int64(float64(base) * e.weekendFactor)
	default:
		return base
	}
}

// Quote menghitung harga per malam untuk rentang half-open [from, to).
func (e *Engine) Quote(ctx context.Context, roomTypeID string, from, to time.Time) ([]Quote, error) {
	var base int64
	var found bool

	e.mu.RLock()
	src := e.baseSource
	staticBase, staticOk := e.base[roomTypeID]
	calStore := e.calendarStore
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

	if !found && staticOk {
		if staticBase <= 0 {
			return nil, ErrUnpricedRoomType
		}
		base = staticBase
		found = true
	}

	if !found {
		return nil, ErrUnknownRoomType
	}

	// Evaluasi calendar override jika store tersedia
	if calStore != nil {
		stayNights := int(to.Sub(from).Hours() / 24)
		overrides, err := calStore.GetCalendar(ctx, from, to, "RO", &roomTypeID)
		if err == nil && len(overrides) > 0 {
			overrideMap := make(map[string]CalendarOverride, len(overrides))
			for _, o := range overrides {
				dateKey := o.Date.Format("2006-01-02")
				overrideMap[dateKey] = o
			}

			// Periksa restriksi Closed to Departure (CTD) pada tanggal check-out
			if depOverride, ok := overrideMap[to.Format("2006-01-02")]; ok && depOverride.IsCTD {
				return nil, ErrClosedToDeparture
			}

			var out []Quote
			for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
				dateKey := d.Format("2006-01-02")
				o, ok := overrideMap[dateKey]
				if ok {
					if o.IsStopSell {
						return nil, ErrStopSellApplied
					}
					if d.Equal(from) && o.IsCTA {
						return nil, ErrClosedToArrival
					}
					if o.MinLOS > 0 && stayNights < o.MinLOS {
						return nil, ErrMinLengthOfStay
					}
					if o.MaxLOS > 0 && stayNights > o.MaxLOS {
						return nil, ErrMaxLengthOfStay
					}
					if o.PriceOverrideIDR != nil {
						out = append(out, Quote{Date: d, RateMinor: *o.PriceOverrideIDR})
						continue
					}
				}

				out = append(out, Quote{Date: d, RateMinor: e.dailyBaseRate(base, d)})
			}
			return out, nil
		}
	}

	var out []Quote
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		out = append(out, Quote{Date: d, RateMinor: e.dailyBaseRate(base, d)})
	}
	return out, nil
}

// EvaluateAvailability memeriksa ketersediaan kamar, restriksi stop-sell/MLOS, dan menghitung tarif per malam untuk search API.
func (e *Engine) EvaluateAvailability(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, stayNights int) (bool, string, []Quote, error) {
	nightly, err := e.Quote(ctx, roomTypeID, checkIn, checkOut)
	if err != nil {
		switch {
		case errors.Is(err, ErrStopSellApplied):
			return false, ReasonStopSell, nil, nil
		case errors.Is(err, ErrClosedToArrival):
			return false, ReasonClosedToArrival, nil, nil
		case errors.Is(err, ErrClosedToDeparture):
			return false, ReasonClosedToDeparture, nil, nil
		case errors.Is(err, ErrMinLengthOfStay):
			return false, ReasonMinLengthOfStayNotMet, nil, nil
		case errors.Is(err, ErrMaxLengthOfStay):
			return false, ReasonMaxLengthOfStayExceeded, nil, nil
		default:
			return false, "", nil, err
		}
	}
	return true, ReasonAvailable, nightly, nil
}

// calculateBreakfastCharge menghitung total biaya sarapan berdasarkan paket dan demografi tamu (BE-R10).
func calculateBreakfastCharge(planCode string, req QuoteRequest, numNights, numGuests int) int64 {
	if planCode != RatePlanBedBreakfast {
		return 0
	}

	var dailyBreakfast int64
	if req.Adults > 0 || req.Children > 0 {
		dailyBreakfast += int64(req.Adults) * int64(BreakfastRatePerPersonPerNight)
		if len(req.ChildAges) > 0 {
			for _, age := range req.ChildAges {
				switch {
				case age < 6:
					// 0-5 tahun: gratis (complimentary)
				case age <= 11:
					// 6-11 tahun: diskon 50%
					dailyBreakfast += int64(BreakfastRateChildPerNight)
				default:
					// >=12 tahun: tarif dewasa
					dailyBreakfast += int64(BreakfastRatePerPersonPerNight)
				}
			}
		} else if req.Children > 0 {
			dailyBreakfast += int64(req.Children) * int64(BreakfastRateChildPerNight)
		}
	} else {
		// Fallback jika hanya mengirim num_guests agregat
		dailyBreakfast = int64(BreakfastRatePerPersonPerNight) * int64(numGuests)
	}

	return dailyBreakfast * int64(numNights)
}

// evaluatePromo mengevaluasi kode promo dinamis dari PromoStore.
func (e *Engine) evaluatePromo(ctx context.Context, promoCode string, req QuoteRequest, roomSubtotal int64) (int64, string, string, error) {
	defaultPolicy := PolicyFlexible48h
	defaultDesc := "Pembatalan gratis hingga 48 jam sebelum jam 14:00 WIB pada tanggal check-in. Pembatalan setelah batas waktu dikenakan biaya 100%."

	promo := strings.ToUpper(strings.TrimSpace(promoCode))
	if promo == "" {
		return 0, defaultPolicy, defaultDesc, nil
	}

	e.mu.RLock()
	ps := e.promoStore
	e.mu.RUnlock()

	if ps == nil {
		return 0, "", "", ErrInvalidPromoCode
	}

	campaign, err := ps.GetByCode(ctx, promo)
	if err != nil {
		return 0, "", "", ErrInvalidPromoCode
	}

	now := time.Now()
	if !campaign.IsActive || now.Before(campaign.ValidFrom) || now.After(campaign.ValidTo) {
		return 0, "", "", ErrPromoExpired
	}
	if campaign.QuotaUsed >= campaign.QuotaTotal {
		return 0, "", "", ErrPromoQuotaExhausted
	}

	stayNights := int(req.CheckOut.Sub(req.CheckIn).Hours() / 24)
	if stayNights < campaign.MinStayNights {
		return 0, "", "", ErrPromoMinStayNotMet
	}

	if len(campaign.ApplicableRoomTypes) > 0 {
		matched := slices.Contains(campaign.ApplicableRoomTypes, req.RoomTypeID)
		if !matched {
			return 0, "", "", ErrPromoRoomNotApplicable
		}
	}

	var discount int64
	if campaign.DiscountType == "PERCENT" {
		discount = (roomSubtotal * int64(campaign.DiscountValue)) / 100
	} else {
		discount = int64(campaign.DiscountValue)
	}

	if campaign.MaxDiscountIDR != nil && discount > *campaign.MaxDiscountIDR {
		discount = *campaign.MaxDiscountIDR
	}

	desc := fmt.Sprintf("Tarif promo %s tidak dapat dibatalkan atau di-refund.", campaign.Name)
	return discount, PolicyNonRefundable, desc, nil
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
	if req.Adults > 0 || req.Children > 0 {
		numGuests = req.Adults + req.Children
	}
	if numGuests < 1 {
		numGuests = 2
	}

	// Ambil tarif per malam kamar dasar (termasuk validasi stop-sell dan MLOS)
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

	// Biaya sarapan jika paket bed_and_breakfast (BE-R10)
	breakfastCharge := calculateBreakfastCharge(planCode, req, numNights, numGuests)

	// Evaluasi promo code & kebijakan pembatalan
	discount, cancelPolicy, cancelDesc, err := e.evaluatePromo(ctx, req.PromoCode, req, roomSubtotal)
	if err != nil {
		return LockedQuote{}, err
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
		Adults:           req.Adults,
		Children:         req.Children,
		ChildAges:        req.ChildAges,
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
		if err := e.quoteStore.SaveQuote(ctx, lq); err != nil {
			return LockedQuote{}, fmt.Errorf("%w: %v", ErrSaveQuoteFailed, err)
		}
	}

	return lq, nil
}
