package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/platform/featureflag"
)

func newTestEnforcerForFF(t *testing.T) *casbin.SyncedEnforcer {
	t.Helper()
	policies := [][]string{
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"p", "guest", "/api/v1/search", "GET"},
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

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

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
			middleware := RequireFeature(ffMgr, tc.flagKey)
			handler := middleware(okHandler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.role != "" {
				ctx := context.WithValue(req.Context(), RoleKey, tc.role)
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

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
	ffMgr := featureflag.NewMemoryManager(nil) // default 17 flags
	enforcer := newTestEnforcerForFF(t)

	deps := Deps{
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
			Total int                  `json:"total"`
			Flags []featureflag.Flag   `json:"flags"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Total != 17 {
			t.Errorf("expected total 17 flags, got %d", resp.Total)
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
			"enabled": disabled,
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
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
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
