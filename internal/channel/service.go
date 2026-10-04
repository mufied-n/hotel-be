package channel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/platform/eventbus"
	"github.com/google/uuid"
)

// InventoryChecker antarmuka untuk mengecek ketersediaan kamar sebelum reservasi OTA dicatat.
type InventoryChecker interface {
	CheckAvailability(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms int) error
	CheckAvailabilityWithBuffer(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms, safetyBuffer int) error
}

// Service mengelola integrasi kanal OTA, validasi signature, dan koordinasi event hub.
type Service struct {
	store     Store
	inventory InventoryChecker
	bus       eventbus.Bus
}

func NewService(store Store, inventory InventoryChecker, bus eventbus.Bus) *Service {
	return &Service{
		store:     store,
		inventory: inventory,
		bus:       bus,
	}
}

// VerifySignature memverifikasi tanda tangan HMAC-SHA256 dari webhook OTA.
func (s *Service) VerifySignature(secret string, payload []byte, sigHeader string) bool {
	if secret == "" || len(payload) == 0 || sigHeader == "" {
		return false
	}

	var expectedSig string
	// Dukung format t=timestamp,v1=signature atau hex langsung
	if strings.Contains(sigHeader, "v1=") {
		parts := strings.Split(sigHeader, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "v1=") {
				expectedSig = strings.TrimPrefix(part, "v1=")
				break
			}
		}
	} else {
		expectedSig = strings.TrimSpace(sigHeader)
	}

	if expectedSig == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	computed := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computed), []byte(expectedSig))
}

// ProcessInboundEvent memproses webhook pemesanan masuk dari mitra OTA secara atomik dan idempoten.
func (s *Service) ProcessInboundEvent(ctx context.Context, req *InboundEventRequest, rawPayload []byte) error {
	if req == nil || req.Provider == "" || req.EventID == "" {
		return ErrInvalidEventPayload
	}

	// 1. Verifikasi mitra aktif
	partner, err := s.store.GetPartnerByCode(ctx, req.Provider)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPartnerNotFound, req.Provider)
	}

	// 2. Simpan ke kotak masuk event (idempoten per provider & event_id)
	inboxEvt := &EventInbox{
		ID:                uuid.New(),
		Provider:          req.Provider,
		EventID:           req.EventID,
		EventType:         req.EventType,
		ExternalReference: req.ExternalReference,
		Payload:           rawPayload,
		Status:            "PENDING",
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.store.SaveEventInbox(ctx, inboxEvt); err != nil {
		return err // ErrDuplicateEvent jika sudah pernah masuk
	}

	// 3. Validasi tanggal dan ketersediaan stok kamar
	checkIn, err := time.Parse("2006-01-02", req.CheckIn)
	if err != nil {
		_ = s.store.UpdateEventInboxStatus(ctx, inboxEvt.ID, "FAILED", "format check_in tidak valid")
		return fmt.Errorf("invalid check_in format: %w", err)
	}
	checkOut, err := time.Parse("2006-01-02", req.CheckOut)
	if err != nil {
		_ = s.store.UpdateEventInboxStatus(ctx, inboxEvt.ID, "FAILED", "format check_out tidak valid")
		return fmt.Errorf("invalid check_out format: %w", err)
	}

	numRooms := req.Rooms
	if numRooms <= 0 {
		numRooms = 1
	}

	var roomTypeUUID *uuid.UUID
	if req.RoomTypeID != "" {
		if parsed, err := uuid.Parse(req.RoomTypeID); err == nil {
			roomTypeUUID = &parsed
		}
	}

	// Cek inventaris dengan mempertimbangkan safety buffer mitra OTA
	if s.inventory != nil && req.RoomTypeID != "" {
		buffer := partner.SafetyBuffer
		if buffer < 0 {
			buffer = 0
		}
		if err := s.inventory.CheckAvailabilityWithBuffer(ctx, req.RoomTypeID, checkIn, checkOut, numRooms, buffer); err != nil {
			// Kuota tidak mencukupi / overbooking conflict!
			reason := fmt.Sprintf("Kamar penuh/kuota tidak mencukupi untuk %s s.d %s (safety buffer %d): %v", req.CheckIn, req.CheckOut, buffer, err)
			_ = s.store.UpdateEventInboxStatus(ctx, inboxEvt.ID, "QUARANTINED", reason)

			issue := &SyncIssue{
				Provider:          req.Provider,
				ExternalReference: req.ExternalReference,
				EventType:         req.EventType,
				RoomTypeID:        roomTypeUUID,
				Status:            StatusQuarantinedConflict,
				Reason:            reason,
				CreatedAt:         time.Now().UTC(),
			}
			_ = s.store.SaveSyncIssue(ctx, issue)

			// Siarkan alert darurat ke Front Desk via EventBus
			if s.bus != nil {
				conflictData, _ := json.Marshal(map[string]interface{}{
					"provider":           req.Provider,
					"external_reference": req.ExternalReference,
					"room_type_id":       req.RoomTypeID,
					"conflict_reason":    reason,
					"check_in":           req.CheckIn,
					"check_out":          req.CheckOut,
					"guest_name":         req.GuestName,
				})
				_ = s.bus.Publish(ctx, "hospitality.channel.conflict", "channel_conflict", req.EventID, conflictData)
			}

			return ErrAllotmentExhausted
		}
	}

	// 4. Status Sukses Diproses
	_ = s.store.UpdateEventInboxStatus(ctx, inboxEvt.ID, "PROCESSED", "")

	// 5. Publikasikan event ke bus terpusat NATS JetStream
	if s.bus != nil {
		channelSubject := fmt.Sprintf("hospitality.channel.%s.received", strings.ToLower(req.Provider))
		_ = s.bus.Publish(ctx, channelSubject, "channel_reservation_received", req.EventID, rawPayload)

		// Broadcast notifikasi booking ke Front Desk
		broadcastData, _ := json.Marshal(map[string]interface{}{
			"booking_id":  req.ExternalReference,
			"reference":   req.ExternalReference,
			"guest_name":  req.GuestName,
			"room_type":   req.RoomTypeID,
			"check_in":    req.CheckIn,
			"check_out":   req.CheckOut,
			"source":      req.Provider,
			"status":      "CONFIRMED",
			"total_minor": req.TotalPayoutIDR * 100,
		})
		_ = s.bus.Publish(ctx, "hospitality.booking.channel.created", "booking_created", req.EventID, broadcastData)
	}

	return nil
}

// ListSyncIssues mengembalikan daftar insiden karantina inventaris kanal.
func (s *Service) ListSyncIssues(ctx context.Context) ([]*SyncIssue, error) {
	return s.store.ListSyncIssues(ctx)
}

// GetSyncIssue mengambil rincian satu insiden karantina berdasarkan ID.
func (s *Service) GetSyncIssue(ctx context.Context, id uuid.UUID) (*SyncIssue, error) {
	return s.store.GetSyncIssue(ctx, id)
}

// ResolveSyncIssue menyelesaikan insiden karantina dengan aksi Complimentary Upgrade, Reject, atau Force Overbook.
func (s *Service) ResolveSyncIssue(ctx context.Context, issueID uuid.UUID, req *ResolveIssueRequest, staffUsername string) (*SyncIssue, error) {
	if req == nil {
		return nil, ErrInvalidAction
	}
	issue, err := s.store.GetSyncIssue(ctx, issueID)
	if err != nil {
		return nil, err
	}
	if issue.Status != StatusQuarantinedConflict {
		return nil, ErrIssueAlreadyResolved
	}

	var resolvedIssue *SyncIssue
	switch req.Action {
	case ActionComplimentaryUpgrade:
		if req.TargetRoomTypeID == "" {
			return nil, ErrMissingTargetRoomType
		}
		targetUUID, err := uuid.Parse(req.TargetRoomTypeID)
		if err != nil {
			return nil, fmt.Errorf("invalid target_room_type_id: %w", err)
		}

		// Temukan event inbox terkait untuk mengekstrak tanggal & jumlah kamar asli
		inboxEvt, err := s.store.GetEventInboxByReference(ctx, issue.Provider, issue.ExternalReference)
		if err != nil {
			return nil, fmt.Errorf("lookup original reservation: %w", err)
		}
		var checkIn, checkOut time.Time
		rooms := 1
		if inboxEvt != nil && len(inboxEvt.Payload) > 0 {
			var origReq InboundEventRequest
			if err := json.Unmarshal(inboxEvt.Payload, &origReq); err == nil {
				if parsedIn, err := time.Parse("2006-01-02", origReq.CheckIn); err == nil {
					checkIn = parsedIn
				}
				if parsedOut, err := time.Parse("2006-01-02", origReq.CheckOut); err == nil {
					checkOut = parsedOut
				}
				if origReq.Rooms > 0 {
					rooms = origReq.Rooms
				}
			}
		}
		if checkIn.IsZero() || checkOut.IsZero() {
			checkIn = time.Now().UTC()
			checkOut = checkIn.AddDate(0, 0, 1)
		}

		resolvedIssue, err = s.store.ResolveWithUpgrade(ctx, issueID, targetUUID, checkIn, checkOut, rooms, staffUsername, req.Notes)
		if err != nil {
			return nil, err
		}

	case ActionRejectAndCancel:
		resolvedIssue, err = s.store.ResolveSyncIssue(ctx, issueID, StatusRejected, nil, staffUsername, req.Notes)
		if err != nil {
			return nil, err
		}

	case ActionForceOverbookConfirmed:
		resolvedIssue, err = s.store.ResolveSyncIssue(ctx, issueID, StatusOverbookedOverridden, nil, staffUsername, req.Notes)
		if err != nil {
			return nil, err
		}

	default:
		return nil, ErrInvalidAction
	}

	// Publikasikan event ke bus jika tersedia
	if s.bus != nil && resolvedIssue != nil {
		eventData, _ := json.Marshal(map[string]interface{}{
			"issue_id":           resolvedIssue.ID,
			"provider":           resolvedIssue.Provider,
			"external_reference": resolvedIssue.ExternalReference,
			"status":             resolvedIssue.Status,
			"upgrade_room_type":  req.TargetRoomTypeID,
			"resolved_by":        staffUsername,
			"notes":              req.Notes,
		})
		_ = s.bus.Publish(ctx, "hospitality.channel.issue_resolved", "channel_issue_resolved", issueID.String(), eventData)
	}

	return resolvedIssue, nil
}

// GetPartner mengembalikan konfigurasi mitra berdasarkan provider_code.
func (s *Service) GetPartner(ctx context.Context, providerCode string) (*Partner, error) {
	return s.store.GetPartnerByCode(ctx, providerCode)
}
