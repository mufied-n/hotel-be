package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

// GuestBookings mengambil daftar riwayat booking milik tamu (GET /api/v1/guest/bookings).
func GuestBookings(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan.")
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan portal tamu belum dikonfigurasi.")
			return
		}

		status := c.Query("status")
		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		bookings, err := d.GuestSvc.ListBookings(c.Request.Context(), sess.GuestEmail, status, limit)
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memuat daftar pemesanan.")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"data":  bookings,
			"total": len(bookings),
		})
	}
}

// GuestBookingDetail membaca rincian reservasi tamu (GET /api/v1/guest/bookings/:id).
func GuestBookingDetail(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan.")
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan portal tamu belum dikonfigurasi.")
			return
		}

		id := c.Param("id")
		detail, err := d.GuestSvc.GetBookingDetail(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.WriteGuestError(c, http.StatusNotFound, "BOOKING_NOT_FOUND", "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memuat detail pemesanan.")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"booking":         detail,
			"allowed_actions": detail.AllowedActions,
		})
	}
}

// GuestBookingPayment memulihkan link pembayaran reservasi tamu (GET /api/v1/guest/bookings/:id/payment).
func GuestBookingPayment(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan.")
			return
		}

		if d.BookingSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan pemesanan belum dikonfigurasi.")
			return
		}

		id := c.Param("id")
		b, err := d.BookingSvc.Get(c.Request.Context(), id)
		if err != nil || !strings.EqualFold(strings.TrimSpace(sess.GuestEmail), strings.TrimSpace(b.GuestEmail)) {
			middleware.WriteGuestError(c, http.StatusNotFound, "BOOKING_NOT_FOUND", "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.")
			return
		}

		recovery, err := d.BookingSvc.GetPaymentRecovery(c.Request.Context(), id)
		if errors.Is(err, booking.ErrHoldExpired) {
			middleware.WriteGuestError(c, http.StatusGone, "HOLD_EXPIRED", "Batas waktu pembayaran reservasi telah kedaluwarsa, kamar telah dilepas ke publik.")
			return
		}
		if errors.Is(err, booking.ErrPaymentRecoveryNotPending) {
			middleware.WriteGuestError(c, http.StatusConflict, "BOOKING_NOT_PENDING", "Pembayaran tidak dapat dilanjutkan karena reservasi tidak berstatus pending.")
			return
		}
		if errors.Is(err, booking.ErrPaymentGatewayTimeout) {
			middleware.WriteGuestError(c, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "Koneksi gateway pembayaran terputus, silakan coba beberapa saat lagi.")
			return
		}
		if errors.Is(err, booking.ErrPaymentDefinitiveFailure) {
			middleware.WriteGuestError(c, http.StatusBadGateway, "PAYMENT_FAILED", "Gateway pembayaran menolak pembuatan tagihan.")
			return
		}
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memulihkan tautan pembayaran.")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, recovery)
	}
}

// GuestBookingReceipt mengunduh invoice bukti reservasi (GET /api/v1/guest/bookings/:id/receipt).
func GuestBookingReceipt(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan.")
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan portal tamu belum dikonfigurasi.")
			return
		}

		id := c.Param("id")
		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.WriteGuestError(c, http.StatusNotFound, "BOOKING_NOT_FOUND", "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.")
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				middleware.WriteGuestError(c, http.StatusBadRequest, "RECEIPT_NOT_AVAILABLE", "Invoice resmi dan bukti reservasi hanya tersedia setelah pembayaran dikonfirmasi.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memuat invoice bukti pemesanan.")
			return
		}

		c.Header("Cache-Control", "no-store, private")
		middleware.WriteJSON(c, http.StatusOK, receipt)
	}
}

// GuestBookingCalendar mengunduh file kalender .ics (GET /api/v1/guest/bookings/:id/calendar.ics).
func GuestBookingCalendar(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan.")
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan portal tamu belum dikonfigurasi.")
			return
		}

		id := c.Param("id")
		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), sess.GuestEmail, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.WriteGuestError(c, http.StatusNotFound, "BOOKING_NOT_FOUND", "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini.")
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				middleware.WriteGuestError(c, http.StatusBadRequest, "RECEIPT_NOT_AVAILABLE", "File kalender hanya tersedia setelah pembayaran dikonfirmasi.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memproses file kalender.")
			return
		}

		icsBytes, err := d.GuestSvc.GenerateCalendarICS(receipt)
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal menghasilkan file kalender.")
			return
		}

		c.Header("Content-Type", "text/calendar; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"pulang-booking-%s.ics\"", id))
		c.Header("Cache-Control", "no-store, private")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(icsBytes)
	}
}
