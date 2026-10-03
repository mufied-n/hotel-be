package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

func TestHealthAndReady_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		handler    gin.HandlerFunc
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "Healthz returns 200 OK",
			handler:    Healthz,
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusOK,
		},
		{
			name: "Ready with nil check returns 200 OK",
			handler: Ready(Deps{
				ReadyCheck: nil,
			}),
			method:     http.MethodGet,
			path:       "/ready",
			wantStatus: http.StatusOK,
		},
		{
			name: "Ready with successful check returns 200 OK",
			handler: Ready(Deps{
				ReadyCheck: func(ctx context.Context) error { return nil },
			}),
			method:     http.MethodGet,
			path:       "/ready",
			wantStatus: http.StatusOK,
		},
		{
			name: "Ready with failing check returns 503 Service Unavailable",
			handler: Ready(Deps{
				ReadyCheck: func(ctx context.Context) error { return errors.New("db disconnected") },
			}),
			method:     http.MethodGet,
			path:       "/ready",
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Handle(tc.method, tc.path, tc.handler)

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestHelpersAndDefaults(t *testing.T) {
	// 1. parseDate
	d, err := parseDate("2026-10-14")
	if err != nil || d.Year() != 2026 || d.Month() != 10 || d.Day() != 14 {
		t.Errorf("parseDate failed: %v", err)
	}
	_, err = parseDate("invalid")
	if err == nil {
		t.Errorf("expected error for invalid date")
	}

	// 2. sumQuotes
	total := sumQuotes([]rates.Quote{
		{RateMinor: 100},
		{RateMinor: 250},
	})
	if total != 350 {
		t.Errorf("sumQuotes = %d, want 350", total)
	}

	// 3. WithDefaults
	deps := Deps{}.WithDefaults()
	if deps.CatalogStore == nil {
		t.Errorf("expected default CatalogStore")
	}
	if deps.RequestTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", deps.RequestTimeout)
	}

	// 4. writeDomainError
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/err", nil)

	writeDomainError(c, booking.ErrNotFound, []errRule{ruleBookingNotFound}, getBookingFallback)
	if rec.Code != http.StatusNotFound {
		t.Errorf("writeDomainError matched rule status = %d, want 404", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/err", nil)
	writeDomainError(c2, errors.New("unknown"), []errRule{ruleBookingNotFound}, getBookingFallback)
	if rec2.Code != http.StatusInternalServerError {
		t.Errorf("writeDomainError fallback status = %d, want 500", rec2.Code)
	}
}
