package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/adapter/docgen"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/gin-gonic/gin"
)

// GuestBookingVoucherPDF mengunduh Confirmation Voucher berformat PDF resmi (GET /api/v1/guest/bookings/:id/voucher.pdf).
func GuestBookingVoucherPDF(d Deps) gin.HandlerFunc {
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
				middleware.WriteGuestError(c, http.StatusBadRequest, "RECEIPT_NOT_AVAILABLE", "Confirmation voucher hanya tersedia setelah pembayaran dikonfirmasi.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memuat berkas voucher pemesanan.")
			return
		}

		pdfBytes, err := buildVoucherPDFBytes(receipt)
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal menghasilkan berkas PDF voucher.")
			return
		}

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"voucher-%s.pdf\"", receipt.BookingReference))
		c.Header("Cache-Control", "no-store, private")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
	}
}

// GuestBookingInvoicePDF mengunduh Faktur Pajak Daerah Resmi (PBJT Kabupaten Sleman 10%) (GET /api/v1/guest/bookings/:id/invoice.pdf).
func GuestBookingInvoicePDF(d Deps) gin.HandlerFunc {
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
				middleware.WriteGuestError(c, http.StatusBadRequest, "RECEIPT_NOT_AVAILABLE", "Faktur pajak daerah PBJT hanya tersedia setelah pembayaran dikonfirmasi.")
				return
			}
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memuat berkas faktur pajak pemesanan.")
			return
		}

		pdfBytes, err := buildInvoicePDFBytes(receipt)
		if err != nil {
			middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal menghasilkan berkas PDF faktur pajak.")
			return
		}

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"faktur-pbjt-%s.pdf\"", receipt.BookingReference))
		c.Header("Cache-Control", "no-store, private")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
	}
}

// GetBookingVoucherPDF mengunduh voucher konfirmasi bagi staf hotel atau pemesan via token (GET /api/v1/bookings/:id/voucher.pdf).
func GetBookingVoucherPDF(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil || d.BookingSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan reservasi belum dikonfigurasi", "NOT_IMPLEMENTED")
			return
		}

		id := c.Param("id")
		authCtx := middleware.GetAuthContext(c.Request.Context())
		guestToken := middleware.GetGuestToken(c.Request.Context())

		email := ""
		if authCtx.Role == "guest" {
			b, err := d.BookingSvc.Get(c.Request.Context(), id)
			if err != nil {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			authorized := false
			if b.GuestToken != "" && guestToken == b.GuestToken {
				authorized = true
				email = b.GuestEmail
			} else {
				sessionToken := middleware.ExtractGuestSessionToken(c.Request)
				if sessionToken != "" {
					sess, sErr := d.GuestSvc.ValidateSession(c.Request.Context(), sessionToken)
					if sErr == nil && sess != nil && strings.EqualFold(strings.TrimSpace(sess.GuestEmail), strings.TrimSpace(b.GuestEmail)) {
						authorized = true
						email = b.GuestEmail
					}
				}
			}
			if !authorized {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan atau Anda tidak memiliki akses", "BOOKING_NOT_FOUND")
				return
			}
		}

		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), email, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "voucher konfirmasi hanya tersedia setelah pembayaran dikonfirmasi", "RECEIPT_NOT_AVAILABLE")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memuat voucher", "INTERNAL_ERROR")
			return
		}

		pdfBytes, err := buildVoucherPDFBytes(receipt)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal merender berkas voucher PDF", "INTERNAL_ERROR")
			return
		}

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"voucher-%s.pdf\"", receipt.BookingReference))
		c.Header("Cache-Control", "no-store, private")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
	}
}

// GetBookingInvoicePDF mengunduh faktur pajak daerah PBJT bagi staf finance/admin atau pemesan (GET /api/v1/bookings/:id/invoice.pdf).
func GetBookingInvoicePDF(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil || d.BookingSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan reservasi belum dikonfigurasi", "NOT_IMPLEMENTED")
			return
		}

		id := c.Param("id")
		authCtx := middleware.GetAuthContext(c.Request.Context())
		guestToken := middleware.GetGuestToken(c.Request.Context())

		email := ""
		if authCtx.Role == "guest" {
			b, err := d.BookingSvc.Get(c.Request.Context(), id)
			if err != nil {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			authorized := false
			if b.GuestToken != "" && guestToken == b.GuestToken {
				authorized = true
				email = b.GuestEmail
			} else {
				sessionToken := middleware.ExtractGuestSessionToken(c.Request)
				if sessionToken != "" {
					sess, sErr := d.GuestSvc.ValidateSession(c.Request.Context(), sessionToken)
					if sErr == nil && sess != nil && strings.EqualFold(strings.TrimSpace(sess.GuestEmail), strings.TrimSpace(b.GuestEmail)) {
						authorized = true
						email = b.GuestEmail
					}
				}
			}
			if !authorized {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan atau Anda tidak memiliki akses", "BOOKING_NOT_FOUND")
				return
			}
		}

		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), email, id)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
				return
			}
			if errors.Is(err, guest.ErrReceiptNotAvailable) {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "faktur pajak daerah PBJT hanya tersedia setelah pembayaran dikonfirmasi", "RECEIPT_NOT_AVAILABLE")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memuat faktur", "INTERNAL_ERROR")
			return
		}

		pdfBytes, err := buildInvoicePDFBytes(receipt)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal merender berkas faktur PDF", "INTERNAL_ERROR")
			return
		}

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"faktur-pbjt-%s.pdf\"", receipt.BookingReference))
		c.Header("Cache-Control", "no-store, private")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
	}
}

// VerifyVoucher memvalidasi QR signature voucher untuk check-in instan front desk (GET /api/v1/front-desk/verify-voucher).
func VerifyVoucher(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.GuestSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan portal reservasi belum dikonfigurasi", "NOT_IMPLEMENTED")
			return
		}

		ref := strings.TrimSpace(c.Query("ref"))
		token := strings.TrimSpace(c.Query("token"))
		bookingID := strings.TrimSpace(c.Query("id"))

		if ref == "" || token == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "parameter ref dan token wajib diisi", "MISSING_PARAMETERS")
			return
		}

		lookupID := bookingID
		if lookupID == "" {
			lookupID = ref
		}

		receipt, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), "", lookupID)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusNotFound, "reservasi voucher tidak ditemukan", "BOOKING_NOT_FOUND")
			return
		}

		// Validasi tanda tangan kriptografis HMAC
		valid := docgen.VerifyVoucherToken("", receipt.BookingReference, receipt.BookingID, receipt.StayDetails.CheckInDate, token)
		if !valid {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "tanda tangan digital voucher tidak sah atau telah dimodifikasi", "INVALID_QR_SIGNATURE")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, gin.H{
			"valid":               true,
			"verification_status": "SIGNATURE_VERIFIED",
			"booking_id":          receipt.BookingID,
			"reference":           receipt.BookingReference,
			"guest_name":          receipt.GuestDetails.Name,
			"guest_email":         receipt.GuestDetails.Email,
			"guest_phone":         receipt.GuestDetails.Phone,
			"room_type_name":      receipt.RoomItem.RoomTypeName,
			"check_in":            receipt.StayDetails.CheckInDate,
			"check_out":           receipt.StayDetails.CheckOutDate,
			"num_rooms":           receipt.RoomItem.NumRooms,
			"num_guests":          receipt.GuestDetails.NumGuests,
			"status":              receipt.Status,
			"total_paid_idr":      receipt.PricingBreakdown.TotalPriceMinor,
		})
	}
}

func buildVoucherPDFBytes(receipt *guest.ReceiptDTO) ([]byte, error) {
	qrToken := docgen.SignVoucherToken("", receipt.BookingReference, receipt.BookingID, receipt.StayDetails.CheckInDate)
	qrContent := fmt.Sprintf("https://pulangkeuttara.id/voucher/verify?ref=%s&token=%s&id=%s", receipt.BookingReference, qrToken, receipt.BookingID)

	qrPNG, err := docgen.GenerateQRPNG(qrContent, 200)
	if err != nil {
		return nil, err
	}

	checkIn, _ := time.Parse("2006-01-02", receipt.StayDetails.CheckInDate)
	checkOut, _ := time.Parse("2006-01-02", receipt.StayDetails.CheckOutDate)
	paidAt, pErr := time.Parse(time.RFC3339, receipt.PaymentSummary.PaidAt)
	if pErr != nil {
		paidAt = time.Now()
	}

	vData := docgen.VoucherData{
		Reference:     receipt.BookingReference,
		BookingID:     receipt.BookingID,
		GuestName:     receipt.GuestDetails.Name,
		GuestEmail:    receipt.GuestDetails.Email,
		GuestPhone:    receipt.GuestDetails.Phone,
		RoomTypeName:  receipt.RoomItem.RoomTypeName,
		RatePlanName:  receipt.RoomItem.RatePlanCode,
		Inclusions:    receipt.RoomItem.MealPlan,
		CheckIn:       checkIn,
		CheckOut:      checkOut,
		Nights:        receipt.StayDetails.TotalNights,
		NumRooms:      receipt.RoomItem.NumRooms,
		Adults:        receipt.GuestDetails.NumGuests,
		Children:      0,
		TotalPaidIDR:  receipt.PricingBreakdown.TotalPriceMinor,
		PaymentMethod: receipt.PaymentSummary.Provider,
		PaidAt:        paidAt,
		QRToken:       qrToken,
	}

	return docgen.GenerateVoucherPDF(vData, qrPNG)
}

func buildInvoicePDFBytes(receipt *guest.ReceiptDTO) ([]byte, error) {
	dpp, svc, pbjt := docgen.CalculatePBJTTaxes(receipt.PricingBreakdown.TotalPriceMinor)

	checkIn, _ := time.Parse("2006-01-02", receipt.StayDetails.CheckInDate)
	checkOut, _ := time.Parse("2006-01-02", receipt.StayDetails.CheckOutDate)
	paidAt, pErr := time.Parse(time.RFC3339, receipt.PaymentSummary.PaidAt)
	if pErr != nil {
		paidAt = time.Now()
	}

	iData := docgen.InvoiceData{
		InvoiceNumber:     receipt.InvoiceNumber,
		Reference:         receipt.BookingReference,
		BookingID:         receipt.BookingID,
		GuestName:         receipt.GuestDetails.Name,
		GuestEmail:        receipt.GuestDetails.Email,
		RoomTypeName:      receipt.RoomItem.RoomTypeName,
		CheckIn:           checkIn,
		CheckOut:          checkOut,
		Nights:            receipt.StayDetails.TotalNights,
		NumRooms:          receipt.RoomItem.NumRooms,
		NetRoomChargesIDR: dpp,
		ServiceChargeIDR:  svc,
		PBJTTaxIDR:        pbjt,
		TotalPaidIDR:      receipt.PricingBreakdown.TotalPriceMinor,
		PaymentMethod:     receipt.PaymentSummary.Provider,
		PaidAt:            paidAt,
	}

	return docgen.GenerateInvoicePDF(iData)
}
