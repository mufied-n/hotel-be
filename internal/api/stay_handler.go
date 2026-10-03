package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/hotel-booking/internal/stay"
)

// handleRoomMove melayani POST /api/v1/bookings/{id}/room-move (FR-STAY-01).
// Memindahkan tamu yang sedang menginap (checked_in) ke kamar fisik baru.
func handleRoomMove(d Deps) http.HandlerFunc {
	type req struct {
		TargetRoomNumber string `json:"target_room_number"`
		ReasonCategory   string `json:"reason_category"`
		Notes            string `json:"notes"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if d.StaySvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(chi.URLParam(r, "id"))
		if bookingID == "" {
			httpErrorCode(w, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		authCtx := GetAuthContext(r.Context())
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

		res, err := d.StaySvc.MoveRoom(r.Context(), input)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, stay.ErrInvalidBookingStatus) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "INVALID_BOOKING_STATUS")
				return
			}
			if errors.Is(err, stay.ErrSameRoomMove) {
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_INPUT")
				return
			}
			if errors.Is(err, stay.ErrInvalidReasonCategory) {
				httpErrorCode(w, http.StatusBadRequest, "kategori alasan tidak sah (pilihan: maintenance_defect, noise_complaint, upgrade, guest_request)", "INVALID_REASON")
				return
			}
			if errors.Is(err, stay.ErrTargetRoomNotReady) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "TARGET_ROOM_NOT_READY")
				return
			}
			if errors.Is(err, stay.ErrRoomPhysicalOverlap) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "ROOM_PHYSICAL_OVERLAP")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal memindahkan kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, res)
	}
}

// handleExtendStay melayani POST /api/v1/bookings/{id}/extend-stay (FR-STAY-02).
// Memperpanjang masa menginap tamu dan memvalidasi ketersediaan inventaris.
func handleExtendStay(d Deps) http.HandlerFunc {
	type req struct {
		AdditionalNights int    `json:"additional_nights"`
		PaymentMethod    string `json:"payment_method"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if d.StaySvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(chi.URLParam(r, "id"))
		if bookingID == "" {
			httpErrorCode(w, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		authCtx := GetAuthContext(r.Context())
		actorID := authCtx.Subject

		input := stay.ExtendStayInput{
			BookingID:        bookingID,
			AdditionalNights: in.AdditionalNights,
			PaymentMethod:    in.PaymentMethod,
			ActorID:          actorID,
		}

		res, err := d.StaySvc.ExtendStay(r.Context(), input)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, stay.ErrInvalidAdditionalNights) {
				httpErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_ADDITIONAL_NIGHTS")
				return
			}
			if errors.Is(err, stay.ErrInvalidBookingStatus) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "INVALID_BOOKING_STATUS")
				return
			}
			if errors.Is(err, stay.ErrNoAvailabilityForExtension) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "NO_AVAILABILITY_FOR_EXTENSION")
				return
			}
			if errors.Is(err, stay.ErrRoomPhysicalOverlap) {
				httpErrorCode(w, http.StatusConflict, err.Error(), "ROOM_PHYSICAL_OVERLAP")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal memperpanjang masa menginap", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, res)
	}
}

// handleListRoomMoves melayani GET /api/v1/bookings/{id}/room-moves (FR-STAY-03).
// Mengambil jejak audit riwayat pemindahan kamar untuk sebuah reservasi.
func handleListRoomMoves(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.StaySvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "stay service not configured", "INTERNAL_ERROR")
			return
		}

		bookingID := strings.TrimSpace(chi.URLParam(r, "id"))
		if bookingID == "" {
			httpErrorCode(w, http.StatusBadRequest, "parameter 'id' booking wajib diisi", "INVALID_BOOKING_ID")
			return
		}

		moves, err := d.StaySvc.ListRoomMoves(r.Context(), bookingID)
		if err != nil {
			if errors.Is(err, stay.ErrBookingNotFound) {
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal memuat riwayat pemindahan kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"booking_id": bookingID,
			"moves":      moves,
		})
	}
}
