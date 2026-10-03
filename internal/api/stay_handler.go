package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/stay"
	"github.com/gin-gonic/gin"
)

// handleRoomMove melayani POST /api/v1/bookings/{id}/room-move (FR-STAY-01).
// Memindahkan tamu yang sedang menginap (checked_in) ke kamar fisik baru.
func handleRoomMove(d Deps) gin.HandlerFunc {
	type req struct {
		TargetRoomNumber string `json:"target_room_number" validate:"required"`
		ReasonCategory   string `json:"reason_category" validate:"required"`
		Notes            string `json:"notes"`
	}

	return func(c *gin.Context) {
		if d.StaySvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(c.Param("id"))
		if bookingID == "" {
			httpErrorCode(c, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		var in req
		if err := json.UnmarshalRead(c.Request.Body, &in); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		if !validateDTO(c, &in) {
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
		actorID := authCtx.Subject
		actorRole := authCtx.Role

		input := stay.RoomMoveInput{
			BookingID:        bookingID,
			TargetRoomNumber: strings.TrimSpace(in.TargetRoomNumber),
			ReasonCategory:   stay.ReasonCategory(strings.TrimSpace(in.ReasonCategory)),
			Notes:            strings.TrimSpace(in.Notes),
			ActorID:          actorID,
			ActorRole:        actorRole,
		}

		res, err := d.StaySvc.MoveRoom(c.Request.Context(), input)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, stay.ErrInvalidBookingStatus) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "INVALID_BOOKING_STATUS")
				return
			}
			if errors.Is(err, stay.ErrSameRoomMove) {
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_INPUT")
				return
			}
			if errors.Is(err, stay.ErrInvalidReasonCategory) {
				httpErrorCode(c, http.StatusBadRequest, "kategori alasan tidak sah (pilihan: maintenance_defect, noise_complaint, upgrade, guest_request)", "INVALID_REASON")
				return
			}
			if errors.Is(err, stay.ErrTargetRoomNotReady) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "TARGET_ROOM_NOT_READY")
				return
			}
			if errors.Is(err, stay.ErrRoomPhysicalOverlap) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "ROOM_PHYSICAL_OVERLAP")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal memindahkan kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, res)
	}
}

// handleExtendStay melayani POST /api/v1/bookings/{id}/extend-stay (FR-STAY-02).
// Memperpanjang masa menginap tamu dan memvalidasi ketersediaan inventaris.
func handleExtendStay(d Deps) gin.HandlerFunc {
	type req struct {
		AdditionalNights int    `json:"additional_nights"`
		PaymentMethod    string `json:"payment_method"`
	}

	return func(c *gin.Context) {
		if d.StaySvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(c.Param("id"))
		if bookingID == "" {
			httpErrorCode(c, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		var in req
		if err := json.UnmarshalRead(c.Request.Body, &in); err != nil {
			httpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		authCtx := GetAuthContext(c.Request.Context())
		actorID := authCtx.Subject

		input := stay.ExtendStayInput{
			BookingID:        bookingID,
			AdditionalNights: in.AdditionalNights,
			PaymentMethod:    in.PaymentMethod,
			ActorID:          actorID,
		}

		res, err := d.StaySvc.ExtendStay(c.Request.Context(), input)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, stay.ErrInvalidAdditionalNights) {
				httpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_ADDITIONAL_NIGHTS")
				return
			}
			if errors.Is(err, stay.ErrInvalidBookingStatus) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "INVALID_BOOKING_STATUS")
				return
			}
			if errors.Is(err, stay.ErrNoAvailabilityForExtension) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "NO_AVAILABILITY_FOR_EXTENSION")
				return
			}
			if errors.Is(err, stay.ErrRoomPhysicalOverlap) {
				httpErrorCode(c, http.StatusConflict, err.Error(), "ROOM_PHYSICAL_OVERLAP")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal memperpanjang masa menginap", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, res)
	}
}

// handleListRoomMoves melayani GET /api/v1/bookings/{id}/room-moves (FR-STAY-03).
// Mengambil jejak audit riwayat pemindahan kamar untuk sebuah reservasi.
func handleListRoomMoves(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.StaySvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(c.Param("id"))
		if bookingID == "" {
			httpErrorCode(c, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		moves, err := d.StaySvc.ListRoomMoves(c.Request.Context(), bookingID)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal memuat riwayat pemindahan kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"booking_id": bookingID,
			"moves":      moves,
		})
	}
}
