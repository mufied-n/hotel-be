package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/gin-gonic/gin"
)

// handleFrontDeskDailyRoster melayani GET /api/v1/front-desk/daily-roster (FR-FD-01).
// Menampilkan ringkasan metrik harian, daftar arrivals, departures, dan tamu in-house.
func handleFrontDeskDailyRoster(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
			return
		}

		targetDate := time.Now().UTC()
		if dateStr := strings.TrimSpace(c.Query("date")); dateStr != "" {
			parsed, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				httpErrorCode(c, http.StatusBadRequest, "parameter 'date' harus berformat YYYY-MM-DD", "INVALID_DATE_FORMAT")
				return
			}
			targetDate = parsed
		}

		roster, err := d.FrontDeskSvc.GetDailyRoster(c.Request.Context(), targetDate)
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal memuat data roster meja depan", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, roster)
	}
}

// handleFrontDeskRecordHandover melayani POST /api/v1/front-desk/handover-notes (FR-FD-03).
// Mencatat serah terima shift antar-regu meja depan.
func handleFrontDeskRecordHandover(d Deps) gin.HandlerFunc {
	type req struct {
		Shift          string `json:"shift" validate:"required"`
		CashFloatMinor int64  `json:"cash_float_minor"`
		PendingIssues  string `json:"pending_issues"`
		VIPGuestNotes  string `json:"vip_guest_notes"`
	}

	return func(c *gin.Context) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
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

		input := frontdesk.RecordHandoverInput{
			Shift:          frontdesk.ShiftType(strings.ToLower(strings.TrimSpace(in.Shift))),
			CashFloatMinor: in.CashFloatMinor,
			PendingIssues:  strings.TrimSpace(in.PendingIssues),
			VIPGuestNotes:  strings.TrimSpace(in.VIPGuestNotes),
			ActorID:        actorID,
			ActorRole:      actorRole,
		}

		note, err := d.FrontDeskSvc.RecordHandover(c.Request.Context(), input)
		if err != nil {
			if errors.Is(err, frontdesk.ErrInvalidShift) {
				httpErrorCode(c, http.StatusBadRequest, "shift wajib berupa 'morning', 'afternoon', atau 'night'", "INVALID_SHIFT")
				return
			}
			if errors.Is(err, frontdesk.ErrEmptyHandoverNote) {
				httpErrorCode(c, http.StatusBadRequest, "catatan serah terima shift tidak boleh kosong", "INVALID_INPUT")
				return
			}
			httpErrorCode(c, http.StatusInternalServerError, "gagal mencatat serah terima shift", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusCreated, map[string]any{
			"status": "ok",
			"note":   note,
		})
	}
}

// handleFrontDeskListHandovers melayani GET /api/v1/front-desk/handover-notes (FR-FD-03).
// Mengambil riwayat catatan serah terima shift dengan pagination.
func handleFrontDeskListHandovers(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FrontDeskSvc == nil {
			httpErrorCode(c, http.StatusInternalServerError, "frontdesk service not configured", "INTERNAL_ERROR")
			return
		}

		limit := 20
		if lStr := c.Query("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		offset := 0
		if oStr := c.Query("offset"); oStr != "" {
			if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
				offset = o
			}
		}

		notes, total, err := d.FrontDeskSvc.ListHandovers(c.Request.Context(), limit, offset)
		if err != nil {
			httpErrorCode(c, http.StatusInternalServerError, "gagal memuat riwayat serah terima shift", "INTERNAL_ERROR")
			return
		}

		writeJSON(c, http.StatusOK, map[string]any{
			"total": total,
			"notes": notes,
		})
	}
}
