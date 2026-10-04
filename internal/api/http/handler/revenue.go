package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

// GetRevenueCalendar mengembalikan kalender tarif dan restriksi operasional.
func GetRevenueCalendar(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		startStr := strings.TrimSpace(c.Query("start_date"))
		endStr := strings.TrimSpace(c.Query("end_date"))
		if startStr == "" || endStr == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "start_date and end_date are required (YYYY-MM-DD)", "MISSING_DATE_RANGE")
			return
		}

		startDate, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "invalid start_date format, expected YYYY-MM-DD", "INVALID_DATE_FORMAT")
			return
		}
		endDate, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "invalid end_date format, expected YYYY-MM-DD", "INVALID_DATE_FORMAT")
			return
		}

		if endDate.Before(startDate) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "end_date must be greater than or equal to start_date", "INVALID_DATE_RANGE")
			return
		}
		if endDate.Sub(startDate) > 90*24*time.Hour {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "date range exceeds maximum limit of 90 days", "DATE_RANGE_TOO_LARGE")
			return
		}

		planCode := strings.TrimSpace(c.DefaultQuery("rate_plan_code", "RO"))
		var roomTypeID *string
		if rt := strings.TrimSpace(c.Query("room_type_id")); rt != "" {
			roomTypeID = &rt
		}

		if d.CalendarStore == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "rate calendar service is not configured", "CALENDAR_NOT_CONFIGURED")
			return
		}

		items, err := d.CalendarStore.GetCalendar(c.Request.Context(), startDate, endDate, planCode, roomTypeID)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to query rate calendar", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, gin.H{
			"start_date": startStr,
			"end_date":   endStr,
			"items":      items,
		})
	}
}

// BulkUpdateCalendar memproses pembaruan tarif dan restriksi kalender secara massal.
func BulkUpdateCalendar(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req rates.BulkCalendarUpdateRequest
		if !middleware.DecodeJSON(c, &req, "INVALID_JSON", "body JSON tidak valid") {
			return
		}

		if len(req.RoomTypeIDs) == 0 {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "at least one room_type_id is required", "MISSING_ROOM_TYPE_IDS")
			return
		}
		if req.StartDate == "" || req.EndDate == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "start_date and end_date are required", "MISSING_DATE_RANGE")
			return
		}

		if d.CalendarStore == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "rate calendar service is not configured", "CALENDAR_NOT_CONFIGURED")
			return
		}

		affected, err := d.CalendarStore.BulkUpsertOverrides(c.Request.Context(), req)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, err.Error(), "BULK_UPDATE_FAILED")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, gin.H{
			"status":            "success",
			"message":           "bulk calendar updated successfully",
			"affected_records":  affected,
			"cache_invalidated": true,
		})
	}
}

// ListPromos mengembalikan seluruh kampanye kode promo.
func ListPromos(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.PromoStore == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "promo service is not configured", "PROMO_NOT_CONFIGURED")
			return
		}

		list, err := d.PromoStore.ListCampaigns(c.Request.Context())
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to list promos", "INTERNAL_ERROR")
			return
		}

		if list == nil {
			list = []rates.PromoCampaign{}
		}

		middleware.WriteJSON(c, http.StatusOK, gin.H{
			"items": list,
		})
	}
}

// CreatePromo membuat kampanye promo baru.
func CreatePromo(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var p rates.PromoCampaign
		if !middleware.DecodeJSON(c, &p, "INVALID_JSON", "body JSON tidak valid") {
			return
		}

		code := strings.TrimSpace(p.Code)
		if code == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "promo code is required", "MISSING_PROMO_CODE")
			return
		}
		if p.DiscountValue <= 0 {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "discount_value must be greater than zero", "INVALID_DISCOUNT_VALUE")
			return
		}
		if p.DiscountType != "PERCENT" && p.DiscountType != "FIXED" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "discount_type must be PERCENT or FIXED", "INVALID_DISCOUNT_TYPE")
			return
		}
		if p.DiscountType == "PERCENT" && p.DiscountValue > 100 {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "percentage discount cannot exceed 100", "INVALID_DISCOUNT_PERCENT")
			return
		}
		if p.ValidTo.Before(p.ValidFrom) {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "valid_to must be after valid_from", "INVALID_VALIDITY_PERIOD")
			return
		}

		if d.PromoStore == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "promo service is not configured", "PROMO_NOT_CONFIGURED")
			return
		}

		p.IsActive = true

		id, err := d.PromoStore.CreateCampaign(c.Request.Context(), p)
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to create promo campaign", "INTERNAL_ERROR")
			return
		}

		p.ID = id
		middleware.WriteJSON(c, http.StatusCreated, p)
	}
}

type updatePromoReq struct {
	IsActive   *bool `json:"is_active" binding:"required"`
	QuotaTotal int   `json:"quota_total"`
}

// UpdatePromo memperbarui status aktif atau kuota promo.
func UpdatePromo(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "promo id is required", "MISSING_ID")
			return
		}

		var req updatePromoReq
		if !middleware.DecodeJSON(c, &req, "INVALID_JSON", "body JSON tidak valid") {
			return
		}

		if d.PromoStore == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "promo service is not configured", "PROMO_NOT_CONFIGURED")
			return
		}

		isActive := true
		if req.IsActive != nil {
			isActive = *req.IsActive
		}

		err := d.PromoStore.UpdateCampaign(c.Request.Context(), id, isActive, req.QuotaTotal)
		if err != nil {
			if errors.Is(err, rates.ErrPromoNotFound) {
				middleware.HttpErrorCode(c, http.StatusNotFound, "promo campaign not found", "PROMO_NOT_FOUND")
				return
			}
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "failed to update promo campaign", "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, gin.H{
			"status":  "success",
			"message": "promo campaign updated",
		})
	}
}
