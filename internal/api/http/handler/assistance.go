package handler

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/assistance"
	"github.com/gin-gonic/gin"
)

// CreateGuestSpecialRequest menangani permintaan khusus tamu untuk booking tertentu (POST /api/v1/guest/bookings/:id/special-requests).
func CreateGuestSpecialRequest(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := middleware.GuestSessionFromContext(c.Request.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			middleware.HttpErrorCode(c, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := c.Param("id")
		var input assistance.CreateRequestInput
		if err := json.UnmarshalRead(c.Request.Body, &input); err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}
		input.BookingID = bookingID

		created, err := d.AssistanceSvc.CreateGuestRequest(c.Request.Context(), session.GuestEmail, input)
		if err != nil {
			switch {
			case errors.Is(err, assistance.ErrBookingNotFound):
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidCategory):
				middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_CATEGORY")
			case errors.Is(err, assistance.ErrEmptyDescription):
				middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "EMPTY_DESCRIPTION")
			default:
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal membuat permintaan khusus", "INTERNAL_ERROR")
			}
			return
		}

		middleware.WriteJSON(c, http.StatusCreated, created)
	}
}

// ListGuestSpecialRequests mengembalikan daftar permintaan khusus milik tamu untuk booking tertentu (GET /api/v1/guest/bookings/:id/special-requests).
func ListGuestSpecialRequests(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		session := middleware.GuestSessionFromContext(c.Request.Context())
		if session == nil || strings.TrimSpace(session.GuestEmail) == "" {
			middleware.HttpErrorCode(c, http.StatusUnauthorized, "sesi tamu tidak valid", "UNAUTHORIZED")
			return
		}

		bookingID := c.Param("id")
		requests, err := d.AssistanceSvc.ListGuestRequests(c.Request.Context(), session.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, assistance.ErrBookingNotFound) {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal mengambil daftar permintaan khusus", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"booking_id": bookingID,
			"requests":   requests,
		})
	}
}

// ListStaffSpecialRequests mengembalikan antrean permintaan khusus untuk staf hotel (GET /api/v1/front-desk/special-requests).
func ListStaffSpecialRequests(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		filter := assistance.ListFilter{
			Department: c.Query("department"),
			Status:     c.Query("status"),
			BookingID:  c.Query("booking_id"),
		}

		items, err := d.AssistanceSvc.ListStaffQueue(c.Request.Context(), filter)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal mengambil antrean tugas staf", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"items": items,
			"total": len(items),
		})
	}
}

// UpdateStaffSpecialRequestStatus memperbarui status permintaan khusus oleh staf (PUT /api/v1/front-desk/special-requests/:id/status).
func UpdateStaffSpecialRequestStatus(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.AssistanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusServiceUnavailable, "layanan permintaan khusus belum tersedia", "SERVICE_UNAVAILABLE")
			return
		}

		id := c.Param("id")
		var body struct {
			ToStatus   assistance.Status `json:"to_status"`
			StaffNotes string            `json:"staff_notes"`
		}

		if err := json.UnmarshalRead(c.Request.Body, &body); err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "format json tidak valid", "INVALID_JSON")
			return
		}

		authCtx := middleware.GetAuthContext(c.Request.Context())
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
				middleware.HttpErrorCode(c, http.StatusNotFound, "permintaan khusus tidak ditemukan", "REQUEST_NOT_FOUND")
			case errors.Is(err, assistance.ErrInvalidStatusTransition):
				middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_STATUS_TRANSITION")
			case errors.Is(err, assistance.ErrStaffNotesRequired):
				middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "STAFF_NOTES_REQUIRED")
			default:
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memperbarui status permintaan", "INTERNAL_ERROR")
			}
			return
		}

		middleware.WriteJSON(c, http.StatusOK, updated)
	}
}
