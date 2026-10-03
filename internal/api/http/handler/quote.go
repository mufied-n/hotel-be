package handler

import (
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

// CalculateQuote menghitung kuotasi harga kamar terkunci (POST /api/v1/quotes).
func CalculateQuote(d Deps) gin.HandlerFunc {
	type req struct {
		RoomTypeID   string `json:"room_type_id" validate:"required"`
		RatePlanCode string `json:"rate_plan_code"`
		CheckIn      string `json:"check_in" validate:"required"`
		CheckOut     string `json:"check_out" validate:"required"`
		NumRooms     int    `json:"num_rooms"`
		NumGuests    int    `json:"num_guests"`
		Adults       int    `json:"adults,omitempty"`
		Children     int    `json:"children,omitempty"`
		ChildAges    []int  `json:"child_ages,omitempty"`
		PromoCode    string `json:"promo_code"`
	}
	return func(c *gin.Context) {
		var in req
		if !middleware.DecodeJSON(c, &in, "INVALID_JSON", "body JSON tidak valid") {
			return
		}
		if !validateDTO(c, &in) {
			return
		}
		from, err1 := parseDate(in.CheckIn)
		to, err2 := parseDate(in.CheckOut)
		if in.RoomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "room_type_id, check_in, check_out (check_in < check_out) wajib valid", "INVALID_DATE_FORMAT")
			return
		}

		gc, gErr := validateQuoteGuestCounts(in.NumRooms, in.NumGuests, in.Adults, in.Children, in.ChildAges)
		if gErr != nil {
			middleware.HttpErrorCode(c, gErr.status, gErr.msg, gErr.code)
			return
		}
		numRooms := gc.Rooms
		numGuests := gc.Total

		roomTypeID := in.RoomTypeID
		if d.CatalogStore != nil {
			variant, err := d.CatalogStore.GetVariant(c.Request.Context(), in.RoomTypeID)
			if err != nil {
				writeDomainError(c, err, quoteVariantLookupError, quoteVariantFallback)
				return
			}
			roomTypeID = variant.ID
			if in.Adults > 0 && in.Adults > variant.MaxAdults*numRooms {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "jumlah dewasa melebihi kapasitas kamar", "EXCEEDS_CAPACITY")
				return
			}
			if in.Children > 0 && in.Children > variant.MaxChildren*numRooms {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "jumlah anak melebihi kapasitas kamar", "EXCEEDS_CAPACITY")
				return
			}
			if numGuests > variant.MaxCapacity*numRooms {
				middleware.HttpErrorCode(c, http.StatusBadRequest, "jumlah tamu melebihi kapasitas maksimum varian kamar", "EXCEEDS_CAPACITY")
				return
			}
		}

		if strings.TrimSpace(in.PromoCode) != "" && !middleware.FeatureEnabled(c, d.FeatureFlag, "ff_promotions_engine") {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "fitur kode promosi sedang dinonaktifkan sementara", "PROMOTIONS_DISABLED")
			return
		}
		if d.RateEngine == nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "rate engine not configured", "INTERNAL_ERROR")
			return
		}
		q, err := d.RateEngine.CalculateLockedQuote(c.Request.Context(), rates.QuoteRequest{
			RoomTypeID:   roomTypeID,
			RatePlanCode: in.RatePlanCode,
			CheckIn:      from,
			CheckOut:     to,
			NumRooms:     numRooms,
			NumGuests:    numGuests,
			Adults:       in.Adults,
			Children:     in.Children,
			ChildAges:    in.ChildAges,
			PromoCode:    in.PromoCode,
		})
		if err != nil {
			writeDomainError(c, err, calculateQuoteErrors, calculateQuoteFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, q)
	}
}
