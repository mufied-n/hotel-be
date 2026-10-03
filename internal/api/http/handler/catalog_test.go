package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

type mockCatalogStore struct {
	variants []catalog.RoomVariant
	err      error
}

func (m *mockCatalogStore) ListVariants(_ context.Context) ([]catalog.RoomVariant, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.variants, nil
}

func (m *mockCatalogStore) GetVariant(_ context.Context, idOrCode string) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	for _, v := range m.variants {
		if v.ID == idOrCode || v.Code == idOrCode {
			return v, nil
		}
	}
	return catalog.RoomVariant{}, catalog.ErrVariantNotFound
}

func (m *mockCatalogStore) CreateVariant(_ context.Context, v catalog.RoomVariant) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	if v.Code == "duplicate" {
		return catalog.RoomVariant{}, catalog.ErrDuplicateCode
	}
	v.ID = "new-id"
	m.variants = append(m.variants, v)
	return v, nil
}

func (m *mockCatalogStore) UpdateVariant(_ context.Context, id string, v catalog.RoomVariant) (catalog.RoomVariant, error) {
	if m.err != nil {
		return catalog.RoomVariant{}, m.err
	}
	for i, existing := range m.variants {
		if existing.ID == id {
			v.ID = id
			m.variants[i] = v
			return v, nil
		}
	}
	return catalog.RoomVariant{}, catalog.ErrVariantNotFound
}

func (m *mockCatalogStore) DeleteVariant(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if id == "in-use" {
		return catalog.ErrCannotDelete
	}
	for i, existing := range m.variants {
		if existing.ID == id {
			m.variants = append(m.variants[:i], m.variants[i+1:]...)
			return nil
		}
	}
	return catalog.ErrVariantNotFound
}

func TestCatalogHandlers_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rateEngine := rates.NewEngine(map[string]int64{}, 1.0)
	baseStore := &mockCatalogStore{
		variants: []catalog.RoomVariant{
			{ID: "v1", Code: "std-king", Name: "Standard King", BasePriceMinor: 500000, MaxCapacity: 2},
		},
	}
	deps := Deps{CatalogStore: baseStore, RateEngine: rateEngine}

	r := gin.New()
	r.GET("/api/v1/catalog/rooms", GetCatalogRooms(deps))
	r.GET("/api/v1/catalog/rooms/:id", GetCatalogRoom(deps))
	r.POST("/api/v1/catalog/rooms", CreateCatalogRoom(deps))
	r.PUT("/api/v1/catalog/rooms/:id", UpdateCatalogRoom(deps))
	r.DELETE("/api/v1/catalog/rooms/:id", DeleteCatalogRoom(deps))

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		wantStatus int
	}{
		{
			name:       "list catalog rooms returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/catalog/rooms",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get catalog room found returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/catalog/rooms/v1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get catalog room not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/catalog/rooms/nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "create catalog room valid returns 201",
			method:     http.MethodPost,
			url:        "/api/v1/catalog/rooms",
			body:       `{"code":"dlx-twin","name":"Deluxe Twin","base_price_minor":750000,"max_capacity":2}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "create catalog room duplicate returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/catalog/rooms",
			body:       `{"code":"duplicate","name":"Duplicate Room","base_price_minor":750000,"max_capacity":2}`,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "create catalog room invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/catalog/rooms",
			body:       `{bad json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update catalog room valid returns 200",
			method:     http.MethodPut,
			url:        "/api/v1/catalog/rooms/v1",
			body:       `{"code":"std-king-up","name":"Standard King Updated","base_price_minor":600000,"max_capacity":2}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "update catalog room not found returns 404",
			method:     http.MethodPut,
			url:        "/api/v1/catalog/rooms/nonexistent",
			body:       `{"code":"none","name":"None","base_price_minor":600000,"max_capacity":2}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete catalog room cannot delete returns 409",
			method:     http.MethodDelete,
			url:        "/api/v1/catalog/rooms/in-use",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "delete catalog room not found returns 404",
			method:     http.MethodDelete,
			url:        "/api/v1/catalog/rooms/nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete catalog room valid returns 200",
			method:     http.MethodDelete,
			url:        "/api/v1/catalog/rooms/v1",
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

func TestCatalogHandlers_Errors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	errStore := &mockCatalogStore{err: errors.New("db error")}
	rateEngine := rates.NewEngine(map[string]int64{}, 1.0)
	deps := Deps{CatalogStore: errStore, RateEngine: rateEngine}

	r := gin.New()
	r.GET("/catalog/rooms", GetCatalogRooms(deps))
	r.GET("/catalog/rooms/:id", GetCatalogRoom(deps))
	r.POST("/catalog/rooms", CreateCatalogRoom(deps))
	r.PUT("/catalog/rooms/:id", UpdateCatalogRoom(deps))
	r.DELETE("/catalog/rooms/:id", DeleteCatalogRoom(deps))

	// list error -> 500
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/catalog/rooms", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}

	// get error -> 500
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/catalog/rooms/v1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}

	// create error -> 500
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/catalog/rooms", bytes.NewBufferString(`{"id":"v2","name":"v2","code":"v2","base_price_minor":100}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}

	// update error -> 500
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/catalog/rooms/v1", bytes.NewBufferString(`{"id":"v1","name":"v1","code":"v1","base_price_minor":100}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}

	// delete error -> 500
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/catalog/rooms/v1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}
