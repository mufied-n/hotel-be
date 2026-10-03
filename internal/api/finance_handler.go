package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/hotel-booking/internal/finance"
	"github.com/go-chi/chi/v5"
)

type financeRefundRequest struct {
	BookingID   string `json:"booking_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
}

type resolveCaseRequest struct {
	Action string `json:"action"`
	Notes  string `json:"notes"`
}

// POST /api/v1/finance/refunds
// Memproses inisiasi dan eksekusi pengembalian dana (refund) melalui payment gateway secara idempoten.
func handleFinanceRefund(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FinanceSvc == nil {
			httpErrorCode(w, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		var req financeRefundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "format payload json tidak valid", "BAD_REQUEST")
			return
		}

		auth := GetAuthContext(r.Context())
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

		refund, err := d.FinanceSvc.ProcessRefund(r.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, finance.ErrBookingNotFound):
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			case errors.Is(err, finance.ErrInvalidAmount):
				httpErrorCode(w, http.StatusBadRequest, "nominal refund harus lebih dari 0", "INVALID_AMOUNT")
				return
			case errors.Is(err, finance.ErrReasonRequired):
				httpErrorCode(w, http.StatusBadRequest, "alasan refund wajib diisi minimal 5 karakter", "REASON_REQUIRED")
				return
			case errors.Is(err, finance.ErrBookingNotPaid):
				httpErrorCode(w, http.StatusConflict, "booking belum dibayar lunas atau sudah kadaluwarsa", "BOOKING_NOT_PAID")
				return
			case errors.Is(err, finance.ErrOverRefund):
				httpErrorCode(w, http.StatusConflict, err.Error(), "OVER_REFUND_EXCEEDED")
				return
			case errors.Is(err, finance.ErrGatewayFailed):
				httpErrorCode(w, http.StatusBadGateway, "kegagalan gateway pembayaran: "+err.Error(), "GATEWAY_REFUND_FAILED")
				return
			default:
				httpErrorCode(w, http.StatusInternalServerError, "gagal memproses refund: "+err.Error(), "INTERNAL_ERROR")
				return
			}
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"status": "success",
			"refund": refund,
		})
	}
}

// GET /api/v1/finance/cases
// Mengambil daftar kasus anomali / sengketa pembayaran (misal late payment).
func handleFinanceCases(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FinanceSvc == nil {
			httpErrorCode(w, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		status := strings.TrimSpace(r.URL.Query().Get("status"))
		limit := 50
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		cases, err := d.FinanceSvc.ListCases(r.Context(), status, limit)
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca daftar kasus pembayaran", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total": len(cases),
			"cases": cases,
		})
	}
}

// POST /api/v1/finance/cases/{id}/resolve
// Menyelesaikan sengketa pembayaran dengan tindakan yang disetujui (refund, alokasi kamar, dsb).
func handleFinanceResolveCase(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FinanceSvc == nil {
			httpErrorCode(w, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		caseID := chi.URLParam(r, "id")
		if strings.TrimSpace(caseID) == "" {
			httpErrorCode(w, http.StatusBadRequest, "case id wajib diisi", "BAD_REQUEST")
			return
		}

		var req resolveCaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "format payload json tidak valid", "BAD_REQUEST")
			return
		}

		if strings.TrimSpace(req.Action) == "" {
			httpErrorCode(w, http.StatusBadRequest, "tindakan resolusi (action) wajib diisi", "ACTION_REQUIRED")
			return
		}

		auth := GetAuthContext(r.Context())
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

		if err := d.FinanceSvc.ResolveCase(r.Context(), input); err != nil {
			switch {
			case errors.Is(err, finance.ErrCaseNotFound):
				httpErrorCode(w, http.StatusNotFound, "kasus pembayaran tidak ditemukan", "CASE_NOT_FOUND")
				return
			case errors.Is(err, finance.ErrCaseAlreadyResolved):
				httpErrorCode(w, http.StatusConflict, "kasus pembayaran sudah selesai diselesaikan sebelumnya", "CASE_ALREADY_RESOLVED")
				return
			default:
				httpErrorCode(w, http.StatusInternalServerError, "gagal menyelesaikan kasus pembayaran: "+err.Error(), "INTERNAL_ERROR")
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "kasus pembayaran berhasil diselesaikan",
		})
	}
}

// GET /api/v1/finance/reconciliations
// Mengambil ringkasan data rekonsiliasi kas hotel.
func handleFinanceSummary(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FinanceSvc == nil {
			httpErrorCode(w, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		summary, err := d.FinanceSvc.GetReconciliationSummary(r.Context())
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memuat ringkasan rekonsiliasi", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, summary)
	}
}

// GET /api/v1/guest/bookings/{id}/refund-status
// Mengambil status pengembalian dana khusus tamu pemilik reservasi (UU PDP No. 27/2022).
func handleGuestRefundStatus(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := GuestSessionFromContext(r.Context())
		if sess == nil {
			httpErrorCode(w, http.StatusUnauthorized, "sesi tamu tidak ditemukan", "UNAUTHORIZED")
			return
		}

		if d.FinanceSvc == nil {
			httpErrorCode(w, http.StatusNotImplemented, "layanan finansial belum dikonfigurasi", "FINANCE_NOT_CONFIGURED")
			return
		}

		bookingID := chi.URLParam(r, "id")
		if strings.TrimSpace(bookingID) == "" {
			httpErrorCode(w, http.StatusBadRequest, "id booking wajib diisi", "BAD_REQUEST")
			return
		}

		statusView, err := d.FinanceSvc.GetBookingRefundStatus(r.Context(), sess.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, finance.ErrBookingNotFound) {
				// Anti-IDOR: sembunyikan apakah booking tidak ada atau bukan miliknya
				httpErrorCode(w, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			httpErrorCode(w, http.StatusInternalServerError, "gagal membaca status refund tamu", "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, statusView)
	}
}
