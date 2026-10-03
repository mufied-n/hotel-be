package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/go-chi/chi/v5"
)

type updateFlagRequest struct {
	Enabled      *bool    `json:"enabled"`
	AllowedRoles []string `json:"allowed_roles"`
}

// handleAdminListFlags melayani GET /api/v1/admin/feature-flags (FR-FF-04).
func handleAdminListFlags(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FeatureFlag == nil {
			httpErrorCode(w, http.StatusNotImplemented, "feature flag manager not configured", "NOT_CONFIGURED")
			return
		}

		flags := d.FeatureFlag.List(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"total": len(flags),
			"flags": flags,
		})
	}
}

// handleAdminUpdateFlag melayani PUT /api/v1/admin/feature-flags/{key} (FR-FF-04).
func handleAdminUpdateFlag(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.FeatureFlag == nil {
			httpErrorCode(w, http.StatusNotImplemented, "feature flag manager not configured", "NOT_CONFIGURED")
			return
		}

		key := chi.URLParam(r, "key")
		if key == "" {
			httpErrorCode(w, http.StatusBadRequest, "key feature flag wajib disertakan", "INVALID_FLAG_KEY")
			return
		}

		var req updateFlagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpErrorCode(w, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		current, ok := d.FeatureFlag.Get(r.Context(), key)
		if !ok {
			httpErrorCode(w, http.StatusNotFound, "feature flag tidak ditemukan", "FLAG_NOT_FOUND")
			return
		}

		enabled := current.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		allowedRoles := req.AllowedRoles
		if allowedRoles == nil {
			allowedRoles = current.AllowedRoles
		}

		auth := GetAuthContext(r.Context())
		actor := auth.Subject
		if actor == "" || actor == "anonymous" {
			actor = "admin"
		}

		updated, err := d.FeatureFlag.Update(r.Context(), key, enabled, allowedRoles, actor)
		if errors.Is(err, featureflag.ErrFlagNotFound) {
			httpErrorCode(w, http.StatusNotFound, "feature flag tidak ditemukan", "FLAG_NOT_FOUND")
			return
		}
		if err != nil {
			httpErrorCode(w, http.StatusInternalServerError, "gagal memperbarui feature flag: "+err.Error(), "INTERNAL_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "updated",
			"flag":   updated,
		})
	}
}
