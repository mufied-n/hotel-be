package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/assistance"
)

// POST /api/v1/guest/bookings/:id/special-requests
func handleCreateGuestSpecialRequest(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := GuestSessionFromContext(c.Request.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			httpErrorCode(c, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := c.Param("id")
		var input assistance.CreateRequestInput
		if err := json.UnmarshalRead(c.Request.Body, &input); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}
		input.BookingID = bookingID

		created, err := d.AssistanceSvc.CreateGuestRequest(c.Request.Context(), session.GuestEmail, input)
		if err != nil {
			switch {
			case errors.Is(err, assistance.ErrBookingNotFound):
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidCategory):
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_CATEGORY")
			case errors.Is(err, assistance.ErrEmptyDescription):
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "EMPTY_DESCRIPTION")
			default:
				httpErrorCode(c, http.StatusInternalServerError, "gagal membuat permintaan khusus", "INTERNAL_ERROR")
			}
			return
		}

		writeJSON(c, http.StatusCreated, created)
	}
}

// GET /api/v1/guest/bookings/:id/special-requests
func handleListGuestSpecialRequests(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := GuestSessionFromContext(c.Request.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			httpErrorCode(c, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := c.Param("id")
		requests, err := d.AssistanceSvc.ListGuestRequests(c.Request.Context(), session.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, assistance.ErrBookingNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal mengambil daftar permintaan khusus", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"booking_id": bookingID,
			"requests":   requests,
		})
	}
}

// GET /api/v1/front-desk/special-requests
func handleListStaffSpecialRequests(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		filter := assistance.ListFilter{
			Department: c.Query("department"),
			Status:     c.Query("status"),
			BookingID:  c.Query("booking_id"),
		}

		items, err := d.AssistanceSvc.ListStaffQueue(c.Request.Context(), filter)
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal mengambil antrean tugas staf", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"items": items,
			"total": len(items),
		})
	}
}

// PUT /api/v1/front-desk/special-requests/:id/status
func handleUpdateStaffSpecialRequestStatus(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			httpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		id := c.Param("id")
		var body struct {
			ToStatus   assistance.Status `json:"to_status"`
			StaffNotes string            `json:"staff_notes"`
		}

		if err := json.UnmarshalRead(c.Request.Body, &body); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
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

		updated, err := d.AssistanceSvc.UpdateStatus(c.Request.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, assistance.ErrRequestNotFound):
				httpErrorCode(c, http.StatusNotFound, "permintaan khusus tidak ditemukan", "REQUEST_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidStatusTransition):
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_STATUS_TRANSITION")
			case errors.Is(err, assistance.ErrStaffNotesRequired):
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "STAFF_NOTES_REQUIRED")
			default:
				httpErrorCode(c, http.StatusInternalServerError, "gagal memperbarui status permintaan", "INTERNAL_ERROR")
			}
			return
		}

		writeJSON(c, http.StatusOK, updated)
	}
}
