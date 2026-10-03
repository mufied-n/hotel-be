package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/hotel-booking/internal/assistance"
)

// POST /api/v1/guest/bookings/{id}/special-requests
func handleCreateGuestSpecialRequest(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AssistanceSvc == nil {
			httpErrorCode(w, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := GuestSessionFromContext(r.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			httpErrorCode(w, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := chi.URLParam(r, "id")
		var input assistance.CreateRequestInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}
		input.BookingID = bookingID

		created, err := d.AssistanceSvc.CreateGuestRequest(r.Context(), session.GuestEmail, input)
		if err != nil {
			switch {
			case errors.Is(err, assistance.ErrBookingNotFound):
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidCategory):
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_CATEGORY")
			case errors.Is(err, assistance.ErrEmptyDescription):
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "EMPTY_DESCRIPTION")
			default:
				httpErrorCode(w, http.StatusInternalServerError, "gagal membuat permintaan khusus", "INTERNAL_ERROR")
			}
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

// GET /api/v1/guest/bookings/{id}/special-requests
func handleListGuestSpecialRequests(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AssistanceSvc == nil {
			httpErrorCode(w, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := GuestSessionFromContext(r.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			httpErrorCode(w, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := chi.URLParam(r, "id")
		requests, err := d.AssistanceSvc.ListGuestRequests(r.Context(), session.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, assistance.ErrBookingNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal mengambil daftar permintaan khusus", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"booking_id": bookingID,
			"requests":   requests,
		})
	}
}

// GET /api/v1/front-desk/special-requests
func handleListStaffSpecialRequests(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AssistanceSvc == nil {
			httpErrorCode(w, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		filter := assistance.ListFilter{
			Department: r.URL.Query().Get("department"),
			Status:     r.URL.Query().Get("status"),
			BookingID:  r.URL.Query().Get("booking_id"),
		}

		items, err := d.AssistanceSvc.ListStaffQueue(r.Context(), filter)
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal mengambil antrean tugas staf", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"total": len(items),
		})
	}
}

// PUT /api/v1/front-desk/special-requests/{id}/status
func handleUpdateStaffSpecialRequestStatus(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AssistanceSvc == nil {
			httpErrorCode(w, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		id := chi.URLParam(r, "id")
		var body struct {
			ToStatus   assistance.Status `json:"to_status"`
			StaffNotes string            `json:"staff_notes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}

		authCtx := GetAuthContext(r.Context())
		actor := authCtx.Subject
		if actor == "" {
			actor = authCtx.Role
		}
		if actor == "" {
			actor = "staff"
		}

		input := assistance.UpdateStatusInput{
			RequestID:  id,
			ToStatus:   body.ToStatus,
			StaffNotes: body.StaffNotes,
			HandledBy:  actor,
		}

		updated, err := d.AssistanceSvc.UpdateStatus(r.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, assistance.ErrRequestNotFound):
				httpErrorCode(w, http.StatusNotFound, "permintaan khusus tidak ditemukan", "REQUEST_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidStatusTransition):
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_STATUS_TRANSITION")
			case errors.Is(err, assistance.ErrStaffNotesRequired):
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "STAFF_NOTES_REQUIRED")
			default:
				httpErrorCode(w, http.StatusInternalServerError, "gagal memperbarui status permintaan", "INTERNAL_ERROR")
			}
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}
