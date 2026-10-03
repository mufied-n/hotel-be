package handler

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/gin-gonic/gin"
)

type updateFlagRequest struct {
	Enabled      *bool    `json:"enabled"`
	AllowedRoles []string `json:"allowed_roles"`
}

// AdminListFlags melayani GET /api/v1/admin/feature-flags (FR-FF-04).
func AdminListFlags(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FeatureFlag == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "feature flag manager not configured", "NOT_CONFIGURED")
			return
		}

		flags := d.FeatureFlag.List(c.Request.Context())
		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"total": len(flags),
			"flags": flags,
		})
	}
}

// AdminUpdateFlag melayani PUT /api/v1/admin/feature-flags/{key} (FR-FF-04).
func AdminUpdateFlag(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.FeatureFlag == nil {
			middleware.HttpErrorCode(c, http.StatusNotImplemented, "feature flag manager not configured", "NOT_CONFIGURED")
			return
		}

		key := c.Param("key")
		if key == "" {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "key feature flag wajib disertakan", "INVALID_FLAG_KEY")
			return
		}

		var req updateFlagRequest
		if err := json.UnmarshalRead(c.Request.Body, &req); err != nil {
			middleware.HttpErrorCode(c, http.StatusBadRequest, "body JSON tidak valid", "INVALID_JSON")
			return
		}

		current, ok := d.FeatureFlag.Get(c.Request.Context(), key)
		if !ok {
			middleware.HttpErrorCode(c, http.StatusNotFound, "feature flag tidak ditemukan", "FLAG_NOT_FOUND")
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

		auth := middleware.GetAuthContext(c.Request.Context())
		actor := auth.Subject
		if actor == "" || actor == "anonymous" {
			actor = "admin"
		}

		updated, err := d.FeatureFlag.Update(c.Request.Context(), key, enabled, allowedRoles, actor)
		if errors.Is(err, featureflag.ErrFlagNotFound) {
			middleware.HttpErrorCode(c, http.StatusNotFound, "feature flag tidak ditemukan", "FLAG_NOT_FOUND")
			return
		}
		if err != nil {
			middleware.HttpErrorCode(c, http.StatusInternalServerError, "gagal memperbarui feature flag: "+err.Error(), "INTERNAL_ERROR")
			return
		}

		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"status": "updated",
			"flag":   updated,
		})
	}
}
