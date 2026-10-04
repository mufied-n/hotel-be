package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

type mockRevenueCalendarStore struct {
	getCalendarFn func(ctx context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]rates.CalendarOverride, error)
	bulkUpsertFn  func(ctx context.Context, req rates.BulkCalendarUpdateRequest) (int64, error)
}

func (m *mockRevenueCalendarStore) GetCalendar(ctx context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]rates.CalendarOverride, error) {
	if m.getCalendarFn != nil {
		return m.getCalendarFn(ctx, start, end, planCode, roomTypeID)
	}
	return []rates.CalendarOverride{}, nil
}

func (m *mockRevenueCalendarStore) BulkUpsertOverrides(ctx context.Context, req rates.BulkCalendarUpdateRequest) (int64, error) {
	if m.bulkUpsertFn != nil {
		return m.bulkUpsertFn(ctx, req)
	}
	return 1, nil
}

type mockRevenuePromoStore struct {
	listCampaignsFn  func(ctx context.Context) ([]rates.PromoCampaign, error)
	createCampaignFn func(ctx context.Context, p rates.PromoCampaign) (string, error)
	updateCampaignFn func(ctx context.Context, id string, isActive bool, quotaTotal int) error
}

func (m *mockRevenuePromoStore) GetByCode(ctx context.Context, code string) (rates.PromoCampaign, error) {
	return rates.PromoCampaign{}, nil
}
func (m *mockRevenuePromoStore) ListCampaigns(ctx context.Context) ([]rates.PromoCampaign, error) {
	if m.listCampaignsFn != nil {
		return m.listCampaignsFn(ctx)
	}
	return []rates.PromoCampaign{}, nil
}
func (m *mockRevenuePromoStore) CreateCampaign(ctx context.Context, p rates.PromoCampaign) (string, error) {
	if m.createCampaignFn != nil {
		return m.createCampaignFn(ctx, p)
	}
	return "p-new-uuid", nil
}
func (m *mockRevenuePromoStore) UpdateCampaign(ctx context.Context, id string, isActive bool, quotaTotal int) error {
	if m.updateCampaignFn != nil {
		return m.updateCampaignFn(ctx, id, isActive, quotaTotal)
	}
	return nil
}
func (m *mockRevenuePromoStore) ReserveQuotaAtomic(ctx context.Context, code string) error {
	return nil
}
func (m *mockRevenuePromoStore) ReleaseQuotaAtomic(ctx context.Context, code string) error {
	return nil
}

func TestGetRevenueCalendar_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		query      string
		store      rates.RateCalendarStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing_date_range_no_params",
			query:      "",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_DATE_RANGE",
		},
		{
			name:       "missing_end_date",
			query:      "?start_date=2026-10-10",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_DATE_RANGE",
		},
		{
			name:       "invalid_start_date_format",
			query:      "?start_date=10-10-2026&end_date=2026-10-12",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name:       "invalid_end_date_format",
			query:      "?start_date=2026-10-10&end_date=not-a-date",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
		{
			name:       "end_date_before_start_date",
			query:      "?start_date=2026-10-15&end_date=2026-10-10",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_RANGE",
		},
		{
			name:       "date_range_exceeds_90_days",
			query:      "?start_date=2026-01-01&end_date=2026-05-01",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "DATE_RANGE_TOO_LARGE",
		},
		{
			name:       "calendar_store_nil",
			query:      "?start_date=2026-10-10&end_date=2026-10-12",
			store:      nil,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "CALENDAR_NOT_CONFIGURED",
		},
		{
			name:  "calendar_store_error",
			query: "?start_date=2026-10-10&end_date=2026-10-12",
			store: &mockRevenueCalendarStore{
				getCalendarFn: func(_ context.Context, _, _ time.Time, _ string, _ *string) ([]rates.CalendarOverride, error) {
					return nil, errors.New("db error")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:  "success_with_default_plan",
			query: "?start_date=2026-10-10&end_date=2026-10-12",
			store: &mockRevenueCalendarStore{
				getCalendarFn: func(_ context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]rates.CalendarOverride, error) {
					if planCode != "RO" {
						t.Errorf("planCode = %s, want RO", planCode)
					}
					if roomTypeID != nil {
						t.Errorf("roomTypeID should be nil")
					}
					return []rates.CalendarOverride{
						{Date: start, EffectivePrice: 850000},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "success_with_room_type_and_plan",
			query: "?start_date=2026-10-10&end_date=2026-10-12&rate_plan_code=BB&room_type_id=rt-deluxe",
			store: &mockRevenueCalendarStore{
				getCalendarFn: func(_ context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]rates.CalendarOverride, error) {
					if planCode != "BB" {
						t.Errorf("planCode = %s, want BB", planCode)
					}
					if roomTypeID == nil || *roomTypeID != "rt-deluxe" {
						t.Errorf("roomTypeID = %v, want rt-deluxe", roomTypeID)
					}
					return []rates.CalendarOverride{}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			d := Deps{CalendarStore: tt.store}
			r.GET("/api/v1/revenue/calendar", GetRevenueCalendar(d))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/revenue/calendar"+tt.query, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var errResp struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error body: %v", err)
				}
				if errResp.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", errResp.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestBulkUpdateCalendar_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		body       string
		store      rates.RateCalendarStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid_json",
			body:       "{invalid-json",
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
		{
			name:       "missing_room_type_ids",
			body:       `{"room_type_ids":[],"start_date":"2026-10-10","end_date":"2026-10-12"}`,
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_ROOM_TYPE_IDS",
		},
		{
			name:       "missing_start_date",
			body:       `{"room_type_ids":["rt-1"],"start_date":"","end_date":"2026-10-12"}`,
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_DATE_RANGE",
		},
		{
			name:       "missing_end_date",
			body:       `{"room_type_ids":["rt-1"],"start_date":"2026-10-10","end_date":""}`,
			store:      &mockRevenueCalendarStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_DATE_RANGE",
		},
		{
			name:       "calendar_store_nil",
			body:       `{"room_type_ids":["rt-1"],"start_date":"2026-10-10","end_date":"2026-10-12"}`,
			store:      nil,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "CALENDAR_NOT_CONFIGURED",
		},
		{
			name: "store_error",
			body: `{"room_type_ids":["rt-1"],"start_date":"2026-10-10","end_date":"2026-10-12"}`,
			store: &mockRevenueCalendarStore{
				bulkUpsertFn: func(_ context.Context, _ rates.BulkCalendarUpdateRequest) (int64, error) {
					return 0, errors.New("upsert validation failed")
				},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BULK_UPDATE_FAILED",
		},
		{
			name: "success_bulk_update",
			body: `{"room_type_ids":["rt-1"],"start_date":"2026-10-10","end_date":"2026-10-12","is_stop_sell":true}`,
			store: &mockRevenueCalendarStore{
				bulkUpsertFn: func(_ context.Context, req rates.BulkCalendarUpdateRequest) (int64, error) {
					if len(req.RoomTypeIDs) != 1 || req.RoomTypeIDs[0] != "rt-1" {
						t.Errorf("unexpected room_type_ids: %v", req.RoomTypeIDs)
					}
					if req.IsStopSell == nil || *req.IsStopSell != true {
						t.Errorf("expected is_stop_sell true")
					}
					return 3, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			d := Deps{CalendarStore: tt.store}
			r.PUT("/api/v1/revenue/calendar/bulk", BulkUpdateCalendar(d))

			req := httptest.NewRequest(http.MethodPut, "/api/v1/revenue/calendar/bulk", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var errResp struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error body: %v", err)
				}
				if errResp.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", errResp.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestListPromos_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		store      rates.PromoStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "promo_store_nil",
			store:      nil,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "PROMO_NOT_CONFIGURED",
		},
		{
			name: "promo_store_error",
			store: &mockRevenuePromoStore{
				listCampaignsFn: func(_ context.Context) ([]rates.PromoCampaign, error) {
					return nil, errors.New("query failure")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name: "success_empty_list",
			store: &mockRevenuePromoStore{
				listCampaignsFn: func(_ context.Context) ([]rates.PromoCampaign, error) {
					return nil, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "success_with_items",
			store: &mockRevenuePromoStore{
				listCampaignsFn: func(_ context.Context) ([]rates.PromoCampaign, error) {
					return []rates.PromoCampaign{
						{Code: "OCTOBREAK", Name: "Promo Okt"},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			d := Deps{PromoStore: tt.store}
			r.GET("/api/v1/revenue/promos", ListPromos(d))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/revenue/promos", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var errResp struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error body: %v", err)
				}
				if errResp.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", errResp.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestCreatePromo_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now()
	validFrom := now.Format(time.RFC3339)
	validTo := now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	pastTo := now.Add(-24 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name       string
		body       string
		store      rates.PromoStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid_json",
			body:       "{not-json",
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
		{
			name:       "missing_promo_code",
			body:       `{"code":"","discount_value":10,"discount_type":"PERCENT"}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_PROMO_CODE",
		},
		{
			name:       "invalid_discount_value_zero",
			body:       `{"code":"SALE","discount_value":0,"discount_type":"PERCENT"}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DISCOUNT_VALUE",
		},
		{
			name:       "invalid_discount_type",
			body:       `{"code":"SALE","discount_value":10,"discount_type":"INVALID"}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DISCOUNT_TYPE",
		},
		{
			name:       "percent_exceeds_100",
			body:       `{"code":"SALE","discount_value":105,"discount_type":"PERCENT"}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DISCOUNT_PERCENT",
		},
		{
			name:       "valid_to_before_valid_from",
			body:       `{"code":"SALE","discount_value":10,"discount_type":"PERCENT","valid_from":"` + validFrom + `","valid_to":"` + pastTo + `"}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_VALIDITY_PERIOD",
		},
		{
			name:       "promo_store_nil",
			body:       `{"code":"SALE","discount_value":10,"discount_type":"PERCENT","valid_from":"` + validFrom + `","valid_to":"` + validTo + `"}`,
			store:      nil,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "PROMO_NOT_CONFIGURED",
		},
		{
			name: "store_error",
			body: `{"code":"SALE","discount_value":10,"discount_type":"PERCENT","valid_from":"` + validFrom + `","valid_to":"` + validTo + `"}`,
			store: &mockRevenuePromoStore{
				createCampaignFn: func(_ context.Context, _ rates.PromoCampaign) (string, error) {
					return "", errors.New("duplicate key")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name: "success_percent",
			body: `{"code":"PROMO15","name":"Diskon 15%","discount_value":15,"discount_type":"PERCENT","valid_from":"` + validFrom + `","valid_to":"` + validTo + `"}`,
			store: &mockRevenuePromoStore{
				createCampaignFn: func(_ context.Context, p rates.PromoCampaign) (string, error) {
					if p.Code != "PROMO15" || p.DiscountValue != 15 {
						t.Errorf("unexpected promo payload: %+v", p)
					}
					return "p-created-id", nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "success_fixed",
			body: `{"code":"POTONG50K","name":"Potongan 50rb","discount_value":50000,"discount_type":"FIXED","valid_from":"` + validFrom + `","valid_to":"` + validTo + `"}`,
			store: &mockRevenuePromoStore{
				createCampaignFn: func(_ context.Context, p rates.PromoCampaign) (string, error) {
					if p.DiscountType != "FIXED" || p.DiscountValue != 50000 {
						t.Errorf("unexpected promo payload: %+v", p)
					}
					return "p-created-fixed", nil
				},
			},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			d := Deps{PromoStore: tt.store}
			r.POST("/api/v1/revenue/promos", CreatePromo(d))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/revenue/promos", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var errResp struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error body: %v", err)
				}
				if errResp.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", errResp.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestUpdatePromo_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		id         string
		body       string
		store      rates.PromoStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing_id",
			id:         "%20%20%20",
			body:       `{"is_active":false}`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_ID",
		},
		{
			name:       "invalid_json",
			id:         "promo-1",
			body:       `{bad-json`,
			store:      &mockRevenuePromoStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
		{
			name:       "promo_store_nil",
			id:         "promo-1",
			body:       `{"is_active":false}`,
			store:      nil,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "PROMO_NOT_CONFIGURED",
		},
		{
			name: "promo_not_found",
			id:   "promo-nonexistent",
			body: `{"is_active":false}`,
			store: &mockRevenuePromoStore{
				updateCampaignFn: func(_ context.Context, id string, _ bool, _ int) error {
					return rates.ErrPromoNotFound
				},
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "PROMO_NOT_FOUND",
		},
		{
			name: "store_internal_error",
			id:   "promo-1",
			body: `{"is_active":false}`,
			store: &mockRevenuePromoStore{
				updateCampaignFn: func(_ context.Context, id string, _ bool, _ int) error {
					return errors.New("db disconnect")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name: "success_update",
			id:   "promo-1",
			body: `{"is_active":false,"quota_total":50}`,
			store: &mockRevenuePromoStore{
				updateCampaignFn: func(_ context.Context, id string, isActive bool, quotaTotal int) error {
					if id != "promo-1" || isActive != false || quotaTotal != 50 {
						t.Errorf("unexpected update args: id=%s, active=%v, quota=%d", id, isActive, quotaTotal)
					}
					return nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			d := Deps{PromoStore: tt.store}
			r.PUT("/api/v1/revenue/promos/:id", UpdatePromo(d))

			url := "/api/v1/revenue/promos/" + tt.id
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var errResp struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error body: %v", err)
				}
				if errResp.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", errResp.Code, tt.wantCode)
				}
			}
		})
	}
}
