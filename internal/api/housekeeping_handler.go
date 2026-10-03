package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/example/hotel-booking/internal/housekeeping"
)

// handleHousekeepingRooms melayani GET /api/v1/housekeeping/rooms (FR-HK-01).
// Menampilkan daftar 95 kamar beserta ringkasan status kebersihan dan okupansi.
func handleHousekeepingRooms(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.HousekeepingSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "housekeeping service not configured", "INTERNAL_ERROR")
			return
		}

		q := r.URL.Query()
		floor := 0
		if floorStr := q.Get("floor"); floorStr != "" {
			if f, err := strconv.Atoi(floorStr); err == nil {
				floor = f
			}
		}

		status := strings.TrimSpace(q.Get("status"))
		// Normalisasi sinonim status
		switch status {
		case "dirty":
			status = string(housekeeping.StatusVacantDirty)
		case "clean":
			status = string(housekeeping.StatusVacantClean)
		}

		roomTypeID := strings.TrimSpace(q.Get("room_type_id"))

		summary, err := d.HousekeepingSvc.GetRoomBoard(r.Context(), floor, status, roomTypeID)
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memuat status operasional kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, summary)
	}
}

// handleHousekeepingStatus melayani PUT /api/v1/housekeeping/rooms/{id}/status (FR-HK-02).
// Memperbarui status kebersihan kamar dengan validasi transisi terpusat.
func handleHousekeepingStatus(d Deps) http.HandlerFunc {
	type req struct {
		ToStatus string `json:"to_status"`
		Notes    string `json:"notes"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if d.HousekeepingSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "housekeeping service not configured", "INTERNAL_ERROR")
			return
		}

		roomNumber := chi.URLParam(r, "id")
		if roomNumber == "" {
			httpErrorCode(w, http.StatusBadRequest, "nomor kamar wajib disertakan", "INVALID_ROOM_NUMBER")
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		toStatusStr := strings.TrimSpace(in.ToStatus)
		switch toStatusStr {
		case "dirty":
			toStatusStr = string(housekeeping.StatusVacantDirty)
		case "clean":
			toStatusStr = string(housekeeping.StatusVacantClean)
		}

		targetStatus := housekeeping.CleanlinessStatus(toStatusStr)
		authCtx := GetAuthContext(r.Context())

		input := housekeeping.UpdateStatusInput{
			RoomNumber: roomNumber,
			ToStatus:   targetStatus,
			Notes:      strings.TrimSpace(in.Notes),
			ActorID:    authCtx.Subject,
			ActorRole:  authCtx.Role,
		}

		err := d.HousekeepingSvc.UpdateStatus(r.Context(), input)
		switch {
		case errors.Is(err, housekeeping.ErrRoomNotFound):
			httpErrorCode(w, http.StatusNotFound, "kamar tidak ditemukan", "ROOM_NOT_FOUND")
			return
		case errors.Is(err, housekeeping.ErrUnauthorizedTransition):
			httpErrorCode(w, http.StatusForbidden, "hanya GM Admin yang berwenang mengubah kamar menjadi out_of_order", "UNAUTHORIZED_TRANSITION")
			return
		case errors.Is(err, housekeeping.ErrInvalidTransition):
			httpErrorCode(w, http.StatusConflict, err.Error(), "INVALID_STATUS_TRANSITION")
			return
		case err != nil:
			httpErrorCode(w, http.StatusInternalServerError, "gagal memperbarui status kebersihan kamar", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":             "ok",
			"room_number":        roomNumber,
			"cleanliness_status": targetStatus,
			"updated_at":         time.Now().UTC(),
		})
	}
}

// handleHousekeepingOOO melayani POST /api/v1/housekeeping/rooms/{id}/out-of-order (FR-HK-03).
// Menandai kamar rusak berat (out_of_order) dan memotong kuota inventaris web.
func handleHousekeepingOOO(d Deps) http.HandlerFunc {
	type req struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Reason    string `json:"reason"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if d.HousekeepingSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "housekeeping service not configured", "INTERNAL_ERROR")
			return
		}

		authCtx := GetAuthContext(r.Context())
		if authCtx.Role != "gm_admin" {
			httpErrorCode(w, http.StatusForbidden, "hanya GM Admin yang berwenang menetapkan status Out of Order", "UNAUTHORIZED_ACTION")
			return
		}

		roomNumber := chi.URLParam(r, "id")
		if roomNumber == "" {
			httpErrorCode(w, http.StatusBadRequest, "nomor kamar wajib disertakan", "INVALID_ROOM_NUMBER")
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		startDate, err1 := time.Parse("2006-01-02", in.StartDate)
		endDate, err2 := time.Parse("2006-01-02", in.EndDate)
		if err1 != nil || err2 != nil || !startDate.Before(endDate) {
			httpErrorCode(w, http.StatusBadRequest, "start_date dan end_date (YYYY-MM-DD, start_date < end_date) wajib valid", "INVALID_DATE_RANGE")
			return
		}

		err := d.HousekeepingSvc.MarkRoomOutOfOrder(r.Context(), roomNumber, startDate, endDate, in.Reason)
		switch {
		case errors.Is(err, housekeeping.ErrRoomNotFound):
			httpErrorCode(w, http.StatusNotFound, "kamar tidak ditemukan", "ROOM_NOT_FOUND")
			return
		case errors.Is(err, housekeeping.ErrInvalidTransition):
			httpErrorCode(w, http.StatusConflict, err.Error(), "INVALID_STATUS_TRANSITION")
			return
		case err != nil:
			httpErrorCode(w, http.StatusInternalServerError, "gagal menetapkan kamar out of order", "INTERNAL_ERROR")
			return
		}

		var dates []string
		for cur := startDate; cur.Before(endDate); cur = cur.AddDate(0, 0, 1) {
			dates = append(dates, cur.Format("2006-01-02"))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":                   "ok",
			"room_number":              roomNumber,
			"cleanliness_status":       housekeeping.StatusOutOfOrder,
			"inventory_deducted_dates": dates,
		})
	}
}
