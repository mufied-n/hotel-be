package handler

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/gin-gonic/gin"
)

// CreateBooking membuat reservasi booking baru dengan proteksi idempotency terisolasi (POST /api/v1/bookings).
func CreateBooking(d Deps) gin.HandlerFunc {
	type req struct {
		QuoteID              string `json:"quote_id"`
		TermsAccepted        bool   `json:"terms_accepted"`
		PrivacyAccepted      bool   `json:"privacy_accepted"`
		RoomTypeID           string `json:"room_type_id"`
		CheckIn              string `json:"check_in"`
		CheckOut             string `json:"check_out"`
		NumRooms             int    `json:"num_rooms"`
		NumGuests            int    `json:"num_guests"`
		GuestName            string `json:"guest_name"`
		GuestEmail           string `json:"guest_email"`
		GuestPhone           string `json:"guest_phone,omitempty"`
		EstimatedArrivalTime string `json:"estimated_arrival_time,omitempty"`
		SpecialRequests      string `json:"special_requests,omitempty"`
	}
	return func(c *gin.Context) {
		idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if len(idempotencyKey) > 64 {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "idempotency key must be between 1 and 64 characters", "INVALID_IDEMPOTENCY_KEY")
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if middleware.IsMaxBytesError(err) {
				middleware.HttpErrorCode(c, http.StatusRequestEntityTooLarge, "request body melebihi batas 1MB", "PAYLOAD_TOO_LARGE")
				return
			}
			middleware.HttpErrorCode(c, http.StatusBadRequest, "gagal membaca request body", "INVALID_BODY")
			return
		}

		middleware.WithIdempotency(c, d.IdempotencyStore, d.FeatureFlag, idempotencyKey, bodyBytes, func() (int, []byte, error) {
			var in req
			if err := json.Unmarshal(bodyBytes, &in); err != nil {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
				return 0, nil, err
			}
			if !validateDTO(c, &in) {
				return 0, nil, errors.New("validation failed")
			}
			from, err1 := parseDate(in.CheckIn)
			to, err2 := parseDate(in.CheckOut)
			if err1 != nil || err2 != nil {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "check_in/check_out wajib format YYYY-MM-DD", "INVALID_DATE_FORMAT")
				return 0, nil, errors.New("invalid date format")
			}

			roomsCount := in.NumRooms
			if roomsCount <= 0 {
				roomsCount = 1
			}
			if in.NumGuests > 0 && in.NumGuests < roomsCount {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "guests count must be greater than or equal to rooms count", "INVALID_GUEST_COUNT")
				return 0, nil, errors.New("invalid guest count")
			}
			if d.CatalogStore != nil && in.RoomTypeID != "" {
				variant, err := d.CatalogStore.GetVariant(c.Request.Context(), in.RoomTypeID)
				if err != nil && errors.Is(err, catalog.ErrVariantNotFound) {
					middleware.HttpErrorCode(c, http.StatusNotFound, "tipe kamar tidak ditemukan", "ROOM_NOT_FOUND")
					return 0, nil, err
				}
				if err == nil {
					in.RoomTypeID = variant.ID
					if in.NumGuests > variant.MaxCapacity*roomsCount {
						middleware.HttpErrorCode(c, http.StatusBadRequest, "jumlah tamu melebihi kapasitas maksimum varian kamar", "EXCEEDS_CAPACITY")
						return 0, nil, errors.New("exceeds capacity")
					}
				}
			}

			b, charge, err := d.BookingSvc.Create(c.Request.Context(), booking.CreateInput{
				QuoteID:              in.QuoteID,
				TermsAccepted:        in.TermsAccepted,
				PrivacyAccepted:      in.PrivacyAccepted,
				RoomTypeID:           in.RoomTypeID,
				CheckIn:              from,
				CheckOut:             to,
				NumRooms:             in.NumRooms,
				NumGuests:            in.NumGuests,
				GuestName:            in.GuestName,
				GuestEmail:           in.GuestEmail,
				GuestPhone:           in.GuestPhone,
				EstimatedArrivalTime: in.EstimatedArrivalTime,
				SpecialRequests:      in.SpecialRequests,
			})
			if err != nil {
				writeDomainError(c, err, createBookingErrors, createBookingFallback)
				return 0, nil, err
			}
			if d.Enqueuer != nil {
				_ = d.Enqueuer.EnqueueReleaseHold(c.Request.Context(), b.ID, d.BookingSvc.HoldTimeout())
			}

			respObj := map[string]any{
				"booking":            b,
				"guest_access_token": b.GuestToken,
				"payment_url":        charge.PaymentURL,
				"reference":          charge.Reference,
				"expires_at":         b.ExpiresAt,
				"server_time":        time.Now().UTC(),
			}
			respBytes, err := json.Marshal(respObj)
			if err != nil {
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to serialize booking response", "INTERNAL_ERROR")
				return 0, nil, err
			}

			return http.StatusCreated, respBytes, nil
		})
	}
}

// GetBooking membaca detail reservasi (GET /api/v1/bookings/{id}).
func GetBooking(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		b, err := d.BookingSvc.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeDomainError(c, err, getBookingErrors, getBookingFallback)
			return
		}

		authCtx := middleware.GetAuthContext(c.Request.Context())
		guestToken := middleware.GetGuestToken(c.Request.Context())

		if !middleware.FeatureEnabled(c, d.FeatureFlag, "ff_pii_masking_guard") || authCtx.Role != "guest" || (b.GuestToken != "" && guestToken == b.GuestToken) {
			b.GuestToken = ""
			middleware.WriteJSON(c, http.StatusOK, b)
			return
		}

		middleware.WriteJSON(c, http.StatusOK, b.ToPublicDTO())
	}
}

// GetBookingPayment membaca informasi pemulihan tautan pembayaran (GET /api/v1/bookings/{id}/payment).
func GetBookingPayment(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.BookingSvc == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "layanan pemesanan belum dikonfigurasi", "NOT_IMPLEMENTED")
			return
		}
		id := c.Param("id")
		b, err := d.BookingSvc.Get(c.Request.Context(), id)
		if err != nil {
			writeDomainError(c, err, bookingPaymentLookup, getBookingFallback)
			return
		}

		authCtx := middleware.GetAuthContext(c.Request.Context())
		guestToken := middleware.GetGuestToken(c.Request.Context())

		authorized := false
		if authCtx.Role != "guest" {
			authorized = true
		} else if b.GuestToken != "" && guestToken == b.GuestToken {
			authorized = true
		} else if d.GuestSvc != nil {
			sessionToken := middleware.ExtractGuestSessionToken(c.Request)
			if sessionToken != "" {
				sess, sErr := d.GuestSvc.ValidateSession(c.Request.Context(), sessionToken)
				if sErr == nil && sess != nil && strings.EqualFold(strings.TrimSpace(sess.GuestEmail), strings.TrimSpace(b.GuestEmail)) {
					authorized = true
				}
			}
		}

		if !authorized {
			middleware.HttpErrorCode(c, http.StatusNotFound, "booking tidak ditemukan atau Anda tidak memiliki akses", "BOOKING_NOT_FOUND")
			return
		}

		recovery, err := d.BookingSvc.GetPaymentRecovery(c.Request.Context(), id)
		if err != nil {
			writeDomainError(c, err, paymentRecoveryErrors, paymentRecoveryFallback)
			return
		}

		middleware.WriteJSON(c, http.StatusOK, recovery)
	}
}

// CancelBooking membatalkan reservasi (POST /api/v1/bookings/{id}/cancel).
func CancelBooking(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		authCtx := middleware.GetAuthContext(c.Request.Context())
		guestToken := middleware.GetGuestToken(c.Request.Context())

		if authCtx.Role == "guest" {
			b, err := d.BookingSvc.Get(c.Request.Context(), id)
			if err != nil {
				writeDomainError(c, err, getBookingErrors, getBookingFallback)
				return
			}
			if b.GuestToken == "" || guestToken != b.GuestToken {
				middleware.HttpErrorCode(c, http.StatusForbidden, "guest token tidak valid atau tidak memiliki akses pembatalan", "FORBIDDEN_OWNERSHIP")
				return
			}
		}

		if err := d.BookingSvc.Cancel(c.Request.Context(), id); err != nil {
			if errors.Is(err, booking.ErrNonRefundable) && middleware.FeatureEnabled(c, d.FeatureFlag, "ff_strict_cancellation_policy") {
				middleware.HttpErrorCode(c, http.StatusConflict, "reservasi non-refundable tidak dapat dibatalkan oleh tamu", "NON_REFUNDABLE_BOOKING")
				return
			}
			if errors.Is(err, booking.ErrCancellationDeadlineExceeded) && middleware.FeatureEnabled(c, d.FeatureFlag, "ff_strict_cancellation_policy") {
				middleware.HttpErrorCode(c, http.StatusConflict, "batas waktu pembatalan gratis 48 jam sebelum check-in telah terlewati", "CANCELLATION_DEADLINE_EXCEEDED")
				return
			}
			writeDomainError(c, err, cancelBookingErrors, cancelBookingFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]string{"status": "cancelled", "id": id})
	}
}

// CheckIn memproses tamu check-in (POST /api/v1/bookings/{id}/check-in).
func CheckIn(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if !middleware.FeatureEnabled(c, d.FeatureFlag, "ff_room_readiness_checkin_guard") {
			ctx = booking.WithBypassRoomReadiness(ctx)
		}
		res, err := d.BookingSvc.CheckIn(ctx, c.Param("id"))
		if err != nil {
			writeDomainError(c, err, checkInErrors, checkInFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, res)
	}
}

// CheckOut memproses tamu check-out (POST /api/v1/bookings/{id}/check-out).
func CheckOut(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := d.BookingSvc.CheckOut(c.Request.Context(), id); err != nil {
			writeDomainError(c, err, checkOutErrors, checkOutFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]string{"status": "checked_out", "id": id})
	}
}

// NoShow menandai reservasi tidak hadir (POST /api/v1/bookings/{id}/no-show).
func NoShow(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := d.BookingSvc.MarkNoShow(c.Request.Context(), id); err != nil {
			writeDomainError(c, err, noShowErrors, noShowFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]string{"status": "no_show", "id": id})
	}
}
