package handler

import (
	"net/http"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/gin-gonic/gin"
)

// Healthz menangani healthcheck probe liveness (GET/HEAD /healthz).
func Healthz(c *gin.Context) {
	middleware.WriteJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready menangani probe kesiapan runtime (GET/HEAD /ready).
func Ready(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ReadyCheck != nil {
			if err := d.ReadyCheck(c.Request.Context()); err != nil {
				middleware.WriteJSON(c, http.StatusServiceUnavailable, map[string]string{
					"status": "unavailable",
					"error":  err.Error(),
				})
				return
			}
		}
		env := "production"
		if d.IsDevelopment {
			env = "development"
		}
		payMode := "fake"
		if d.XenditGateway != nil {
			payMode = "xendit"
		}
		notifMode := d.NotifierMode
		if notifMode == "" {
			notifMode = "log"
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]string{
			"status":          "ready",
			"environment":     env,
			"payment_gateway": payMode,
			"notifier":        notifMode,
		})
	}
}
