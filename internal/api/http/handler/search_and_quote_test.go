package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

type mockInvStore struct {
	avail []inventory.Availability
	err   error
}

func (m *mockInvStore) GetByDate(_ context.Context, _ string, from, to time.Time) ([]inventory.Availability, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.avail) > 0 {
		return m.avail, nil
	}
	var res []inventory.Availability
	for d := from; d.Before(to); d = d.Add(24 * time.Hour) {
		res = append(res, inventory.Availability{Date: d, TotalRooms: 5, AvailableRooms: 5})
	}
	return res, nil
}

type mockRates struct {
	quotes []rates.Quote
	err    error
}

func (m *mockRates) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quotes, nil
}

func TestSearchAndQuote_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	variants := []catalog.RoomVariant{
		{ID: "var-1", Code: "std-king", Name: "Standard King", BasePriceMinor: 500000, MaxCapacity: 2, MaxAdults: 2, MaxChildren: 1},
	}
	catStore := catalog.NewMemoryStore(variants)

	invStore := &mockInvStore{
		avail: []inventory.Availability{
			{Date: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), TotalRooms: 5, AvailableRooms: 5},
			{Date: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), TotalRooms: 5, AvailableRooms: 5},
		},
	}

	rateEngine := rates.NewEngine(map[string]int64{
		"var-1": 500_000,
	}, 1.25)

	memCal := rates.NewMemoryCalendarStore()
	isStopSell := true
	_, _ = memCal.BulkUpsertOverrides(context.Background(), rates.BulkCalendarUpdateRequest{
		RoomTypeIDs: []string{"var-1"},
		StartDate:   "2026-10-20",
		EndDate:     "2026-10-20",
		IsStopSell:  &isStopSell,
	})
	minStay3 := 3
	_, _ = memCal.BulkUpsertOverrides(context.Background(), rates.BulkCalendarUpdateRequest{
		RoomTypeIDs: []string{"var-1"},
		StartDate:   "2026-10-25",
		EndDate:     "2026-10-27",
		MinLOS:      &minStay3,
	})
	rateEngine.SetCalendarStore(memCal)

	deps := Deps{
		CatalogStore:  catStore,
		InvStore:      invStore,
		RateEngine:    rateEngine,
		RateSvc:       rateEngine,
		QuoteStore:    rateEngine.QuoteStore(),
		CalendarStore: memCal,
	}

	r := gin.New()
	r.GET("/api/v1/search", SearchRooms(deps))
	r.GET("/api/v1/availability", GetAvailability(deps))
	r.POST("/api/v1/quotes", CalculateQuote(deps))

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		wantStatus int
	}{
		{
			name:       "search missing check-in returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_out=2026-10-15",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search check-out before check-in returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2026-10-16&check_out=2026-10-15",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search valid dates returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2026-10-14&check_out=2026-10-16&rooms=1&adults=2",
			wantStatus: http.StatusOK,
		},
		{
			name:       "availability missing date returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/availability?room_type_id=var-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "availability valid query returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/availability?room_type_id=var-1&check_in=2026-10-14&check_out=2026-10-16",
			wantStatus: http.StatusOK,
		},
		{
			name:       "quote invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote missing required fields returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"check_in":"2026-10-14"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote valid payload returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","rate_plan_code":"room_only","check_in":"2026-10-14","check_out":"2026-10-16","num_rooms":1,"num_guests":2}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "search in the past returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2020-01-01&check_out=2020-01-03",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search exceeds max stay returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2026-10-14&check_out=2026-11-20",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search exceeds horizon returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2028-10-14&check_out=2028-10-16",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "availability unknown room type returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/availability?room_type_id=unknown-var&check_in=2026-10-14&check_out=2026-10-16",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "quote check_out before check_in returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-16","check_out":"2026-10-14","num_rooms":1,"num_guests":2}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote variant not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"nonexistent","check_in":"2026-10-14","check_out":"2026-10-16","num_rooms":1,"num_guests":2}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "quote adults exceeds capacity returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-14","check_out":"2026-10-16","num_rooms":1,"num_guests":5,"adults":5}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote children exceeds capacity returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-14","check_out":"2026-10-16","num_rooms":1,"num_guests":3,"adults":1,"children":3}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote promo disabled returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-14","check_out":"2026-10-16","num_rooms":1,"num_guests":2,"promo_code":"DISCOUNT50"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote stop sell restriction returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-20","check_out":"2026-10-22","num_rooms":1,"num_guests":2}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quote min length of stay violated returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/quotes",
			body:       `{"room_type_id":"var-1","check_in":"2026-10-25","check_out":"2026-10-26","num_rooms":1,"num_guests":2}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search on stop sold date returns 200 with room unavailable",
			method:     http.MethodGet,
			url:        "/api/v1/search?check_in=2026-10-20&check_out=2026-10-22&rooms=1&adults=2",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Buffer
			if tc.body != "" {
				bodyReader = bytes.NewBufferString(tc.body)
			} else {
				bodyReader = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.url, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
