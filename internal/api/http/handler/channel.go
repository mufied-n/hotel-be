package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/channel"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HandleChannelWebhook menangani notifikasi webhook reservasi masuk dari mitra OTA (POST /api/v1/channel-events).
func HandleChannelWebhook(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ChannelSvc == nil {
			c.JSON(http.StatusNotImplemented, gin.H{
				"code":    "NOT_IMPLEMENTED",
				"message": "Layanan integrasi kanal belum dikonfigurasi.",
			})
			return
		}

		provider := c.GetHeader("X-Channel-Provider")
		if provider == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "MISSING_HEADER",
				"message": "Header X-Channel-Provider wajib disertakan.",
			})
			return
		}

		partner, err := d.ChannelSvc.GetPartner(c.Request.Context(), provider)
		if err != nil {
			if errors.Is(err, channel.ErrPartnerNotFound) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    "UNKNOWN_PROVIDER",
					"message": "Mitra kanal tidak dikenal atau tidak aktif.",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Gagal memverifikasi mitra kanal.",
			})
			return
		}

		sigHeader := c.GetHeader("X-Channel-Signature")
		if sigHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "MISSING_SIGNATURE",
				"message": "Header X-Channel-Signature wajib disertakan.",
			})
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "INVALID_BODY",
				"message": "Gagal membaca body permintaan.",
			})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Validasi tanda tangan digital HMAC-SHA256
		if !d.ChannelSvc.VerifySignature(partner.WebhookSecret, bodyBytes, sigHeader) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "INVALID_SIGNATURE",
				"message": "Tanda tangan webhook HMAC tidak valid.",
			})
			return
		}

		var req channel.InboundEventRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "INVALID_JSON",
				"message": "Format JSON webhook tidak valid.",
			})
			return
		}
		req.Provider = provider

		err = d.ChannelSvc.ProcessInboundEvent(c.Request.Context(), &req, bodyBytes)
		if err != nil {
			if errors.Is(err, channel.ErrDuplicateEvent) {
				// Respons idempoten: tetap 200 OK agar OTA tidak retry berulang
				c.JSON(http.StatusOK, gin.H{
					"status":   "DUPLICATE_ACCEPTED",
					"event_id": req.EventID,
					"message":  "Event duplikat telah diterima sebelumnya.",
				})
				return
			}
			if errors.Is(err, channel.ErrAllotmentExhausted) {
				c.JSON(http.StatusConflict, gin.H{
					"code":     "ALLOTMENT_EXHAUSTED",
					"event_id": req.EventID,
					"message":  "Kuota kamar penuh untuk rentang tanggal yang diminta. Event telah dikarantina.",
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "PROCESS_ERROR",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status":   "ACCEPTED",
			"event_id": req.EventID,
			"message":  "Event kanal berhasil diterima dan diproses.",
		})
	}
}

// HandleListChannelSyncIssues mengembalikan daftar insiden karantina inventaris kanal (GET /api/v1/staff/channel-sync-issues).
func HandleListChannelSyncIssues(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ChannelSvc == nil {
			c.JSON(http.StatusNotImplemented, gin.H{
				"code":    "NOT_IMPLEMENTED",
				"message": "Layanan kanal belum aktif.",
			})
			return
		}

		issues, err := d.ChannelSvc.ListSyncIssues(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Gagal mengambil daftar insiden sinkronisasi.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"issues": issues,
		})
	}
}

// HandleGetChannelPartner mengembalikan informasi mitra kanal (GET /api/v1/staff/channel-partners/:code).
func HandleGetChannelPartner(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ChannelSvc == nil {
			c.JSON(http.StatusNotImplemented, gin.H{
				"code":    "NOT_IMPLEMENTED",
				"message": "Layanan kanal belum aktif.",
			})
			return
		}

		code := c.Param("code")
		partner, err := d.ChannelSvc.GetPartner(c.Request.Context(), code)
		if err != nil {
			if errors.Is(err, channel.ErrPartnerNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"code":    "NOT_FOUND",
					"message": "Mitra kanal tidak ditemukan.",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Gagal memuat data mitra kanal.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"partner": partner,
		})
	}
}

// HandleResolveChannelSyncIssue mengeksekusi resolusi insiden karantina kanal (POST /api/v1/staff/channel-sync-issues/:id/resolve).
func HandleResolveChannelSyncIssue(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ChannelSvc == nil {
			c.JSON(http.StatusNotImplemented, gin.H{
				"code":    "NOT_IMPLEMENTED",
				"message": "Layanan integrasi kanal belum dikonfigurasi.",
			})
			return
		}

		idStr := c.Param("id")
		issueID, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "INVALID_ID",
				"message": "Format issue ID tidak valid.",
			})
			return
		}

		var req channel.ResolveIssueRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Payload JSON resolusi tidak valid.",
			})
			return
		}

		ac := middleware.GetAuthContext(c.Request.Context())
		staffUsername := strings.TrimPrefix(ac.Subject, "staff:")
		if staffUsername == "" {
			staffUsername = "staff"
		}

		resolvedIssue, err := d.ChannelSvc.ResolveSyncIssue(c.Request.Context(), issueID, &req, staffUsername)
		if err != nil {
			switch {
			case errors.Is(err, channel.ErrIssueNotFound):
				c.JSON(http.StatusNotFound, gin.H{
					"code":    "NOT_FOUND",
					"message": "Insiden karantina tidak ditemukan.",
				})
			case errors.Is(err, channel.ErrIssueAlreadyResolved):
				c.JSON(http.StatusConflict, gin.H{
					"code":    "ALREADY_RESOLVED",
					"message": "Insiden karantina sudah diselesaikan sebelumnya.",
				})
			case errors.Is(err, channel.ErrInvalidAction):
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    "INVALID_ACTION",
					"message": "Aksi resolusi tidak valid. Gunakan COMPLIMENTARY_UPGRADE, REJECT_AND_CANCEL, atau FORCE_OVERBOOK_CONFIRMED.",
				})
			case errors.Is(err, channel.ErrMissingTargetRoomType):
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    "MISSING_TARGET_ROOM_TYPE",
					"message": "target_room_type_id wajib diisi untuk aksi COMPLIMENTARY_UPGRADE.",
				})
			case errors.Is(err, channel.ErrTargetRoomUnavailable):
				c.JSON(http.StatusConflict, gin.H{
					"code":    "TARGET_ROOM_UNAVAILABLE",
					"message": "Kamar pada tipe target tidak mencukupi untuk upgrade.",
				})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    "INTERNAL_ERROR",
					"message": err.Error(),
				})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "SUCCESS",
			"message": "Insiden karantina berhasil diselesaikan.",
			"issue":   resolvedIssue,
		})
	}
}

