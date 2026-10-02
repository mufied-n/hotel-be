// Package api menyediakan HTTP handler (transport layer / driving adapter
// dalam hexagonal — desain §8). Handler TIDAK berisi logika domain; ia
// menerjemahkan HTTP ⇄ use case.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
)

// Deps adalah dependensi transport layer — semuanya interface domain.
type Deps struct {
	BookingSvc *booking.Service
	InvStore   inventory.AvailabilityStore
	RateSvc    rates.RateProvider
	Enqueuer   *workers.Enqueuer
	ReadyCheck func(ctx context.Context) error
	// FakePay memicu konfirmasi pembayaran pada mode dev (FakeGateway).
	FakePay func(w http.ResponseWriter, r *http.Request)
	// Enforcer untuk evaluasi RBAC Casbin thread-safe.
	Enforcer *casbin.SyncedEnforcer
}

// NewRouter merakit seluruh route.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", healthz)
	r.Get("/ready", ready(d))

	// API routes dengan identifikasi subjek dan proteksi RBAC Casbin
	r.Group(func(api chi.Router) {
		api.Use(IdentifySubject())
		if d.Enforcer != nil {
			api.Use(Authorize(d.Enforcer))
		}

		api.Get("/api/v1/availability", getAvailability(d))
		api.Post("/api/v1/bookings", createBooking(d))
		api.Get("/api/v1/bookings/{id}", getBooking(d))
		api.Post("/api/v1/bookings/{id}/cancel", cancelBooking(d))
		api.Post("/api/v1/bookings/{id}/check-in", checkIn(d))
		api.Post("/api/v1/bookings/{id}/check-out", checkOut(d))
		api.Post("/api/v1/bookings/{id}/no-show", noShow(d))

		// Dev-only: simulasi pembayaran sukses → memicu path webhook yang sama
		// dengan gateway nyata (idempotent — §12.4).
		api.Post("/fake-pay/{ref}", d.FakePay)
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

// GET /api/v1/availability?room_type_id=...&check_in=YYYY-MM-DD&check_out=YYYY-MM-DD
func getAvailability(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		roomTypeID := q.Get("room_type_id")
		from, err1 := parseDate(q.Get("check_in"))
		to, err2 := parseDate(q.Get("check_out"))
		if roomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			httpError(w, http.StatusBadRequest, "room_type_id, check_in, check_out (YYYY-MM-DD, check_in < check_out) wajib")
			return
		}
		avail, err := d.InvStore.GetByDate(r.Context(), roomTypeID, from, to)
		if errors.Is(err, inventory.ErrNotFound) {
			httpError(w, http.StatusNotFound, "inventory tidak ditemukan untuk rentang tsb")
			return
		}
		if err != nil {
			httpError(w, http.StatusInternalServerError, "gagal membaca availability")
			return
		}
		quotes, err := d.RateSvc.Quote(r.Context(), roomTypeID, from, to)
		if err != nil {
			httpError(w, http.StatusNotFound, "tipe kamar tidak dikenal")
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
			httpError(w, http.StatusBadRequest, "body JSON tidak valid")
			return
		}
		from, err1 := parseDate(in.CheckIn)
		to, err2 := parseDate(in.CheckOut)
		if err1 != nil || err2 != nil {
			httpError(w, http.StatusBadRequest, "check_in/check_out wajib format YYYY-MM-DD")
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
		if errors.Is(err, booking.ErrInvalidDateRange) || errors.Is(err, booking.ErrInvalidCapacity) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, inventory.ErrInsufficient) || errors.Is(err, booking.ErrInsufficient) {
			httpError(w, http.StatusConflict, "kamar tidak tersedia untuk rentang tsb")
			return
		}
		if errors.Is(err, inventory.ErrNotFound) {
			httpError(w, http.StatusNotFound, "inventory tidak ditemukan")
			return
		}
		if err != nil {
			httpError(w, http.StatusInternalServerError, "gagal membuat booking")
			return
		}
		// Jadwalkan release-hold otomatis t+holdTimeout jika enqueuer aktif (asynq scheduled task).
		if d.Enqueuer != nil {
			_ = d.Enqueuer.EnqueueReleaseHold(r.Context(), b.ID, d.BookingSvc.HoldTimeout())
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"booking":     b,
			"payment_url": charge.PaymentURL,
			"reference":   charge.Reference,
		})
	}
}

// GET /api/v1/bookings/{id}
func getBooking(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := d.BookingSvc.Get(r.Context(), chi.URLParam(r, "id"))
		if errors.Is(err, booking.ErrNotFound) {
			httpError(w, http.StatusNotFound, "booking tidak ditemukan")
			return
		}
		if err != nil {
			httpError(w, http.StatusInternalServerError, "gagal membaca booking")
			return
		}
		writeJSON(w, http.StatusOK, b)
	}
}

// POST /api/v1/bookings/{id}/cancel
func cancelBooking(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := d.BookingSvc.Cancel(r.Context(), id); err != nil {
			if errors.Is(err, booking.ErrIllegalTransition) {
				httpError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, booking.ErrNotFound) {
				httpError(w, http.StatusNotFound, "booking tidak ditemukan")
				return
			}
			httpError(w, http.StatusInternalServerError, "gagal membatalkan booking")
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

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
