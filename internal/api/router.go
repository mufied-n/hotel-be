// Package api menyediakan HTTP handler (transport layer / driving adapter
// dalam hexagonal — desain §8). Handler TIDAK berisi logika domain; ia
// menerjemahkan HTTP ⇄ use case.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/adapter/payment"
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

// Deps adalah dependensi transport layer — semuanya interface domain.
type Deps struct {
	BookingSvc       *booking.Service
	InvStore         inventory.AvailabilityStore
	RateSvc          rates.RateProvider
	RateEngine       *rates.Engine
	QuoteStore       rates.QuoteStore
	CatalogStore     catalog.Store
	Enqueuer         *workers.Enqueuer
	IdempotencyStore IdempotencyStore
	StaffAuth        StaffAuthService
	ReadyCheck       func(ctx context.Context) error
	// FakePay memicu konfirmasi pembayaran pada mode dev (FakeGateway).
	FakePay          gin.HandlerFunc
	// Enforcer untuk evaluasi RBAC Casbin thread-safe.
	Enforcer         *casbin.SyncedEnforcer
	IsDevelopment    bool
	RateLimiter      *RateLimiter
	XenditGateway    *payment.XenditGateway
	GuestSvc         guest.Service
	FinanceSvc       finance.Service
	HousekeepingSvc  housekeeping.Service
	FrontDeskSvc     frontdesk.Service
	StaySvc          stay.Service
	AssistanceSvc    assistance.Service
	FeatureFlag      featureflag.Manager
	NotifierMode     string
}

// NewRouter merakit seluruh route menggunakan Gin engine.
func NewRouter(d Deps) *gin.Engine {
	if d.CatalogStore == nil {
		d.CatalogStore = catalog.NewMemoryStore(catalog.DefaultVariants())
	}
	if d.RateEngine != nil && d.CatalogStore != nil {
		d.RateEngine.SetBaseRateSource(d.CatalogStore)
	}
	if d.RateSvc == nil && d.RateEngine != nil {
		d.RateSvc = d.RateEngine
	}
	if d.QuoteStore == nil && d.RateEngine != nil {
		d.QuoteStore = d.RateEngine.QuoteStore()
	}
	if d.BookingSvc != nil && d.QuoteStore != nil {
		d.BookingSvc.SetQuoteStore(d.QuoteStore)
	}
	if d.IdempotencyStore == nil {
		d.IdempotencyStore = NewMemoryIdempotencyStore()
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 1. RequestID Middleware
	r.Use(func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-Id")
		if reqID == "" {
			reqID = c.GetHeader("X-Request-ID")
		}
		if reqID == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			reqID = hex.EncodeToString(b)
		}
		c.Header("X-Request-Id", reqID)
		c.Next()
	})

	// 2. Panic Recovery (RFC 7807)
	r.Use(func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				writeProblemDetails(c, http.StatusInternalServerError, "Internal Server Error",
					fmt.Sprintf("recovered from panic: %v", rec), "INTERNAL_ERROR")
				c.Abort()
			}
		}()
		c.Next()
	})

	// 3. Timeout Context Middleware (30 detik)
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// 4. Rate Limiter Middleware
	if d.RateLimiter != nil {
		r.Use(d.RateLimiter.Limit())
	}

	r.Match([]string{http.MethodGet, http.MethodHead}, "/healthz", healthz)
	r.Match([]string{http.MethodGet, http.MethodHead}, "/ready", ready(d))
	r.POST("/api/v1/webhooks/xendit", RequireFeature(d.FeatureFlag, "ff_xendit_payment_gateway"), xenditWebhook(d))

	// Guest Auth & My Bookings (F02 & F03)
	r.POST("/api/v1/auth/guest/challenge", RequireFeature(d.FeatureFlag, "ff_guest_portal_auth"), handleGuestChallenge(d))
	r.POST("/api/v1/auth/guest/verify", RequireFeature(d.FeatureFlag, "ff_guest_portal_auth"), handleGuestVerify(d))

	// Staff Auth (BE-R01): login publik; me/logout wajib sesi staf terverifikasi.
	r.POST("/api/v1/auth/staff/login", handleStaffLogin(d))
	staffAuthGroup := r.Group("/api/v1/auth/staff")
	staffAuthGroup.Use(IdentifySubject(staffVerifierOrNil(d.StaffAuth)), requireStaffSession())
	staffAuthGroup.GET("/me", handleStaffMe(d))
	staffAuthGroup.POST("/logout", handleStaffLogout(d))

	guestGroup := r.Group("")
	guestGroup.Use(requireGuestSession(d.GuestSvc))
	{
		guestGroup.GET("/api/v1/auth/guest/me", RequireFeature(d.FeatureFlag, "ff_guest_portal_auth"), handleGuestMe(d))
		guestGroup.POST("/api/v1/auth/guest/logout", RequireFeature(d.FeatureFlag, "ff_guest_portal_auth"), handleGuestLogout(d))
		guestGroup.GET("/api/v1/guest/bookings", RequireFeature(d.FeatureFlag, "ff_guest_my_bookings"), handleGuestBookings(d))
		guestGroup.GET("/api/v1/guest/bookings/:id", RequireFeature(d.FeatureFlag, "ff_guest_my_bookings"), handleGuestBookingDetail(d))
		guestGroup.GET("/api/v1/guest/bookings/:id/receipt", RequireFeature(d.FeatureFlag, "ff_booking_artifacts_receipt"), handleGuestBookingReceipt(d))
		guestGroup.GET("/api/v1/guest/bookings/:id/calendar.ics", RequireFeature(d.FeatureFlag, "ff_booking_artifacts_icalendar"), handleGuestBookingCalendar(d))
		guestGroup.GET("/api/v1/guest/bookings/:id/refund-status", RequireFeature(d.FeatureFlag, "ff_guest_my_bookings"), handleGuestRefundStatus(d))
		guestGroup.POST("/api/v1/guest/bookings/:id/special-requests", RequireFeature(d.FeatureFlag, "ff_guest_special_requests"), handleCreateGuestSpecialRequest(d))
		guestGroup.GET("/api/v1/guest/bookings/:id/special-requests", RequireFeature(d.FeatureFlag, "ff_guest_special_requests"), handleListGuestSpecialRequests(d))
	}

	// API routes dengan identifikasi subjek dan proteksi RBAC Casbin (fail-closed: BE-G14)
	apiGroup := r.Group("")
	apiGroup.Use(IdentifySubject(staffVerifierOrNil(d.StaffAuth)))
	apiGroup.Use(Authorize(d.Enforcer))
	{
		apiGroup.GET("/api/v1/catalog/rooms", getCatalogRooms(d))
		apiGroup.GET("/api/v1/catalog/rooms/:id", getCatalogRoom(d))
		apiGroup.POST("/api/v1/catalog/rooms", RequireFeature(d.FeatureFlag, "ff_catalog_write"), createCatalogRoom(d))
		apiGroup.PUT("/api/v1/catalog/rooms/:id", RequireFeature(d.FeatureFlag, "ff_catalog_write"), updateCatalogRoom(d))
		apiGroup.DELETE("/api/v1/catalog/rooms/:id", RequireFeature(d.FeatureFlag, "ff_catalog_write"), deleteCatalogRoom(d))
		apiGroup.GET("/api/v1/search", RequireFeature(d.FeatureFlag, "ff_multi_variant_search"), searchRooms(d))
		apiGroup.GET("/api/v1/availability", getAvailability(d))
		apiGroup.POST("/api/v1/quotes", RequireFeature(d.FeatureFlag, "ff_quote_locking_engine"), calculateQuote(d))
		apiGroup.POST("/api/v1/bookings", createBooking(d))
		apiGroup.GET("/api/v1/bookings/:id", getBooking(d))
		apiGroup.POST("/api/v1/bookings/:id/cancel", cancelBooking(d))
		apiGroup.POST("/api/v1/bookings/:id/check-in", checkIn(d))
		apiGroup.POST("/api/v1/bookings/:id/check-out", checkOut(d))
		apiGroup.POST("/api/v1/bookings/:id/no-show", noShow(d))

		// Finance Reconciliation & Refunds (F14)
		apiGroup.POST("/api/v1/finance/refunds", RequireFeature(d.FeatureFlag, "ff_gateway_automated_refund"), handleFinanceRefund(d))
		apiGroup.GET("/api/v1/finance/cases", RequireFeature(d.FeatureFlag, "ff_finance_reconciliation"), handleFinanceCases(d))
		apiGroup.POST("/api/v1/finance/cases/:id/resolve", RequireFeature(d.FeatureFlag, "ff_finance_reconciliation"), handleFinanceResolveCase(d))
		apiGroup.GET("/api/v1/finance/reconciliations", RequireFeature(d.FeatureFlag, "ff_finance_reconciliation"), handleFinanceSummary(d))

		// Housekeeping Room Status & Readiness Lifecycle (Proposed 01)
		apiGroup.GET("/api/v1/housekeeping/rooms", RequireFeature(d.FeatureFlag, "ff_housekeeping_board"), handleHousekeepingRooms(d))
		apiGroup.PUT("/api/v1/housekeeping/rooms/:id/status", RequireFeature(d.FeatureFlag, "ff_housekeeping_board"), handleHousekeepingStatus(d))
		apiGroup.POST("/api/v1/housekeeping/rooms/:id/out-of-order", RequireFeature(d.FeatureFlag, "ff_housekeeping_board"), handleHousekeepingOOO(d))

		// Front Desk Daily Operations Roster & Shift Handover Board (Proposed 02)
		apiGroup.GET("/api/v1/front-desk/daily-roster", RequireFeature(d.FeatureFlag, "ff_front_desk_operations"), handleFrontDeskDailyRoster(d))
		apiGroup.GET("/api/v1/front-desk/handover-notes", RequireFeature(d.FeatureFlag, "ff_front_desk_operations"), handleFrontDeskListHandovers(d))
		apiGroup.POST("/api/v1/front-desk/handover-notes", RequireFeature(d.FeatureFlag, "ff_front_desk_operations"), handleFrontDeskRecordHandover(d))

		// Stay Modification: Room Move & Stay Extension (Proposed 03)
		apiGroup.POST("/api/v1/bookings/:id/room-move", RequireFeature(d.FeatureFlag, "ff_stay_modification"), handleRoomMove(d))
		apiGroup.POST("/api/v1/bookings/:id/extend-stay", RequireFeature(d.FeatureFlag, "ff_stay_modification"), handleExtendStay(d))
		apiGroup.GET("/api/v1/bookings/:id/room-moves", RequireFeature(d.FeatureFlag, "ff_stay_modification"), handleListRoomMoves(d))

		// Guest Special Requests & Stay Assistance Desk (Proposed 04)
		apiGroup.GET("/api/v1/front-desk/special-requests", RequireFeature(d.FeatureFlag, "ff_guest_special_requests"), handleListStaffSpecialRequests(d))
		apiGroup.PUT("/api/v1/front-desk/special-requests/:id/status", RequireFeature(d.FeatureFlag, "ff_guest_special_requests"), handleUpdateStaffSpecialRequestStatus(d))

		// Feature Flags Administration (FR-FF-04)
		apiGroup.GET("/api/v1/admin/feature-flags", handleAdminListFlags(d))
		apiGroup.PUT("/api/v1/admin/feature-flags/:key", handleAdminUpdateFlag(d))

		// Dev-only: simulasi pembayaran sukses (BE-G10: gate development only)
		if d.IsDevelopment && d.FakePay != nil {
			apiGroup.POST("/fake-pay/:ref", d.FakePay)
		}
	}

	return r
}

func healthz(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

func ready(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ReadyCheck != nil {
			if err := d.ReadyCheck(c.Request.Context()); err != nil {
				writeJSON(c, http.StatusServiceUnavailable, map[string]string{
					"status": "unavailable",
					"error":  err.Error(),
				})
				return
			}
		}
		env := "production"
		if d.IsDevelopment {
			env = "development"
		}
		payMode := "fake"
		if d.XenditGateway != nil {
			payMode = "xendit"
		}
		notifMode := d.NotifierMode
		if notifMode == "" {
			notifMode = "log"
		}
		writeJSON(c, http.StatusOK, map[string]string{
			"status":          "ready",
			"environment":     env,
			"payment_gateway": payMode,
			"notifier":        notifMode,
		})
	}
}

// GET /api/v1/catalog/rooms (BE-G01)
func getCatalogRooms(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		variants, err := d.CatalogStore.ListVariants(c.Request.Context())
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membaca katalog kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, map[string]any{
			"total": len(variants),
			"rooms": variants,
		})
	}
}

// GET /api/v1/catalog/rooms/{id}
func getCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		v, err := d.CatalogStore.GetVariant(c.Request.Context(), id)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(c, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, v)
	}
}

// POST /api/v1/catalog/rooms
func createCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var v catalog.RoomVariant
		if err := json.UnmarshalRead(c.Request.Body, &v); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_ROOM_PAYLOAD")
			return
		}
		created, err := d.CatalogStore.CreateVariant(c.Request.Context(), v)
		if errors.Is(err, catalog.ErrInvalidVariant) {
			httpErrorCode(c, http.StatusBadRequest, "kode, nama, kapasitas, dan harga dasar wajib diisi", "INVALID_ROOM_DATA")
			return
		}
		if errors.Is(err, catalog.ErrDuplicateCode) {
			httpErrorCode(c, http.StatusConflict, "kode varian kamar sudah digunakan", "CONFLICT_ROOM_CODE")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membuat varian kamar", "CATALOG_ERROR")
			return
		}
		if d.RateEngine != nil {
			d.RateEngine.SetBaseRate(created.ID, created.BasePriceMinor)
		}
		writeJSON(c, http.StatusCreated, created)
	}
}

// PUT /api/v1/catalog/rooms/{id}
func updateCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var v catalog.RoomVariant
		if err := json.UnmarshalRead(c.Request.Body, &v); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_ROOM_PAYLOAD")
			return
		}
		updated, err := d.CatalogStore.UpdateVariant(c.Request.Context(), id, v)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(c, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if errors.Is(err, catalog.ErrInvalidVariant) {
			httpErrorCode(c, http.StatusBadRequest, "kode, nama, kapasitas, dan harga dasar wajib diisi", "INVALID_ROOM_DATA")
			return
		}
		if errors.Is(err, catalog.ErrDuplicateCode) {
			httpErrorCode(c, http.StatusConflict, "kode varian kamar sudah digunakan", "CONFLICT_ROOM_CODE")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal memperbarui varian kamar", "CATALOG_ERROR")
			return
		}
		if d.RateEngine != nil {
			d.RateEngine.SetBaseRate(updated.ID, updated.BasePriceMinor)
		}
		writeJSON(c, http.StatusOK, updated)
	}
}

// DELETE /api/v1/catalog/rooms/{id}
func deleteCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		err := d.CatalogStore.DeleteVariant(c.Request.Context(), id)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(c, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if errors.Is(err, catalog.ErrCannotDelete) {
			httpErrorCode(c, http.StatusConflict, "tidak dapat menghapus varian yang masih digunakan dalam inventaris atau booking", "CANNOT_DELETE_ACTIVE_VARIANT")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal menghapus varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}
}

// SearchResultItem merepresentasikan opsi kamar hasil pencarian lintas varian (BE-G02).
type SearchResultItem struct {
	RoomVariant       catalog.RoomVariant `json:"room_variant"`
	Available         bool                `json:"available"`
	AvailableRooms    int                 `json:"available_rooms"`
	UnavailableReason string              `json:"unavailable_reason,omitempty"`
	TotalPriceMinor   int64               `json:"total_price_minor"`
	Currency          string              `json:"currency"`
	Quotes            []rates.Quote       `json:"quotes"`
}

// GET /api/v1/search?check_in=YYYY-MM-DD&check_out=YYYY-MM-DD&adults=1&children=0&rooms=1&child_ages=5,8
func searchRooms(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		checkInStr := c.Query("check_in")
		checkOutStr := c.Query("check_out")

		from, err1 := parseDate(checkInStr)
		to, err2 := parseDate(checkOutStr)
		if checkInStr == "" || checkOutStr == "" || err1 != nil || err2 != nil {
			httpErrorCode(c, http.StatusBadRequest, "check_in and check_out (YYYY-MM-DD) are required", "INVALID_DATE_FORMAT")
			return
		}
		if !from.Before(to) {
			httpErrorCode(c, http.StatusBadRequest, "check_out must be after check_in", "INVALID_DATE_RANGE")
			return
		}

		nights := int(to.Sub(from).Hours() / 24)
		if nights > 30 {
			httpErrorCode(c, http.StatusBadRequest, "stay duration cannot exceed 30 nights", "EXCEEDS_MAX_LOS")
			return
		}

		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		checkInDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		if checkInDate.Before(today.Add(-24 * time.Hour)) {
			httpErrorCode(c, http.StatusBadRequest, "check_in date cannot be in the past", "PAST_DATE")
			return
		}
		if to.After(now.AddDate(0, 0, 366)) {
			httpErrorCode(c, http.StatusBadRequest, "search dates cannot exceed 365 days booking horizon", "EXCEEDS_HORIZON")
			return
		}

		adults := 1
		if s := c.Query("adults"); s != "" {
			var err error
			adults, err = strconv.Atoi(s)
			if err != nil || adults < 1 {
				httpErrorCode(c, http.StatusBadRequest, "adults must be at least 1", "INVALID_GUEST_COUNT")
				return
			}
		}

		rooms := 1
		if s := c.Query("rooms"); s != "" {
			var err error
			rooms, err = strconv.Atoi(s)
			if err != nil || rooms < 1 || rooms > 8 {
				httpErrorCode(c, http.StatusBadRequest, "rooms must be between 1 and 8", "INVALID_ROOM_COUNT")
				return
			}
		}

		if adults < rooms {
			httpErrorCode(c, http.StatusBadRequest, "adults count must be greater than or equal to rooms count", "INVALID_GUEST_COUNT")
			return
		}

		children := 0
		hasChildrenParam := false
		if s := c.Query("children"); s != "" {
			hasChildrenParam = true
			var err error
			children, err = strconv.Atoi(s)
			if err != nil || children < 0 {
				httpErrorCode(c, http.StatusBadRequest, "children cannot be negative", "INVALID_GUEST_COUNT")
				return
			}
		}

		childAgesStr := c.Query("child_ages")
		if hasChildrenParam && children == 0 && childAgesStr != "" {
			httpErrorCode(c, http.StatusBadRequest, "child_ages cannot be provided when children is 0", "CHILD_AGE_COUNT_MISMATCH")
			return
		}

		if childAgesStr != "" {
			var childAges []int
			for _, ageStr := range strings.Split(childAgesStr, ",") {
				trimmed := strings.TrimSpace(ageStr)
				if trimmed == "" {
					continue
				}
				age, err := strconv.Atoi(trimmed)
				if err != nil || age < 0 || age > 17 {
					httpErrorCode(c, http.StatusBadRequest, "child age must be between 0 and 17", "INVALID_CHILD_AGE")
					return
				}
				childAges = append(childAges, age)
			}
			if hasChildrenParam && len(childAges) != children {
				httpErrorCode(c, http.StatusBadRequest, "child_ages count must match children count", "CHILD_AGE_COUNT_MISMATCH")
				return
			}
			if !hasChildrenParam {
				children = len(childAges)
			}
		} else if hasChildrenParam && children > 0 {
			httpErrorCode(c, http.StatusBadRequest, "child_ages count must match children count", "CHILD_AGE_COUNT_MISMATCH")
			return
		}

		variants, err := d.CatalogStore.ListVariants(c.Request.Context())
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
			return
		}

		var results []SearchResultItem
		availableCount := 0

		for _, v := range variants {
			item := SearchResultItem{
				RoomVariant: v,
				Currency:    "IDR",
			}

			// Kapasitas okupansi (BE-G03, BE-R07):
			// - Tamu per kamar tidak boleh melebihi max_capacity
			// - Dewasa per kamar tidak boleh melebihi max_adults
			// - Anak per kamar tidak boleh melebihi max_children
			totalGuests := adults + children
			if totalGuests > v.MaxCapacity*rooms || adults > v.MaxAdults*rooms || children > v.MaxChildren*rooms {
				item.Available = false
				item.UnavailableReason = "EXCEEDS_CAPACITY"
				results = append(results, item)
				continue
			}

			// Periksa ketersediaan multi-malam kontinu (BE-G02)
			avail, err := d.InvStore.GetByDate(c.Request.Context(), v.ID, from, to)
			if errors.Is(err, inventory.ErrNotFound) {
				item.Available = false
				item.UnavailableReason = "MISSING_INVENTORY"
				results = append(results, item)
				continue
			}
			if err != nil {
				httpErrorCode(c, http.StatusInternalServerError, "gagal memeriksa ketersediaan kamar", "INVENTORY_ERROR")
				return
			}
			if len(avail) < nights {
				item.Available = false
				item.UnavailableReason = "MISSING_INVENTORY"
				results = append(results, item)
				continue
			}

			// Hitung kuotasi tarif (BE-R09: pricing guard)
			quotes, err := d.RateSvc.Quote(c.Request.Context(), v.ID, from, to)
			if err != nil || len(quotes) == 0 {
				item.Available = false
				item.UnavailableReason = "RATE_UNAVAILABLE"
				results = append(results, item)
				continue
			}
			item.Quotes = quotes
			item.TotalPriceMinor = sumQuotes(quotes) * int64(rooms)
			if item.TotalPriceMinor <= 0 {
				item.Available = false
				item.UnavailableReason = "RATE_UNAVAILABLE"
				results = append(results, item)
				continue
			}

			// Cari sisa kamar minimum sepanjang rentang menginap
			minAvail := avail[0].AvailableRooms
			for _, a := range avail {
				if a.AvailableRooms < minAvail {
					minAvail = a.AvailableRooms
				}
			}

			item.AvailableRooms = minAvail
			if minAvail >= rooms {
				item.Available = true
				availableCount++
			} else if minAvail == 0 {
				item.Available = false
				item.UnavailableReason = "SOLD_OUT"
			} else {
				item.Available = false
				item.UnavailableReason = "INSUFFICIENT_ROOMS"
			}

			results = append(results, item)
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"search_criteria": map[string]any{
				"check_in":  checkInStr,
				"check_out": checkOutStr,
				"nights":    nights,
				"rooms":     rooms,
				"adults":    adults,
				"children":  children,
			},
			"total_variants":  len(results),
			"available_count": availableCount,
			"results":         results,
		})
	}
}

// GET /api/v1/availability?room_type_id=...&check_in=YYYY-MM-DD&check_out=YYYY-MM-DD
func getAvailability(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomTypeID := c.Query("room_type_id")
		from, err1 := parseDate(c.Query("check_in"))
		to, err2 := parseDate(c.Query("check_out"))
		if roomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			httpErrorCode(c, http.StatusBadRequest, "room_type_id, check_in, check_out (YYYY-MM-DD, check_in < check_out) wajib", "INVALID_QUERY")
			return
		}
		avail, err := d.InvStore.GetByDate(c.Request.Context(), roomTypeID, from, to)
		if errors.Is(err, inventory.ErrNotFound) {
			httpErrorCode(c, http.StatusNotFound, "inventory tidak ditemukan untuk rentang tsb", "INVENTORY_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membaca availability", "INTERNAL_ERROR")
			return
		}
		quotes, err := d.RateSvc.Quote(c.Request.Context(), roomTypeID, from, to)
		if err != nil {
			httpErrorCode(c, http.StatusNotFound, "tipe kamar tidak dikenal", "UNKNOWN_ROOM_TYPE")
			return
		}
		writeJSON(c, http.StatusOK, map[string]any{
			"availability": avail,
			"quotes":       quotes,
			"total_minor":  sumQuotes(quotes),
		})
	}
}

// POST /api/v1/quotes
func calculateQuote(d Deps) gin.HandlerFunc {
	type req struct {
		RoomTypeID   string `json:"room_type_id" validate:"required"`
		RatePlanCode string `json:"rate_plan_code"`
		CheckIn      string `json:"check_in" validate:"required"`
		CheckOut     string `json:"check_out" validate:"required"`
		NumRooms     int    `json:"num_rooms"`
		NumGuests    int    `json:"num_guests"`
		Adults       int    `json:"adults,omitempty"`
		Children     int    `json:"children,omitempty"`
		ChildAges    []int  `json:"child_ages,omitempty"`
		PromoCode    string `json:"promo_code"`
	}
	return func(c *gin.Context) {
		var in req
		if err := json.UnmarshalRead(c.Request.Body, &in); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}
		if !validateDTO(c, &in) {
			return
		}
		from, err1 := parseDate(in.CheckIn)
		to, err2 := parseDate(in.CheckOut)
		if in.RoomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			httpErrorCode(c, http.StatusBadRequest, "room_type_id, check_in, check_out (check_in < check_out) wajib valid", "INVALID_DATE_FORMAT")
			return
		}

		numRooms := in.NumRooms
		if numRooms <= 0 {
			numRooms = 1
		}
		if numRooms > 8 {
			httpErrorCode(c, http.StatusBadRequest, "rooms must be between 1 and 8", "INVALID_ROOM_COUNT")
			return
		}

		numGuests := in.NumGuests
		if in.Adults > 0 || in.Children > 0 || len(in.ChildAges) > 0 {
			if in.Adults < numRooms {
				httpErrorCode(c, http.StatusBadRequest, "adults count must be greater than or equal to rooms count", "INVALID_GUEST_COUNT")
				return
			}
			if in.Children < 0 {
				httpErrorCode(c, http.StatusBadRequest, "children cannot be negative", "INVALID_GUEST_COUNT")
				return
			}
			if in.Children == 0 && len(in.ChildAges) > 0 {
				httpErrorCode(c, http.StatusBadRequest, "child_ages cannot be provided when children is 0", "CHILD_AGE_COUNT_MISMATCH")
				return
			}
			if in.Children > 0 {
				if len(in.ChildAges) != in.Children {
					httpErrorCode(c, http.StatusBadRequest, "child_ages count must match children count", "CHILD_AGE_COUNT_MISMATCH")
					return
				}
				for _, age := range in.ChildAges {
					if age < 0 || age > 17 {
						httpErrorCode(c, http.StatusBadRequest, "child age must be between 0 and 17", "INVALID_CHILD_AGE")
						return
					}
				}
			}
			numGuests = in.Adults + in.Children
		} else {
			if numGuests < 1 {
				numGuests = 1
			}
			if numGuests < numRooms {
				httpErrorCode(c, http.StatusBadRequest, "guests count must be greater than or equal to rooms count", "INVALID_GUEST_COUNT")
				return
			}
		}

		roomTypeID := in.RoomTypeID
		// Validasi batas fisik kapasitas kamar terhadap katalog (BE-R07)
		if d.CatalogStore != nil {
			variant, err := d.CatalogStore.GetVariant(c.Request.Context(), in.RoomTypeID)
			if err != nil {
				if errors.Is(err, catalog.ErrVariantNotFound) {
					httpErrorCode(c, http.StatusNotFound, "tipe kamar tidak ditemukan", "ROOM_NOT_FOUND")
					return
				}
				httpErrorCode(c, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
				return
			}
			roomTypeID = variant.ID
			if in.Adults > 0 && in.Adults > variant.MaxAdults*numRooms {
				httpErrorCode(c, http.StatusBadRequest, "jumlah dewasa melebihi kapasitas kamar", "EXCEEDS_CAPACITY")
				return
			}
			if in.Children > 0 && in.Children > variant.MaxChildren*numRooms {
				httpErrorCode(c, http.StatusBadRequest, "jumlah anak melebihi kapasitas kamar", "EXCEEDS_CAPACITY")
				return
			}
			if numGuests > variant.MaxCapacity*numRooms {
				httpErrorCode(c, http.StatusBadRequest, "jumlah tamu melebihi kapasitas maksimum varian kamar", "EXCEEDS_CAPACITY")
				return
			}
		}

		if strings.TrimSpace(in.PromoCode) != "" && (d.FeatureFlag != nil && !d.FeatureFlag.IsEnabled(c.Request.Context(), "ff_promotions_engine")) {
			httpErrorCode(c, http.StatusBadRequest, "fitur kode promosi sedang dinonaktifkan sementara", "PROMOTIONS_DISABLED")
			return
		}
		if d.RateEngine == nil {
			httpErrorCode(c, http.StatusInternalServerError, "rate engine not configured", "INTERNAL_ERROR")
			return
		}
		q, err := d.RateEngine.CalculateLockedQuote(c.Request.Context(), rates.QuoteRequest{
			RoomTypeID:   roomTypeID,
			RatePlanCode: in.RatePlanCode,
			CheckIn:      from,
			CheckOut:     to,
			NumRooms:     numRooms,
			NumGuests:    numGuests,
			Adults:       in.Adults,
			Children:     in.Children,
			ChildAges:    in.ChildAges,
			PromoCode:    in.PromoCode,
		})
		if errors.Is(err, rates.ErrInvalidRatePlan) {
			httpErrorCode(c, http.StatusBadRequest, "kode rate plan tidak valid", "INVALID_RATE_PLAN")
			return
		}
		if errors.Is(err, rates.ErrInvalidPromoCode) {
			httpErrorCode(c, http.StatusBadRequest, "kode promo tidak valid atau kedaluwarsa", "INVALID_PROMO_CODE")
			return
		}
		if errors.Is(err, rates.ErrUnknownRoomType) {
			httpErrorCode(c, http.StatusNotFound, "tipe kamar tidak ditemukan", "ROOM_NOT_FOUND")
			return
		}
		if errors.Is(err, rates.ErrUnpricedRoomType) {
			httpErrorCode(c, http.StatusBadRequest, "tarif dasar kamar belum dikonfigurasi", "RATE_UNAVAILABLE")
			return
		}
		if errors.Is(err, rates.ErrSaveQuoteFailed) {
			httpErrorCode(c, http.StatusInternalServerError, "gagal menyimpan kuotasi harga", "INTERNAL_ERROR")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
			return
		}
		writeJSON(c, http.StatusOK, q)
	}
}

// POST /api/v1/bookings
func createBooking(d Deps) gin.HandlerFunc {
	type req struct {
		QuoteID              string `json:"quote_id"`
		TermsAccepted        bool   `json:"terms_accepted"`
		PrivacyAccepted      bool   `json:"privacy_accepted"`
		RoomTypeID           string `json:"room_type_id"`
		CheckIn              string `json:"check_in"`
		CheckOut             string `json:"check_out"`
		NumRooms             int    `json:"num_rooms"`
		NumGuests            int    `json:"num_guests"`
		GuestName            string `json:"guest_name"`
		GuestEmail           string `json:"guest_email"`
		GuestPhone           string `json:"guest_phone,omitempty"`
		EstimatedArrivalTime string `json:"estimated_arrival_time,omitempty"`
		SpecialRequests      string `json:"special_requests,omitempty"`
	}
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpErrorCode(c, http.StatusBadRequest, "gagal membaca request body", "INVALID_BODY")
			return
		}

		idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		useIdem := idempotencyKey != "" && d.IdempotencyStore != nil &&
			(d.FeatureFlag == nil || d.FeatureFlag.IsEnabled(c.Request.Context(), "ff_checkout_idempotency"))
		reqHash := hashBody(bodyBytes)
		completed := false
		if useIdem {
			// Klaim atomik sebelum efek samping apa pun (BE-R08).
			rec, acquired, err := d.IdempotencyStore.Reserve(c.Request.Context(), idempotencyKey, reqHash)
			if err != nil {
				httpErrorCode(c, http.StatusServiceUnavailable, "idempotency store tidak tersedia", "IDEMPOTENCY_UNAVAILABLE")
				return
			}
			if !acquired {
				switch {
				case rec.RequestHash != reqHash:
					httpErrorCode(c, http.StatusConflict, "idempotency key reused with different request payload", "IDEMPOTENCY_CONFLICT")
				case rec.InProgress():
					c.Header("Retry-After", "1")
					httpErrorCode(c, http.StatusConflict, "request dengan idempotency key ini masih diproses", "IDEMPOTENCY_IN_PROGRESS")
				default:
					c.Header("Content-Type", "application/json; charset=utf-8")
					c.Header("Idempotency-Replayed", "true")
					c.Status(rec.ResponseCode)
					_, _ = c.Writer.Write([]byte(rec.ResponseBody))
				}
				return
			}
			// Lepas reservasi pada setiap jalur gagal agar client dapat retry.
			defer func() {
				if !completed {
					_ = d.IdempotencyStore.Release(context.WithoutCancel(c.Request.Context()), idempotencyKey)
				}
			}()
		}

		var in req
		if err := json.Unmarshal(bodyBytes, &in); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}
		if !validateDTO(c, &in) {
			return
		}
		from, err1 := parseDate(in.CheckIn)
		to, err2 := parseDate(in.CheckOut)
		if err1 != nil || err2 != nil {
			httpErrorCode(c, http.StatusBadRequest, "check_in/check_out wajib format YYYY-MM-DD", "INVALID_DATE_FORMAT")
			return
		}

		roomsCount := in.NumRooms
		if roomsCount <= 0 {
			roomsCount = 1
		}
		if in.NumGuests > 0 && in.NumGuests < roomsCount {
			httpErrorCode(c, http.StatusBadRequest, "guests count must be greater than or equal to rooms count", "INVALID_GUEST_COUNT")
			return
		}
		if d.CatalogStore != nil && in.RoomTypeID != "" {
			variant, err := d.CatalogStore.GetVariant(c.Request.Context(), in.RoomTypeID)
			if err != nil && errors.Is(err, catalog.ErrVariantNotFound) {
				httpErrorCode(c, http.StatusNotFound, "tipe kamar tidak ditemukan", "ROOM_NOT_FOUND")
				return
			}
			if err == nil {
				in.RoomTypeID = variant.ID
				if in.NumGuests > variant.MaxCapacity*roomsCount {
					httpErrorCode(c, http.StatusBadRequest, "jumlah tamu melebihi kapasitas maksimum varian kamar", "EXCEEDS_CAPACITY")
					return
				}
			}
		}

		b, charge, err := d.BookingSvc.Create(c.Request.Context(), booking.CreateInput{
			QuoteID:              in.QuoteID,
			TermsAccepted:        in.TermsAccepted,
			PrivacyAccepted:      in.PrivacyAccepted,
			RoomTypeID:           in.RoomTypeID,
			CheckIn:              from,
			CheckOut:             to,
			NumRooms:             in.NumRooms,
			NumGuests:            in.NumGuests,
			GuestName:            in.GuestName,
			GuestEmail:           in.GuestEmail,
			GuestPhone:           in.GuestPhone,
			EstimatedArrivalTime: in.EstimatedArrivalTime,
			SpecialRequests:      in.SpecialRequests,
		})
		if errors.Is(err, booking.ErrExceedsCapacity) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "EXCEEDS_CAPACITY")
			return
		}
		if errors.Is(err, booking.ErrConsentRequired) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "CONSENT_REQUIRED")
			return
		}
		if errors.Is(err, booking.ErrQuoteRequired) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "QUOTE_REQUIRED")
			return
		}
		if errors.Is(err, booking.ErrQuoteExpired) {
			httpErrorCode(c, http.StatusGone, err.Error(), "QUOTE_EXPIRED")
			return
		}
		if errors.Is(err, booking.ErrQuoteAlreadyUsed) {
			httpErrorCode(c, http.StatusConflict, err.Error(), "QUOTE_ALREADY_USED")
			return
		}
		if errors.Is(err, booking.ErrQuoteMismatch) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "QUOTE_MISMATCH")
			return
		}
		if errors.Is(err, booking.ErrInvalidPhone) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_PHONE")
			return
		}
		if errors.Is(err, booking.ErrInvalidArrivalTime) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_ARRIVAL_TIME")
			return
		}
		if errors.Is(err, booking.ErrSpecialRequestTooLong) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "SPECIAL_REQUEST_TOO_LONG")
			return
		}
		if errors.Is(err, booking.ErrInvalidDateRange) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_DATE_RANGE")
			return
		}
		if errors.Is(err, booking.ErrInvalidCapacity) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_CAPACITY")
			return
		}
		if errors.Is(err, booking.ErrExceedsMaxStay) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "EXCEEDS_MAX_LOS")
			return
		}
		if errors.Is(err, booking.ErrPastDate) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "PAST_DATE")
			return
		}
		if errors.Is(err, booking.ErrExceedsHorizon) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "EXCEEDS_HORIZON")
			return
		}
		if errors.Is(err, booking.ErrInvalidGuestInfo) {
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_GUEST_INFO")
			return
		}
		if errors.Is(err, inventory.ErrInsufficient) || errors.Is(err, booking.ErrInsufficient) {
			httpErrorCode(c, http.StatusConflict, "kamar tidak tersedia untuk rentang tsb", "INSUFFICIENT_ROOMS")
			return
		}
		if errors.Is(err, inventory.ErrNotFound) {
			httpErrorCode(c, http.StatusNotFound, "inventory tidak ditemukan", "INVENTORY_NOT_FOUND")
			return
		}
		if errors.Is(err, booking.ErrPaymentGatewayTimeout) {
			httpErrorCode(c, http.StatusGatewayTimeout, "koneksi gateway pembayaran terputus, reservasi tetap tersimpan dalam antrean pemulihan", "GATEWAY_TIMEOUT")
			return
		}
		if errors.Is(err, booking.ErrPaymentDefinitiveFailure) {
			httpErrorCode(c, http.StatusBadGateway, "gateway pembayaran menolak transaksi", "PAYMENT_FAILED")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membuat booking", "INTERNAL_ERROR")
			return
		}
		// Jadwalkan release-hold otomatis t+holdTimeout jika enqueuer aktif (asynq scheduled task).
		if d.Enqueuer != nil {
			_ = d.Enqueuer.EnqueueReleaseHold(c.Request.Context(), b.ID, d.BookingSvc.HoldTimeout())
		}

		respObj := map[string]any{
			"booking":            b,
			"guest_access_token": b.GuestToken,
			"payment_url":        charge.PaymentURL,
			"reference":          charge.Reference,
			"expires_at":         b.ExpiresAt,
			"server_time":        time.Now().UTC(),
		}
		respBytes, _ := json.Marshal(respObj)

		if useIdem {
			now := time.Now().UTC()
			if err := d.IdempotencyStore.Complete(c.Request.Context(), IdempotencyRecord{
				Key:          idempotencyKey,
				RequestHash:  reqHash,
				ResponseCode: http.StatusCreated,
				ResponseBody: string(respBytes),
				CreatedAt:    now,
				ExpiresAt:    now.Add(idempotencyResultTTL),
			}); err != nil {
				// Booking sudah commit; reservasi in-progress kedaluwarsa sendiri (lihat risiko residual di dok tech).
				slog.Error("idempotency.complete_failed", "key", idempotencyKey, "booking_id", b.ID, "error", err)
			}
			// Booking sudah ada: reservasi tidak boleh dilepas walau Complete gagal.
			completed = true
		}

		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Status(http.StatusCreated)
		_, _ = c.Writer.Write(respBytes)
	}
}

// GET /api/v1/bookings/{id}
func getBooking(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		b, err := d.BookingSvc.Get(c.Request.Context(), c.Param("id"))
		if errors.Is(err, booking.ErrNotFound) {
			httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal membaca booking", "INTERNAL_ERROR")
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
		guestToken := GetGuestToken(c.Request.Context())

		// BE-G13, BE-R02: Privasi data tamu (PII) dan perlindungan token sesi.
		// Jika caller adalah staff (bukan guest) ATAU memiliki guest_token yang valid ATAU pii masking guard dinonaktifkan:
		// kembalikan data booking lengkap. Pastikan b.GuestToken di-nolkan agar token kredensial
		// tidak pernah bocor pada operasi read ke staf maupun ke client (BE-R02).
		if (d.FeatureFlag != nil && !d.FeatureFlag.IsEnabled(c.Request.Context(), "ff_pii_masking_guard")) || authCtx.Role != "guest" || (b.GuestToken != "" && guestToken == b.GuestToken) {
			b.GuestToken = ""
			writeJSON(c, http.StatusOK, b)
			return
		}

		writeJSON(c, http.StatusOK, b.ToPublicDTO())
	}
}

// POST /api/v1/bookings/{id}/cancel
func cancelBooking(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		authCtx := GetAuthContext(c.Request.Context())
		guestToken := GetGuestToken(c.Request.Context())

		// BE-G13: Guest hanya diizinkan membatalkan jika memiliki guest_token yang cocok
		if authCtx.Role == "guest" {
			b, err := d.BookingSvc.Get(c.Request.Context(), id)
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if err != nil {
				httpErrorCode(c, http.StatusInternalServerError, "gagal membaca booking", "INTERNAL_ERROR")
				return
			}
			if b.GuestToken == "" || guestToken != b.GuestToken {
				httpErrorCode(c, http.StatusForbidden, "guest token tidak valid atau tidak memiliki akses pembatalan", "FORBIDDEN_OWNERSHIP")
				return
			}
		}

		if err := d.BookingSvc.Cancel(c.Request.Context(), id); err != nil {
			if errors.Is(err, booking.ErrNonRefundable) && (d.FeatureFlag == nil || d.FeatureFlag.IsEnabled(c.Request.Context(), "ff_strict_cancellation_policy")) {
				httpErrorCode(c, http.StatusConflict, "reservasi non-refundable tidak dapat dibatalkan oleh tamu", "NON_REFUNDABLE_BOOKING")
				return
			}
			if errors.Is(err, booking.ErrCancellationDeadlineExceeded) && (d.FeatureFlag == nil || d.FeatureFlag.IsEnabled(c.Request.Context(), "ff_strict_cancellation_policy")) {
				httpErrorCode(c, http.StatusConflict, "batas waktu pembatalan gratis 48 jam sebelum check-in telah terlewati", "CANCELLATION_DEADLINE_EXCEEDED")
				return
			}
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "ILLEGAL_TRANSITION")
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal membatalkan booking", "INTERNAL_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, map[string]string{"status": "cancelled", "id": id})
	}
}

// POST /api/v1/bookings/{id}/check-in
// confirmed → checked_in + room assignment (dijamin GiST di database).
func checkIn(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if d.FeatureFlag != nil && !d.FeatureFlag.IsEnabled(ctx, "ff_room_readiness_checkin_guard") {
			ctx = booking.WithBypassRoomReadiness(ctx)
		}
		res, err := d.BookingSvc.CheckIn(ctx, c.Param("id"))
		switch {
		case errors.Is(err, booking.ErrNotFound):
			httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			return
		case errors.Is(err, booking.ErrIllegalTransition):
			httpErrorCode(c, http.StatusConflict, err.Error(), "ILLEGAL_TRANSITION")
			return
		case errors.Is(err, booking.ErrRoomNotReady):
			httpErrorCode(c, http.StatusConflict, "kamar belum siap huni (belum diinspeksi oleh housekeeping)", "ROOM_NOT_READY")
			return
		case errors.Is(err, booking.ErrNoRoomAvailable):
			httpErrorCode(c, http.StatusConflict, "tidak ada kamar fisik bebas untuk rentang menginap ini", "NO_ROOM_AVAILABLE")
			return
		case err != nil:
			httpErrorCode(c, http.StatusInternalServerError, "gagal check-in", "INTERNAL_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, res)
	}
}

// POST /api/v1/bookings/{id}/check-out
func checkOut(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := d.BookingSvc.CheckOut(c.Request.Context(), id); err != nil {
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "ILLEGAL_TRANSITION")
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal check-out", "INTERNAL_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, map[string]string{"status": "checked_out", "id": id})
	}
}

// POST /api/v1/bookings/{id}/no-show
func noShow(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := d.BookingSvc.MarkNoShow(c.Request.Context(), id); err != nil {
			if errors.Is(err, booking.ErrNoShowTooEarly) {
				httpErrorCode(c, http.StatusBadRequest, "reservasi belum mencapai tanggal check-in untuk ditandai no-show", "NO_SHOW_TOO_EARLY")
				return
			}
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "ILLEGAL_TRANSITION")
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal memproses no-show", "INTERNAL_ERROR")
			return
		}
		writeJSON(c, http.StatusOK, map[string]string{"status": "no_show", "id": id})
	}
}

// ---------- helpers ----------

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func sumQuotes(qs []rates.Quote) int64 {
	var t int64
	for _, q := range qs {
		t += q.RateMinor
	}
	return t
}

// POST /api/v1/webhooks/xendit (PCI-DSS SAQ A & OWASP API Top 10)
// Menerima notifikasi callback pembayaran dari Xendit secara idempoten dan terverifikasi.
func xenditWebhook(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.XenditGateway == nil {
			httpErrorCode(c, http.StatusNotImplemented, "xendit gateway is not configured", "NOT_CONFIGURED")
			return
		}

		token := c.GetHeader("x-callback-token")
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpErrorCode(c, http.StatusBadRequest, "failed to read webhook body", "BAD_REQUEST")
			return
		}

		payload, err := d.XenditGateway.VerifyWebhook(token, bodyBytes)
		if err != nil {
			if errors.Is(err, payment.ErrInvalidWebhookToken) {
				httpErrorCode(c, http.StatusUnauthorized, "invalid webhook token", "UNAUTHORIZED")
				return
			}
			httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_WEBHOOK_PAYLOAD")
			return
		}

		b, err := d.BookingSvc.Get(c.Request.Context(), payload.ExternalID)
		if err != nil {
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "failed to get booking: "+err.Error(), "INTERNAL_ERROR")
			return
		}

		switch payload.Status {
		case "PAID", "SETTLED":
			// Idempotent replay: jika sudah confirmed, langsung 200 OK tanpa error atau efek samping
			if b.Status == booking.StatusConfirmed {
				writeJSON(c, http.StatusOK, map[string]string{
					"status":  "ok",
					"message": "booking already confirmed (idempotent replay)",
				})
				return
			}

			// Validasi Amount (BE-R14: cegah underpayment)
			if payload.Amount != b.TotalPriceMinor {
				httpErrorCode(c, http.StatusUnprocessableEntity, fmt.Sprintf("payment amount mismatch: expected %d, got %d", b.TotalPriceMinor, payload.Amount), "PAYMENT_AMOUNT_MISMATCH")
				return
			}

			// Validasi Currency (BE-R14)
			if payload.Currency != "" && !strings.EqualFold(payload.Currency, b.Currency) {
				httpErrorCode(c, http.StatusUnprocessableEntity, fmt.Sprintf("payment currency mismatch: expected %s, got %s", b.Currency, payload.Currency), "PAYMENT_CURRENCY_MISMATCH")
				return
			}

			// Validasi Invoice ID terhadap buku besar PaymentAttempt (BE-R14)
			attempts, err := d.BookingSvc.GetPaymentAttempts(c.Request.Context(), payload.ExternalID)
			if err == nil && len(attempts) > 0 {
				var hasRef, matched bool
				for _, att := range attempts {
					if att.ProviderReference != "" {
						hasRef = true
						if att.ProviderReference == payload.ID {
							matched = true
							break
						}
					}
				}
				if hasRef && !matched {
					httpErrorCode(c, http.StatusUnprocessableEntity, "invoice ID does not match recorded payment attempt", "INVOICE_ID_MISMATCH")
					return
				}
			}

			if err := d.BookingSvc.Confirm(c.Request.Context(), payload.ExternalID); err != nil {
				if errors.Is(err, booking.ErrHoldExpired) {
					if d.FinanceSvc != nil {
						_, _ = d.FinanceSvc.CreateLatePaymentCase(c.Request.Context(), payload.ExternalID, payload.ID, payload.Amount, "Hold expired before payment arrived")
					}
					httpErrorCode(c, http.StatusConflict, "hold has expired, payment rejected", "HOLD_EXPIRED")
					return
				}
				httpErrorCode(c, http.StatusInternalServerError, "failed to confirm booking: "+err.Error(), "CONFIRM_FAILED")
				return
			}
			writeJSON(c, http.StatusOK, map[string]string{
				"status":  "ok",
				"message": "booking confirmed",
			})
		case "EXPIRED":
			// Proteksi out-of-order expiry (BE-R14): jangan batalkan booking yang sudah berstatus confirmed
			if b.Status == booking.StatusConfirmed {
				writeJSON(c, http.StatusOK, map[string]string{
					"status":  "ignored",
					"message": "booking already confirmed, stale expiry event ignored",
				})
				return
			}

			if err := d.BookingSvc.Cancel(c.Request.Context(), payload.ExternalID); err != nil {
				httpErrorCode(c, http.StatusInternalServerError, "failed to cancel booking: "+err.Error(), "CANCEL_FAILED")
				return
			}
			writeJSON(c, http.StatusOK, map[string]string{
				"status":  "ok",
				"message": "booking cancelled due to invoice expiry",
			})
		default:
			writeJSON(c, http.StatusOK, map[string]string{
				"status":  "ignored",
				"message": "unhandled status: " + payload.Status,
			})
		}
	}
}
