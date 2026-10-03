package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/gin-gonic/gin"
)

func newTestEnforcerForFF(t *testing.T) *casbin.SyncedEnforcer {
	t.Helper()
	policies := [][]string{
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"p", "guest", "/api/v1/search", "GET"},
		{"p", "guest", "/api/v1/quotes", "POST"},
		{"p", "receptionist", "/api/v1/bookings/*", "*"},
	}
	e, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}
	return e
}

func TestRequireFeature_Middleware(t *testing.T) {
	flags := map[string]featureflag.Flag{
		"feat_enabled": {
			Key:          "feat_enabled",
			Enabled:      true,
			AllowedRoles: []string{},
		},
		"feat_disabled": {
			Key:          "feat_disabled",
			Enabled:      false,
			AllowedRoles: []string{},
		},
		"feat_gm_only": {
			Key:          "feat_gm_only",
			Enabled:      true,
			AllowedRoles: []string{"gm_admin"},
		},
	}
	ffMgr := featureflag.NewMemoryManager(flags)

	okHandler := func(c *gin.Context) {
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write([]byte(`{"status":"ok"}`))
	}

	tests := []struct {
		name         string
		flagKey      string
		role         string
		expectedCode int
		expectedBody string
	}{
		{
			name:         "enabled global flag allows request",
			flagKey:      "feat_enabled",
			role:         "guest",
			expectedCode: http.StatusOK,
			expectedBody: `{"status":"ok"}`,
		},
		{
			name:         "disabled global flag returns 503",
			flagKey:      "feat_disabled",
			role:         "guest",
			expectedCode: http.StatusServiceUnavailable,
			expectedBody: "FEATURE_DISABLED",
		},
		{
			name:         "role-scoped flag allows matching role gm_admin",
			flagKey:      "feat_gm_only",
			role:         "gm_admin",
			expectedCode: http.StatusOK,
			expectedBody: `{"status":"ok"}`,
		},
		{
			name:         "role-scoped flag blocks non-matching role receptionist with 503",
			flagKey:      "feat_gm_only",
			role:         "receptionist",
			expectedCode: http.StatusServiceUnavailable,
			expectedBody: "FEATURE_DISABLED",
		},
		{
			name:         "role-scoped flag blocks anonymous role with 503",
			flagKey:      "feat_gm_only",
			role:         "",
			expectedCode: http.StatusServiceUnavailable,
			expectedBody: "FEATURE_DISABLED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/test", RequireFeature(ffMgr, tc.flagKey), okHandler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.role != "" {
				ctx := context.WithValue(req.Context(), RoleKey, tc.role)
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(tc.expectedBody)) {
				t.Errorf("expected body to contain %q, got %q", tc.expectedBody, rec.Body.String())
			}
		})
	}
}

func TestAdminFeatureFlags_API(t *testing.T) {
	testFlags := map[string]featureflag.Flag{
		"ff_multi_variant_search": {
			Key:     "ff_multi_variant_search",
			Name:    "Multi-Variant Search",
			Enabled: true,
		},
		"ff_catalog_write": {
			Key:          "ff_catalog_write",
			Name:         "Catalog Mutation",
			Enabled:      true,
			AllowedRoles: []string{"revenue_mgr", "gm_admin"},
		},
	}
	ffMgr := featureflag.NewMemoryManager(testFlags)
	enforcer := newTestEnforcerForFF(t)

	deps := Deps{
		StaffAuth:   TestStaffVerifier(),
		FeatureFlag: ffMgr,
		Enforcer:    enforcer,
	}
	router := NewRouter(deps)

	// 1. GET /api/v1/admin/feature-flags as gm_admin (should succeed)
	t.Run("GET admin flags as gm_admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/feature-flags", nil)
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Total int                `json:"total"`
			Flags []featureflag.Flag `json:"flags"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Total != 2 {
			t.Errorf("expected total 2 flags, got %d", resp.Total)
		}
	})

	// 2. GET /api/v1/admin/feature-flags as receptionist (forbidden by Casbin RBAC)
	t.Run("GET admin flags as receptionist forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/feature-flags", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden, got %d", rec.Code)
		}
	})

	// 3. PUT /api/v1/admin/feature-flags/ff_multi_variant_search (toggle to disabled)
	t.Run("PUT admin flag toggle", func(t *testing.T) {
		disabled := false
		updatePayload := map[string]any{
			"enabled":       disabled,
			"allowed_roles": []string{"gm_admin"},
		}
		body, _ := json.Marshal(updatePayload)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feature-flags/ff_multi_variant_search", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Status string           `json:"status"`
			Flag   featureflag.Flag `json:"flag"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Flag.Enabled != false {
			t.Errorf("expected flag to be disabled")
		}

		// Now verify that GET /api/v1/search returns 503 FEATURE_DISABLED for guest
		searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12", nil)
		searchRec := httptest.NewRecorder()

		router.ServeHTTP(searchRec, searchReq)

		if searchRec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503 Service Unavailable, got %d: %s", searchRec.Code, searchRec.Body.String())
		}
		if !bytes.Contains(searchRec.Body.Bytes(), []byte("FEATURE_DISABLED")) {
			t.Errorf("expected FEATURE_DISABLED error code, got %s", searchRec.Body.String())
		}
	})

	// 4. PUT /api/v1/admin/feature-flags/invalid_key returns 404
	t.Run("PUT non-existent flag returns 404", func(t *testing.T) {
		body := []byte(`{"enabled": false}`)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feature-flags/non_existent_key", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	// 5. PUT /api/v1/admin/feature-flags/ff_multi_variant_search with invalid JSON returns 400
	t.Run("PUT invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feature-flags/ff_multi_variant_search", bytes.NewReader([]byte("{invalid-json")))
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

func TestFeatureFlags_WiredGuards_TableDriven(t *testing.T) {
	// 1. Promotions Engine Test
	t.Run("ff_promotions_engine blocks promo code when disabled", func(t *testing.T) {
		ffMgr := featureflag.NewMemoryManager(map[string]featureflag.Flag{
			"ff_quote_locking_engine": {
				Key:     "ff_quote_locking_engine",
				Enabled: true,
			},
			"ff_promotions_engine": {
				Key:     "ff_promotions_engine",
				Enabled: false,
			},
		})
		router := NewRouter(Deps{
			StaffAuth:   TestStaffVerifier(),
			FeatureFlag: ffMgr,
			Enforcer:    newTestEnforcerForFF(t),
		})

		body := `{"room_type_id":"01900000-0000-7000-8000-000000000001","check_in":"2026-10-10","check_out":"2026-10-12","promo_code":"OCTOBREAK"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("PROMOTIONS_DISABLED")) {
			t.Errorf("expected PROMOTIONS_DISABLED in error body, got %s", rec.Body.String())
		}
	})

	// 2. Front Desk Operations Flag Test
	t.Run("ff_front_desk_operations blocks roster when disabled", func(t *testing.T) {
		ffMgr := featureflag.NewMemoryManager(map[string]featureflag.Flag{
			"ff_front_desk_operations": {
				Key:     "ff_front_desk_operations",
				Enabled: false,
			},
		})
		router := NewRouter(Deps{
			StaffAuth:   TestStaffVerifier(),
			FeatureFlag: ffMgr,
			Enforcer:    newTestEnforcerForFF(t),
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/front-desk/daily-roster", nil)
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("FEATURE_DISABLED")) {
			t.Errorf("expected FEATURE_DISABLED in body, got %s", rec.Body.String())
		}
	})

	// 3. Stay Modification Flag Test
	t.Run("ff_stay_modification blocks room move when disabled", func(t *testing.T) {
		ffMgr := featureflag.NewMemoryManager(map[string]featureflag.Flag{
			"ff_stay_modification": {
				Key:     "ff_stay_modification",
				Enabled: false,
			},
		})
		router := NewRouter(Deps{
			StaffAuth:   TestStaffVerifier(),
			FeatureFlag: ffMgr,
			Enforcer:    newTestEnforcerForFF(t),
		})

		body := `{"target_room_number":"202","reason_category":"maintenance_defect","notes":"AC bocor"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/bk-test-1/room-move", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer gm_admin")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("FEATURE_DISABLED")) {
			t.Errorf("expected FEATURE_DISABLED in body, got %s", rec.Body.String())
		}
	})
}
