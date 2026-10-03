package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/frontdesk"
)

// handleFrontDeskDailyRoster melayani GET /api/v1/front-desk/daily-roster (FR-FD-01).
// Menampilkan ringkasan metrik harian, daftar arrivals, departures, dan tamu in-house.
func handleFrontDeskDailyRoster(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
			return
		}

		targetDate := time.Now().UTC()
		if dateStr := strings.TrimSpace(r.URL.Query().Get("date")); dateStr != "" {
			parsed, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				httpErrorCode(w, http.StatusBadRequest, "parameter 'date' harus berformat YYYY-MM-DD", "INVALID_DATE_FORMAT")
				return
			}
			targetDate = parsed
		}

		roster, err := d.FrontDeskSvc.GetDailyRoster(r.Context(), targetDate)
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memuat data roster meja depan", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, roster)
	}
}

// handleFrontDeskRecordHandover melayani POST /api/v1/front-desk/handover-notes (FR-FD-03).
// Mencatat serah terima shift antar-regu meja depan.
func handleFrontDeskRecordHandover(d Deps) http.HandlerFunc {
	type req struct {
		Shift          string `json:"shift"`
		CashFloatMinor int64  `json:"cash_float_minor"`
		PendingIssues  string `json:"pending_issues"`
		VIPGuestNotes  string `json:"vip_guest_notes"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
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

		input := frontdesk.RecordHandoverInput{
			Shift:          frontdesk.ShiftType(strings.ToLower(strings.TrimSpace(in.Shift))),
			CashFloatMinor: in.CashFloatMinor,
			PendingIssues:  strings.TrimSpace(in.PendingIssues),
			VIPGuestNotes:  strings.TrimSpace(in.VIPGuestNotes),
			ActorID:        actorID,
			ActorRole:      actorRole,
		}

		note, err := d.FrontDeskSvc.RecordHandover(r.Context(), input)
		if err != nil {
			if errors.Is(err, frontdesk.ErrInvalidShift) {
				httpErrorCode(w, http.StatusBadRequest, "shift wajib berupa 'morning', 'afternoon', atau 'night'", "INVALID_SHIFT")
				return
			}
			if errors.Is(err, frontdesk.ErrEmptyHandoverNote) {
				httpErrorCode(w, http.StatusBadRequest, "catatan serah terima shift tidak boleh kosong", "INVALID_INPUT")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal mencatat serah terima shift", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"status": "ok",
			"note":   note,
		})
	}
}

// handleFrontDeskListHandovers melayani GET /api/v1/front-desk/handover-notes (FR-FD-03).
// Mengambil riwayat catatan serah terima shift dengan pagination.
func handleFrontDeskListHandovers(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(w, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
			return
		}

		q := r.URL.Query()
		limit := 20
		if lStr := q.Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		offset := 0
		if oStr := q.Get("offset"); oStr != "" {
			if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
				offset = o
			}
		}

		notes, total, err := d.FrontDeskSvc.ListHandovers(r.Context(), limit, offset)
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memuat riwayat serah terima shift", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total": total,
			"notes": notes,
		})
	}
}
