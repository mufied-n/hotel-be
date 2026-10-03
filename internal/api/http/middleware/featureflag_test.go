package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/gin-gonic/gin"
)

type mockFlagManager struct {
	featureflag.Manager
	flags map[string]bool
}

func (m *mockFlagManager) IsEnabled(_ context.Context, key string) bool {
	return m.flags[key]
}

func TestRequireFeature_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		ff         featureflag.Manager
		key        string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "nil manager defaults to enabled",
			ff:         nil,
			key:        "any_flag",
			wantStatus: http.StatusOK,
		},
		{
			name:       "disabled flag returns 503 FEATURE_DISABLED",
			ff:         &mockFlagManager{flags: map[string]bool{"promo_feature": false}},
			key:        "promo_feature",
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "FEATURE_DISABLED",
		},
		{
			name:       "enabled flag returns 200",
			ff:         &mockFlagManager{flags: map[string]bool{"promo_feature": true}},
			key:        "promo_feature",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/feature", RequireFeature(tc.ff, tc.key), func(c *gin.Context) {
				c.String(http.StatusOK, "feature active")
			})

			req := httptest.NewRequest(http.MethodGet, "/feature", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestFeatureEnabled_Helper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if !FeatureEnabled(c, nil, "any") {
		t.Errorf("expected FeatureEnabled true when manager is nil")
	}

	ff := &mockFlagManager{flags: map[string]bool{"flag1": true, "flag2": false}}
	if !FeatureEnabled(c, ff, "flag1") {
		t.Errorf("expected flag1 enabled")
	}
	if FeatureEnabled(c, ff, "flag2") {
		t.Errorf("expected flag2 disabled")
	}
}
