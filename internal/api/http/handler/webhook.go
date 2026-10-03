package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/gin-gonic/gin"
)

// XenditWebhook menerima notifikasi callback pembayaran dari Xendit secara idempoten dan terverifikasi (POST /api/v1/webhooks/xendit).
func XenditWebhook(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.XenditGateway == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "xendit gateway is not configured", "NOT_CONFIGURED")
			return
		}

		token := c.GetHeader("x-callback-token")
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if middleware.IsMaxBytesError(err) {
				middleware.HttpErrorCode(c, http.StatusRequestEntityTooLarge, "webhook payload melebihi batas 1MB", "PAYLOAD_TOO_LARGE")
				return
			}
			middleware.HttpErrorCode(c, http.StatusBadRequest, "failed to read webhook body", "BAD_REQUEST")
			return
		}

		payload, err := d.XenditGateway.VerifyWebhook(token, bodyBytes)
		if err != nil {
			if errors.Is(err, payment.ErrInvalidWebhookToken) {
				middleware.HttpErrorCode(c, http.StatusUnauthorized, "invalid webhook token", "UNAUTHORIZED")
				return
			}
			middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "INVALID_WEBHOOK_PAYLOAD")
			return
		}

		outcome, err := d.BookingSvc.ApplyPaymentEvent(c.Request.Context(), booking.PaymentEvent{
			ExternalID: payload.ExternalID,
			ID:         payload.ID,
			Status:     payload.Status,
			Amount:     payload.Amount,
			Currency:   payload.Currency,
		})
		if err != nil {
			switch {
			case errors.Is(err, booking.ErrNotFound):
				middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan", "BOOKING_NOT_FOUND")
			case errors.Is(err, booking.ErrPaymentAmountMismatch):
				middleware.HttpErrorCode(c, http.StatusUnprocessableEntity, err.Error(), "PAYMENT_AMOUNT_MISMATCH")
			case errors.Is(err, booking.ErrPaymentCurrencyMismatch):
				middleware.HttpErrorCode(c, http.StatusUnprocessableEntity, err.Error(), "PAYMENT_CURRENCY_MISMATCH")
			case errors.Is(err, booking.ErrInvoiceMismatch):
				middleware.HttpErrorCode(c, http.StatusUnprocessableEntity, "invoice ID does not match recorded payment attempt", "INVOICE_ID_MISMATCH")
			case errors.Is(err, booking.ErrHoldExpired):
				middleware.HttpErrorCode(c, http.StatusConflict, "hold has expired, payment rejected", "HOLD_EXPIRED")
			case strings.Contains(err.Error(), "confirm"):
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to confirm booking: "+err.Error(), "CONFIRM_FAILED")
			case strings.Contains(err.Error(), "cancel"):
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to cancel booking: "+err.Error(), "CANCEL_FAILED")
			default:
				middleware.HttpErrorCode(c, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
			}
			return
		}

		switch outcome.Status {
		case booking.PaymentOutcomeConfirmed, booking.PaymentOutcomeAlreadyConfirmed, booking.PaymentOutcomeCancelled:
			middleware.WriteJSON(c, http.StatusOK, map[string]string{
				"status":  "ok",
				"message": outcome.Message,
			})
		case booking.PaymentOutcomeStaleIgnored, booking.PaymentOutcomeUnhandledStatus:
			middleware.WriteJSON(c, http.StatusOK, map[string]string{
				"status":  "ignored",
				"message": outcome.Message,
			})
		default:
			middleware.WriteJSON(c, http.StatusOK, map[string]string{
				"status":  "ok",
				"message": outcome.Message,
			})
		}
	}
}
