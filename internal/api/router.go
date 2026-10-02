// Package api menyediakan HTTP handler (transport layer / driving adapter
// dalam hexagonal — desain §8). Handler TIDAK berisi logika domain; ia
// menerjemahkan HTTP ⇄ use case.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
)

// Deps adalah dependensi transport layer — semuanya interface domain.
type Deps struct {
	BookingSvc    *booking.Service
	InvStore      inventory.AvailabilityStore
	RateSvc       rates.RateProvider
	CatalogStore  catalog.Store
	Enqueuer      *workers.Enqueuer
	ReadyCheck    func(ctx context.Context) error
	// FakePay memicu konfirmasi pembayaran pada mode dev (FakeGateway).
	FakePay       func(w http.ResponseWriter, r *http.Request)
	// Enforcer untuk evaluasi RBAC Casbin thread-safe.
	Enforcer      *casbin.SyncedEnforcer
	IsDevelopment bool
	RateLimiter   *RateLimiter
}

// NewRouter merakit seluruh route.
func NewRouter(d Deps) http.Handler {
	if d.CatalogStore == nil {
		d.CatalogStore = catalog.NewMemoryStore(catalog.DefaultVariants())
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	if d.RateLimiter != nil {
		r.Use(d.RateLimiter.Limit())
	}

	r.Get("/healthz", healthz)
	r.Get("/ready", ready(d))

	// API routes dengan identifikasi subjek dan proteksi RBAC Casbin (fail-closed: BE-G14)
	r.Group(func(api chi.Router) {
		api.Use(IdentifySubject())
		api.Use(Authorize(d.Enforcer))

		api.Get("/api/v1/catalog/rooms", getCatalogRooms(d))
		api.Get("/api/v1/catalog/rooms/{id}", getCatalogRoom(d))
		api.Post("/api/v1/catalog/rooms", createCatalogRoom(d))
		api.Put("/api/v1/catalog/rooms/{id}", updateCatalogRoom(d))
		api.Delete("/api/v1/catalog/rooms/{id}", deleteCatalogRoom(d))
		api.Get("/api/v1/search", searchRooms(d))
		api.Get("/api/v1/availability", getAvailability(d))
		api.Post("/api/v1/bookings", createBooking(d))
		api.Get("/api/v1/bookings/{id}", getBooking(d))
		api.Post("/api/v1/bookings/{id}/cancel", cancelBooking(d))
		api.Post("/api/v1/bookings/{id}/check-in", checkIn(d))
		api.Post("/api/v1/bookings/{id}/check-out", checkOut(d))
		api.Post("/api/v1/bookings/{id}/no-show", noShow(d))

		// Dev-only: simulasi pembayaran sukses (BE-G10: gate development only)
		if d.IsDevelopment && d.FakePay != nil {
			api.Post("/fake-pay/{ref}", d.FakePay)
		}
	})

	return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func ready(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.ReadyCheck != nil {
			if err := d.ReadyCheck(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status": "unavailable",
					"error":  err.Error(),
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// GET /api/v1/catalog/rooms (BE-G01)
func getCatalogRooms(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		variants, err := d.CatalogStore.ListVariants(r.Context())
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca katalog kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total": len(variants),
			"rooms": variants,
		})
	}
}

// GET /api/v1/catalog/rooms/{id}
func getCatalogRoom(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		v, err := d.CatalogStore.GetVariant(r.Context(), id)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(w, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

// POST /api/v1/catalog/rooms
func createCatalogRoom(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var v catalog.RoomVariant
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_ROOM_PAYLOAD")
			return
		}
		created, err := d.CatalogStore.CreateVariant(r.Context(), v)
		if errors.Is(err, catalog.ErrInvalidVariant) {
			httpErrorCode(w, http.StatusBadRequest, "kode, nama, kapasitas, dan harga dasar wajib diisi", "INVALID_ROOM_DATA")
			return
		}
		if errors.Is(err, catalog.ErrDuplicateCode) {
			httpErrorCode(w, http.StatusConflict, "kode varian kamar sudah digunakan", "CONFLICT_ROOM_CODE")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membuat varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

// PUT /api/v1/catalog/rooms/{id}
func updateCatalogRoom(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var v catalog.RoomVariant
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_ROOM_PAYLOAD")
			return
		}
		updated, err := d.CatalogStore.UpdateVariant(r.Context(), id, v)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(w, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if errors.Is(err, catalog.ErrInvalidVariant) {
			httpErrorCode(w, http.StatusBadRequest, "kode, nama, kapasitas, dan harga dasar wajib diisi", "INVALID_ROOM_DATA")
			return
		}
		if errors.Is(err, catalog.ErrDuplicateCode) {
			httpErrorCode(w, http.StatusConflict, "kode varian kamar sudah digunakan", "CONFLICT_ROOM_CODE")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memperbarui varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// DELETE /api/v1/catalog/rooms/{id}
func deleteCatalogRoom(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		err := d.CatalogStore.DeleteVariant(r.Context(), id)
		if errors.Is(err, catalog.ErrVariantNotFound) {
			httpErrorCode(w, http.StatusNotFound, "varian kamar tidak ditemukan", "ROOM_VARIANT_NOT_FOUND")
			return
		}
		if errors.Is(err, catalog.ErrCannotDelete) {
			httpErrorCode(w, http.StatusConflict, "tidak dapat menghapus varian yang masih digunakan dalam inventaris atau booking", "CANNOT_DELETE_ACTIVE_VARIANT")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal menghapus varian kamar", "CATALOG_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
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
func searchRooms(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		checkInStr := q.Get("check_in")
		checkOutStr := q.Get("check_out")

		from, err1 := parseDate(checkInStr)
		to, err2 := parseDate(checkOutStr)
		if checkInStr == "" || checkOutStr == "" || err1 != nil || err2 != nil {
			httpErrorCode(w, http.StatusBadRequest, "check_in and check_out (YYYY-MM-DD) are required", "INVALID_DATE_FORMAT")
			return
		}
		if !from.Before(to) {
			httpErrorCode(w, http.StatusBadRequest, "check_out must be after check_in", "INVALID_DATE_RANGE")
			return
		}

		nights := int(to.Sub(from).Hours() / 24)
		if nights > 30 {
			httpErrorCode(w, http.StatusBadRequest, "stay duration cannot exceed 30 nights", "EXCEEDS_MAX_LOS")
			return
		}

		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		checkInDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		if checkInDate.Before(today.Add(-24 * time.Hour)) {
			httpErrorCode(w, http.StatusBadRequest, "check_in date cannot be in the past", "PAST_DATE")
			return
		}
		if to.After(now.AddDate(0, 0, 366)) {
			httpErrorCode(w, http.StatusBadRequest, "search dates cannot exceed 365 days booking horizon", "EXCEEDS_HORIZON")
			return
		}

		adults := 1
		if s := q.Get("adults"); s != "" {
			var err error
			adults, err = strconv.Atoi(s)
			if err != nil || adults < 1 {
				httpErrorCode(w, http.StatusBadRequest, "adults must be at least 1", "INVALID_GUEST_COUNT")
				return
			}
		}

		rooms := 1
		if s := q.Get("rooms"); s != "" {
			var err error
			rooms, err = strconv.Atoi(s)
			if err != nil || rooms < 1 || rooms > 8 {
				httpErrorCode(w, http.StatusBadRequest, "rooms must be between 1 and 8", "INVALID_ROOM_COUNT")
				return
			}
		}

		children := 0
		if s := q.Get("children"); s != "" {
			var err error
			children, err = strconv.Atoi(s)
			if err != nil || children < 0 {
				httpErrorCode(w, http.StatusBadRequest, "children cannot be negative", "INVALID_GUEST_COUNT")
				return
			}
		}

		if childAgesStr := q.Get("child_ages"); childAgesStr != "" {
			for _, ageStr := range strings.Split(childAgesStr, ",") {
				age, err := strconv.Atoi(strings.TrimSpace(ageStr))
				if err != nil || age < 0 || age > 17 {
					httpErrorCode(w, http.StatusBadRequest, "child age must be between 0 and 17", "INVALID_CHILD_AGE")
					return
				}
			}
		}

		variants, err := d.CatalogStore.ListVariants(r.Context())
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
			return
		}

		var results []SearchResultItem
		availableCount := 0

		for _, v := range variants {
			item := SearchResultItem{
				RoomVariant: v,
				Currency:    "IDR",
			}

			// Kapasitas okupansi (BE-G03):
			// - Tamu per kamar tidak boleh melebihi max_capacity
			// - Dewasa per kamar tidak boleh melebihi max_adults
			// - Minimal 1 dewasa per kamar yang dipesan
			totalGuests := adults + children
			if totalGuests > v.MaxCapacity*rooms || adults > v.MaxAdults*rooms || adults < rooms {
				item.Available = false
				item.UnavailableReason = "EXCEEDS_CAPACITY"
				results = append(results, item)
				continue
			}

			// Periksa ketersediaan multi-malam kontinu (BE-G02)
			avail, err := d.InvStore.GetByDate(r.Context(), v.ID, from, to)
			if errors.Is(err, inventory.ErrNotFound) {
				item.Available = false
				item.UnavailableReason = "MISSING_INVENTORY"
				results = append(results, item)
				continue
			}
			if err != nil {
				httpErrorCode(w, http.StatusInternalServerError, "gagal memeriksa ketersediaan kamar", "INVENTORY_ERROR")
				return
			}
			if len(avail) < nights {
				item.Available = false
				item.UnavailableReason = "MISSING_INVENTORY"
				results = append(results, item)
				continue
			}

			// Hitung kuotasi tarif
			quotes, err := d.RateSvc.Quote(r.Context(), v.ID, from, to)
			if err == nil {
				item.Quotes = quotes
				item.TotalPriceMinor = sumQuotes(quotes) * int64(rooms)
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

		writeJSON(w, http.StatusOK, map[string]any{
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
func getAvailability(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		roomTypeID := q.Get("room_type_id")
		from, err1 := parseDate(q.Get("check_in"))
		to, err2 := parseDate(q.Get("check_out"))
		if roomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			httpErrorCode(w, http.StatusBadRequest, "room_type_id, check_in, check_out (YYYY-MM-DD, check_in < check_out) wajib", "INVALID_QUERY")
			return
		}
		avail, err := d.InvStore.GetByDate(r.Context(), roomTypeID, from, to)
		if errors.Is(err, inventory.ErrNotFound) {
			httpErrorCode(w, http.StatusNotFound, "inventory tidak ditemukan untuk rentang tsb", "INVENTORY_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca availability", "INTERNAL_ERROR")
			return
		}
		quotes, err := d.RateSvc.Quote(r.Context(), roomTypeID, from, to)
		if err != nil {
			httpErrorCode(w, http.StatusNotFound, "tipe kamar tidak dikenal", "UNKNOWN_ROOM_TYPE")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"availability": avail,
			"quotes":       quotes,
			"total_minor":  sumQuotes(quotes),
		})
	}
}

// POST /api/v1/bookings
func createBooking(d Deps) http.HandlerFunc {
	type req struct {
		RoomTypeID string `json:"room_type_id"`
		CheckIn    string `json:"check_in"`
		CheckOut   string `json:"check_out"`
		NumRooms   int    `json:"num_rooms"`
		NumGuests  int    `json:"num_guests"`
		GuestName  string `json:"guest_name"`
		GuestEmail string `json:"guest_email"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}
		from, err1 := parseDate(in.CheckIn)
		to, err2 := parseDate(in.CheckOut)
		if err1 != nil || err2 != nil {
			httpErrorCode(w, http.StatusBadRequest, "check_in/check_out wajib format YYYY-MM-DD", "INVALID_DATE_FORMAT")
			return
		}
		b, charge, err := d.BookingSvc.Create(r.Context(), booking.CreateInput{
			RoomTypeID: in.RoomTypeID,
			CheckIn:    from,
			CheckOut:   to,
			NumRooms:   in.NumRooms,
			NumGuests:  in.NumGuests,
			GuestName:  in.GuestName,
			GuestEmail: in.GuestEmail,
		})
		if errors.Is(err, booking.ErrInvalidDateRange) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_DATE_RANGE")
			return
		}
		if errors.Is(err, booking.ErrInvalidCapacity) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_CAPACITY")
			return
		}
		if errors.Is(err, booking.ErrExceedsMaxStay) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "EXCEEDS_MAX_LOS")
			return
		}
		if errors.Is(err, booking.ErrPastDate) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "PAST_DATE")
			return
		}
		if errors.Is(err, booking.ErrExceedsHorizon) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "EXCEEDS_HORIZON")
			return
		}
		if errors.Is(err, booking.ErrInvalidGuestInfo) {
			httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_GUEST_INFO")
			return
		}
		if errors.Is(err, inventory.ErrInsufficient) || errors.Is(err, booking.ErrInsufficient) {
			httpErrorCode(w, http.StatusConflict, "kamar tidak tersedia untuk rentang tsb", "INSUFFICIENT_ROOMS")
			return
		}
		if errors.Is(err, inventory.ErrNotFound) {
			httpErrorCode(w, http.StatusNotFound, "inventory tidak ditemukan", "INVENTORY_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membuat booking", "INTERNAL_ERROR")
			return
		}
		// Jadwalkan release-hold otomatis t+holdTimeout jika enqueuer aktif (asynq scheduled task).
		if d.Enqueuer != nil {
			_ = d.Enqueuer.EnqueueReleaseHold(r.Context(), b.ID, d.BookingSvc.HoldTimeout())
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"booking":            b,
			"guest_access_token": b.GuestToken,
			"payment_url":        charge.PaymentURL,
			"reference":          charge.Reference,
		})
	}
}

// GET /api/v1/bookings/{id}
func getBooking(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := d.BookingSvc.Get(r.Context(), chi.URLParam(r, "id"))
		if errors.Is(err, booking.ErrNotFound) {
			httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca booking", "INTERNAL_ERROR")
			return
		}

		authCtx := GetAuthContext(r.Context())
		guestToken := GetGuestToken(r.Context())

		// BE-G13: Privasi data tamu (PII).
		// Jika caller adalah staff (bukan guest) ATAU memiliki guest_token yang valid,
		// kembalikan data booking lengkap.
		// Jika guest tanpa token yang cocok, kembalikan PublicDTO yang dimasking.
		if authCtx.Role != "guest" || (b.GuestToken != "" && guestToken == b.GuestToken) {
			writeJSON(w, http.StatusOK, b)
			return
		}

		writeJSON(w, http.StatusOK, b.ToPublicDTO())
	}
}

// POST /api/v1/bookings/{id}/cancel
func cancelBooking(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		authCtx := GetAuthContext(r.Context())
		guestToken := GetGuestToken(r.Context())

		// BE-G13: Guest hanya diizinkan membatalkan jika memiliki guest_token yang cocok
		if authCtx.Role == "guest" {
			b, err := d.BookingSvc.Get(r.Context(), id)
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if err != nil {
				httpErrorCode(w, http.StatusInternalServerError, "gagal membaca booking", "INTERNAL_ERROR")
				return
			}
			if b.GuestToken == "" || guestToken != b.GuestToken {
				httpErrorCode(w, http.StatusForbidden, "guest token tidak valid atau tidak memiliki akses pembatalan", "FORBIDDEN_OWNERSHIP")
				return
			}
		}

		if err := d.BookingSvc.Cancel(r.Context(), id); err != nil {
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "ILLEGAL_TRANSITION")
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal membatalkan booking", "INTERNAL_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "id": id})
	}
}

// POST /api/v1/bookings/{id}/check-in
// confirmed → checked_in + room assignment (dijamin GiST di database).
func checkIn(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := d.BookingSvc.CheckIn(r.Context(), chi.URLParam(r, "id"))
		switch {
		case errors.Is(err, booking.ErrNotFound):
			httpError(w, http.StatusNotFound, "booking tidak ditemukan")
			return
		case errors.Is(err, booking.ErrIllegalTransition):
			httpError(w, http.StatusConflict, err.Error())
			return
		case errors.Is(err, booking.ErrNoRoomAvailable):
			httpError(w, http.StatusConflict, "tidak ada kamar fisik bebas untuk rentang menginap ini")
			return
		case err != nil:
			httpError(w, http.StatusInternalServerError, "gagal check-in")
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// POST /api/v1/bookings/{id}/check-out
func checkOut(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := d.BookingSvc.CheckOut(r.Context(), id); err != nil {
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpError(w, http.StatusNotFound, "booking tidak ditemukan")
				return
			}
			httpError(w, http.StatusInternalServerError, "gagal check-out")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "checked_out", "id": id})
	}
}

// POST /api/v1/bookings/{id}/no-show
func noShow(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := d.BookingSvc.MarkNoShow(r.Context(), id); err != nil {
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpError(w, http.StatusNotFound, "booking tidak ditemukan")
				return
			}
			httpError(w, http.StatusInternalServerError, "gagal memproses no-show")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "no_show", "id": id})
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpErrorCode(w http.ResponseWriter, code int, msg, errCode string) {
	title := http.StatusText(code)
	writeProblemDetails(w, code, title, msg, errCode)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	httpErrorCode(w, code, msg, "ERROR")
}
