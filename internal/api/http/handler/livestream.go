package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/platform/eventbus"
	"github.com/gin-gonic/gin"
)

// GuestBookingLiveStatus membuka koneksi HTTP Server-Sent Events (SSE) untuk memantau status
// pembayaran dan siklus hidup reservasi spesifik tamu (GET /api/v1/guest/bookings/:id/live-status).
func GuestBookingLiveStatus(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := middleware.GuestSessionFromContext(c.Request.Context())
		if sess == nil {
			middleware.WriteGuestError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tamu tidak ditemukan.")
			return
		}

		if d.GuestSvc == nil {
			middleware.WriteGuestError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Layanan tamu belum tersedia.")
			return
		}

		bookingID := c.Param("id")
		// Verifikasi kepemilikan reservasi untuk mencegah serangan IDOR
		_, err := d.GuestSvc.GetBookingReceipt(c.Request.Context(), sess.GuestEmail, bookingID)
		if err != nil {
			if errors.Is(err, guest.ErrBookingNotFound) {
				middleware.WriteGuestError(c, http.StatusNotFound, "BOOKING_NOT_FOUND", "Pemesanan tidak ditemukan atau Anda tidak memiliki akses.")
				return
			}
			// Jika ErrReceiptNotAvailable, tamu tetap pemilik sah yang sedang menunggu pembayaran
			if !errors.Is(err, guest.ErrReceiptNotAvailable) {
				middleware.WriteGuestError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Gagal memverifikasi kepemilikan pemesanan.")
				return
			}
		}

		if d.EventBus == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Event bus belum dikonfigurasi"})
			return
		}

		// Set header wajib SSE
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()

		subject := fmt.Sprintf("hospitality.booking.%s.*", bookingID)
		eventChan := make(chan *eventbus.EventMsg, 16)
		sub, err := d.EventBus.SubscribeChan(subject, eventChan)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		defer sub.Unsubscribe()

		ctx := c.Request.Context()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		// Kirim event sambungan awal
		c.SSEvent("connected", gin.H{
			"status":     "CONNECTED",
			"booking_id": bookingID,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
		c.Writer.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.SSEvent("ping", gin.H{"time": time.Now().UTC().Format(time.RFC3339)})
				c.Writer.Flush()
			case msg, ok := <-eventChan:
				if !ok {
					return
				}
				eventType := msg.EventType
				if eventType == "" {
					eventType = "message"
				}
				c.SSEvent(eventType, string(msg.Payload))
				c.Writer.Flush()
			}
		}
	}
}

// FrontDeskLiveStream membuka koneksi HTTP SSE operasional meja depan untuk staf resepsionis (GET /api/v1/front-desk/live-stream).
func FrontDeskLiveStream(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.EventBus == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Event bus belum dikonfigurasi"})
			return
		}

		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()

		// Resepsionis berlangganan ke seluruh subjek event perhotelan
		eventChan := make(chan *eventbus.EventMsg, 64)
		sub, err := d.EventBus.SubscribeChan("hospitality.>", eventChan)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		defer sub.Unsubscribe()

		ctx := c.Request.Context()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		c.SSEvent("connected", gin.H{
			"status":    "CONNECTED",
			"role":      "front_desk",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		c.Writer.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.SSEvent("ping", gin.H{"time": time.Now().UTC().Format(time.RFC3339)})
				c.Writer.Flush()
			case msg, ok := <-eventChan:
				if !ok {
					return
				}
				eventType := msg.EventType
				if eventType == "" {
					eventType = "message"
				}
				c.SSEvent(eventType, string(msg.Payload))
				c.Writer.Flush()
			}
		}
	}
}
