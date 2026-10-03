package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/gin-gonic/gin"
)

// SearchResultItem merepresentasikan opsi kamar hasil pencarian lintas varian (BE-G02).
type SearchResultItem = booking.SearchResultItem

// SearchRooms menangani pencarian ketersediaan kamar lintas varian (GET /api/v1/search).
func SearchRooms(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		checkInStr := c.Query("check_in")
		checkOutStr := c.Query("check_out")

		from, err1 := parseDate(checkInStr)
		to, err2 := parseDate(checkOutStr)
		if checkInStr == "" || checkOutStr == "" || err1 != nil || err2 != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "check_in and check_out (YYYY-MM-DD) are required", "INVALID_DATE_FORMAT")
			return
		}
		if !from.Before(to) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "check_out must be after check_in", "INVALID_DATE_RANGE")
			return
		}

		nights := int(to.Sub(from).Hours() / 24)
		if nights > maxStayNights {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "stay duration cannot exceed 30 nights", "EXCEEDS_MAX_LOS")
			return
		}

		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		checkInDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		if checkInDate.Before(today.Add(-24 * time.Hour)) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "check_in date cannot be in the past", "PAST_DATE")
			return
		}
		if to.After(now.AddDate(0, 0, bookingHorizonDays+1)) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "search dates cannot exceed 365 days booking horizon", "EXCEEDS_HORIZON")
			return
		}

		gc, gErr := parseGuestQuery(c.Query("rooms"), c.Query("adults"), c.Query("children"), c.Query("child_ages"))
		if gErr != nil {
			middleware.HttpErrorCode(c, gErr.status, gErr.msg, gErr.code)
			return
		}

		query := booking.SearchQuery{
			CheckIn:  from,
			CheckOut: to,
			Nights:   nights,
			Rooms:    gc.Rooms,
			Adults:   gc.Adults,
			Children: gc.Children,
		}

		var (
			results []booking.SearchResultItem
			err     error
		)
		if d.BookingSvc != nil {
			results, err = d.BookingSvc.SearchAvailability(c.Request.Context(), query)
		} else {
			results, err = booking.SearchAvailability(c.Request.Context(), d.CatalogStore, d.InvStore, d.RateSvc, query)
		}

		if err != nil {
			if errors.Is(err, booking.ErrCatalogUnavailable) {
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal membaca varian kamar", "CATALOG_ERROR")
				return
			}
			if errors.Is(err, booking.ErrInventoryUnavailable) {
				middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memeriksa ketersediaan kamar", "INVENTORY_ERROR")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
			return
		}

		availableCount := 0
		for _, r := range results {
			if r.Available {
				availableCount++
			}
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"search_criteria": map[string]any{
				"check_in":  checkInStr,
				"check_out": checkOutStr,
				"nights":    nights,
				"rooms":     gc.Rooms,
				"adults":    gc.Adults,
				"children":  gc.Children,
			},
			"total_variants":  len(results),
			"available_count": availableCount,
			"results":         results,
		})
	}
}

// GetAvailability membaca ketersediaan tipe kamar tertentu (GET /api/v1/availability).
func GetAvailability(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomTypeID := c.Query("room_type_id")
		from, err1 := parseDate(c.Query("check_in"))
		to, err2 := parseDate(c.Query("check_out"))
		if roomTypeID == "" || err1 != nil || err2 != nil || !from.Before(to) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "room_type_id, check_in, check_out (YYYY-MM-DD, check_in < check_out) wajib", "INVALID_QUERY")
			return
		}
		avail, err := d.InvStore.GetByDate(c.Request.Context(), roomTypeID, from, to)
		if errors.Is(err, inventory.ErrNotFound) {
			middleware.HttpErrorCode(c, http.StatusNotFound, "inventory tidak ditemukan untuk rentang tsb", "INVENTORY_NOT_FOUND")
			return
		}
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal membaca availability", "INTERNAL_ERROR")
			return
		}
		quotes, err := d.RateSvc.Quote(c.Request.Context(), roomTypeID, from, to)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusNotFound, "tipe kamar tidak dikenal", "UNKNOWN_ROOM_TYPE")
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"availability": avail,
			"quotes":       quotes,
			"total_minor":  sumQuotes(quotes),
		})
	}
}
