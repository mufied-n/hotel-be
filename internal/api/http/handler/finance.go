package handler

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/gin-gonic/gin"
)

type financeRefundRequest struct {
	BookingID   string `json:"booking_id" validate:"required"`
	AmountMinor int64  `json:"amount_minor" validate:"required,gt=0"`
	Reason      string `json:"reason" validate:"required,min=5"`
}

type resolveCaseRequest struct {
	Action string `json:"action" validate:"required"`
	Notes  string `json:"notes"`
}

// FinanceRefund memproses inisiasi dan eksekusi pengembalian dana melalui payment gateway (POST /api/v1/finance/refunds).
func FinanceRefund(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FinanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		var req financeRefundRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "format payload json tidak valid", "BAD_REQUEST")
			return
		}

		if !validateDTO(c, &req) {
			return
		}

		auth := middleware.GetAuthContext(c.Request.Context())
		actorID := auth.Subject
		if actorID == "" || actorID == "anonymous" {
			actorID = "finance_user"
		}

		input := finance.CreateRefundInput{
			BookingID:   req.BookingID,
			AmountMinor: req.AmountMinor,
			Reason:      req.Reason,
			ActorID:     actorID,
			ActorRole:   auth.Role,
		}

		refund, err := d.FinanceSvc.ProcessRefund(c.Request.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, finance.ErrBookingNotFound):
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			case errors.Is(err, finance.ErrInvalidAmount):
				middleware.HttpErrorCode(c, http.StatusBadRequest, "nominal refund harus lebih dari 0", "INVALID_AMOUNT")
				return
			case errors.Is(err, finance.ErrReasonRequired):
				middleware.HttpErrorCode(c, http.StatusBadRequest, "alasan refund wajib diisi minimal 5 karakter", "REASON_REQUIRED")
				return
			case errors.Is(err, finance.ErrBookingNotPaid):
				middleware.HttpErrorCode(c, http.StatusConflict, "booking belum dibayar lunas atau sudah kadaluwarsa", "BOOKING_NOT_PAID")
				return
			case errors.Is(err, finance.ErrOverRefund):
				middleware.HttpErrorCode(c, http.StatusConflict, err.Error(), "OVER_REFUND_EXCEEDED")
				return
			case errors.Is(err, finance.ErrGatewayFailed):
				middleware.HttpErrorCode(c, http.StatusBadGateway, "kegagalan gateway pembayaran: "+err.Error(), "GATEWAY_REFUND_FAILED")
				return
			default:
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memproses refund: "+err.Error(), "INTERNAL_ERROR")
				return
			}
		}

		middleware.WriteJSON(c, http.StatusCreated, map[string]any{
			"status": "success",
			"refund": refund,
		})
	}
}

// FinanceCases mengambil daftar kasus sengketa pembayaran (GET /api/v1/finance/cases).
func FinanceCases(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FinanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		status := strings.TrimSpace(c.Query("status"))
		limit := 50
		if rawLimit := c.Query("limit"); rawLimit != "" {
			if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		cases, err := d.FinanceSvc.ListCases(c.Request.Context(), status, limit)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal membaca daftar kasus pembayaran", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"total": len(cases),
			"cases": cases,
		})
	}
}

// FinanceResolveCase menyelesaikan kasus sengketa pembayaran (POST /api/v1/finance/cases/{id}/resolve).
func FinanceResolveCase(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FinanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		caseID := c.Param("id")
		if strings.TrimSpace(caseID) == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "case id wajib diisi", "BAD_REQUEST")
			return
		}

		var req resolveCaseRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "format payload json tidak valid", "BAD_REQUEST")
			return
		}

		if !validateDTO(c, &req) {
			return
		}

		auth := middleware.GetAuthContext(c.Request.Context())
		actorID := auth.Subject
		if actorID == "" || actorID == "anonymous" {
			actorID = "finance_user"
		}

		input := finance.ResolveCaseInput{
			CaseID:  caseID,
			Action:  req.Action,
			Notes:   req.Notes,
			ActorID: actorID,
		}

		if err := d.FinanceSvc.ResolveCase(c.Request.Context(), input); err != nil {
			switch {
			case errors.Is(err, finance.ErrCaseNotFound):
				middleware.HttpErrorCode(c, http.StatusNotFound, "kasus pembayaran tidak ditemukan", "CASE_NOT_FOUND")
				return
			case errors.Is(err, finance.ErrCaseAlreadyResolved):
				middleware.HttpErrorCode(c, http.StatusConflict, "kasus pembayaran sudah selesai diselesaikan sebelumnya", "CASE_ALREADY_RESOLVED")
				return
			default:
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal menyelesaikan kasus pembayaran: "+err.Error(), "INTERNAL_ERROR")
				return
			}
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "kasus pembayaran berhasil diselesaikan",
		})
	}
}

// FinanceSummary mengambil ringkasan rekonsiliasi kas (GET /api/v1/finance/reconciliations).
func FinanceSummary(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FinanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		summary, err := d.FinanceSvc.GetReconciliationSummary(c.Request.Context())
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memuat ringkasan rekonsiliasi", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, summary)
	}
}

// GuestRefundStatus mengambil status refund milik tamu (GET /api/v1/guest/bookings/{id}/refund-status).
func GuestRefundStatus(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.HttpErrorCode(c, http.StatusUnauthorized, "sesi tamu tidak ditemukan", "UNAUTHORIZED")
			return
		}

		if d.FinanceSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		bookingID := c.Param("id")
		if strings.TrimSpace(bookingID) == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "id booking wajib diisi", "BAD_REQUEST")
			return
		}

		statusView, err := d.FinanceSvc.GetBookingRefundStatus(c.Request.Context(), sess.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, finance.ErrBookingNotFound) {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal membaca status refund tamu", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, statusView)
	}
}
