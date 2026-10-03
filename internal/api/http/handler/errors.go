package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

// errRule memetakan satu error domain ke respons HTTP. msg == "" berarti memakai err.Error().
type errRule struct {
	is     error
	status int
	code   string
	msg    string
}

// writeDomainError menulis respons error pertama yang cocok pada rules (urutan = prioritas).
func writeDomainError(c *gin.Context, err error, rules []errRule, fb errRule) {
	for _, r := range rules {
		if errors.Is(err, r.is) {
			msg := r.msg
			if msg == "" {
				msg = err.Error()
			}
			middleware.HttpErrorCode(c, r.status, msg, r.code)
			return
		}
	}
	msg := fb.msg
	if msg == "" {
		msg = err.Error()
	}
	if fb.status >= http.StatusInternalServerError {
		slog.ErrorContext(c.Request.Context(), "http.unmapped_error",
			"route", c.FullPath(), "code", fb.code, "error", err)
	}
	middleware.HttpErrorCode(c, fb.status, msg, fb.code)
}

var (
	ruleBookingNotFound   = errRule{booking.ErrNotFound, http.StatusNotFound, "BOOKING_NOT_FOUND", "booking tidak ditemukan"}
	ruleIllegalTransition = errRule{booking.ErrIllegalTransition, http.StatusConflict, "ILLEGAL_TRANSITION", ""}
)

// ---- Booking ----

var (
	getBookingErrors      = []errRule{ruleBookingNotFound}
	getBookingFallback    = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal membaca booking"}
	bookingPaymentLookup  = []errRule{{booking.ErrNotFound, http.StatusNotFound, "BOOKING_NOT_FOUND", "booking tidak ditemukan atau Anda tidak memiliki akses"}}
	paymentRecoveryErrors = []errRule{
		ruleBookingNotFound,
		{booking.ErrHoldExpired, http.StatusGone, "HOLD_EXPIRED", "batas waktu pembayaran reservasi telah kedaluwarsa, kamar telah dilepas ke publik"},
		{booking.ErrPaymentRecoveryNotPending, http.StatusConflict, "BOOKING_NOT_PENDING", "pembayaran tidak dapat dilanjutkan karena reservasi tidak berstatus pending"},
		{booking.ErrPaymentGatewayTimeout, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "koneksi gateway pembayaran terputus, silakan coba beberapa saat lagi"},
		{booking.ErrPaymentDefinitiveFailure, http.StatusBadGateway, "PAYMENT_FAILED", "gateway pembayaran menolak pembuatan tagihan"},
	}
	paymentRecoveryFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal memulihkan tautan pembayaran"}

	createBookingErrors = []errRule{
		{booking.ErrExceedsCapacity, http.StatusBadRequest, "EXCEEDS_CAPACITY", ""},
		{booking.ErrConsentRequired, http.StatusBadRequest, "CONSENT_REQUIRED", ""},
		{booking.ErrQuoteRequired, http.StatusBadRequest, "QUOTE_REQUIRED", ""},
		{booking.ErrQuoteExpired, http.StatusGone, "QUOTE_EXPIRED", ""},
		{booking.ErrQuoteAlreadyUsed, http.StatusConflict, "QUOTE_ALREADY_USED", ""},
		{booking.ErrQuoteMismatch, http.StatusBadRequest, "QUOTE_MISMATCH", ""},
		{booking.ErrInvalidPhone, http.StatusBadRequest, "INVALID_PHONE", ""},
		{booking.ErrInvalidArrivalTime, http.StatusBadRequest, "INVALID_ARRIVAL_TIME", ""},
		{booking.ErrSpecialRequestTooLong, http.StatusBadRequest, "SPECIAL_REQUEST_TOO_LONG", ""},
		{booking.ErrInvalidDateRange, http.StatusBadRequest, "INVALID_DATE_RANGE", ""},
		{booking.ErrInvalidCapacity, http.StatusBadRequest, "INVALID_CAPACITY", ""},
		{booking.ErrExceedsMaxStay, http.StatusBadRequest, "EXCEEDS_MAX_LOS", ""},
		{booking.ErrPastDate, http.StatusBadRequest, "PAST_DATE", ""},
		{booking.ErrExceedsHorizon, http.StatusBadRequest, "EXCEEDS_HORIZON", ""},
		{booking.ErrInvalidGuestInfo, http.StatusBadRequest, "INVALID_GUEST_INFO", ""},
		{inventory.ErrInsufficient, http.StatusConflict, "INSUFFICIENT_ROOMS", "kamar tidak tersedia untuk rentang tsb"},
		{booking.ErrInsufficient, http.StatusConflict, "INSUFFICIENT_ROOMS", "kamar tidak tersedia untuk rentang tsb"},
		{inventory.ErrNotFound, http.StatusNotFound, "INVENTORY_NOT_FOUND", "inventory tidak ditemukan"},
		{booking.ErrPaymentGatewayTimeout, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "koneksi gateway pembayaran terputus, reservasi tetap tersimpan dalam antrean pemulihan"},
		{booking.ErrPaymentDefinitiveFailure, http.StatusBadGateway, "PAYMENT_FAILED", "gateway pembayaran menolak transaksi"},
	}
	createBookingFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal membuat booking"}

	cancelBookingErrors   = []errRule{ruleIllegalTransition, ruleBookingNotFound}
	cancelBookingFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal membatalkan booking"}

	checkInErrors = []errRule{
		ruleBookingNotFound,
		ruleIllegalTransition,
		{booking.ErrRoomNotReady, http.StatusConflict, "ROOM_NOT_READY", "kamar belum siap huni (belum diinspeksi oleh housekeeping)"},
		{booking.ErrNoRoomAvailable, http.StatusConflict, "NO_ROOM_AVAILABLE", "tidak ada kamar fisik bebas untuk rentang menginap ini"},
	}
	checkInFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal check-in"}

	checkOutErrors   = []errRule{ruleIllegalTransition, ruleBookingNotFound}
	checkOutFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal check-out"}

	noShowErrors = []errRule{
		{booking.ErrNoShowTooEarly, http.StatusBadRequest, "NO_SHOW_TOO_EARLY", "reservasi belum mencapai tanggal check-in untuk ditandai no-show"},
		ruleIllegalTransition,
		ruleBookingNotFound,
	}
	noShowFallback = errRule{status: http.StatusInternalServerError, code: "INTERNAL_ERROR", msg: "gagal memproses no-show"}
)

// ---- Katalog ----

var (
	ruleVariantNotFound = errRule{catalog.ErrVariantNotFound, http.StatusNotFound, "ROOM_VARIANT_NOT_FOUND", "varian kamar tidak ditemukan"}
	ruleVariantInvalid  = errRule{catalog.ErrInvalidVariant, http.StatusBadRequest, "INVALID_ROOM_DATA", "kode, nama, kapasitas, dan harga dasar wajib diisi"}
	ruleVariantDup      = errRule{catalog.ErrDuplicateCode, http.StatusConflict, "CONFLICT_ROOM_CODE", "kode varian kamar sudah digunakan"}

	getCatalogRoomErrors    = []errRule{ruleVariantNotFound}
	getCatalogRoomFallback  = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal membaca varian kamar"}
	listCatalogFallback     = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal membaca katalog kamar"}
	createCatalogErrors     = []errRule{ruleVariantInvalid, ruleVariantDup}
	createCatalogFallback   = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal membuat varian kamar"}
	updateCatalogErrors     = []errRule{ruleVariantNotFound, ruleVariantInvalid, ruleVariantDup}
	updateCatalogFallback   = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal memperbarui varian kamar"}
	deleteCatalogErrors     = []errRule{ruleVariantNotFound, {catalog.ErrCannotDelete, http.StatusConflict, "CANNOT_DELETE_ACTIVE_VARIANT", "tidak dapat menghapus varian yang masih digunakan dalam inventaris atau booking"}}
	deleteCatalogFallback   = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal menghapus varian kamar"}
	quoteVariantLookupError = []errRule{{catalog.ErrVariantNotFound, http.StatusNotFound, "ROOM_NOT_FOUND", "tipe kamar tidak ditemukan"}}
	quoteVariantFallback    = errRule{status: http.StatusInternalServerError, code: "CATALOG_ERROR", msg: "gagal membaca varian kamar"}
)

// ---- Quote ----

var (
	calculateQuoteErrors = []errRule{
		{rates.ErrInvalidRatePlan, http.StatusBadRequest, "INVALID_RATE_PLAN", "kode rate plan tidak valid"},
		{rates.ErrInvalidPromoCode, http.StatusBadRequest, "INVALID_PROMO_CODE", "kode promo tidak valid atau kedaluwarsa"},
		{rates.ErrUnknownRoomType, http.StatusNotFound, "ROOM_NOT_FOUND", "tipe kamar tidak ditemukan"},
		{rates.ErrUnpricedRoomType, http.StatusBadRequest, "RATE_UNAVAILABLE", "tarif dasar kamar belum dikonfigurasi"},
		{rates.ErrSaveQuoteFailed, http.StatusInternalServerError, "INTERNAL_ERROR", "gagal menyimpan kuotasi harga"},
	}
	calculateQuoteFallback = errRule{status: http.StatusBadRequest, code: "BAD_REQUEST"}
)
