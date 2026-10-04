package channel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/platform/eventbus"
	"github.com/google/uuid"
)

type mockInventoryChecker struct {
	available bool
}

func (m *mockInventoryChecker) CheckAvailability(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms int) error {
	if !m.available {
		return errors.New("rooms sold out")
	}
	return nil
}

func (m *mockInventoryChecker) CheckAvailabilityWithBuffer(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms, safetyBuffer int) error {
	if !m.available {
		return errors.New("rooms sold out or buffer exhausted")
	}
	return nil
}

func computeHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestService_VerifySignature_TableDriven(t *testing.T) {
	secret := "my_test_secret_123"
	payload := []byte(`{"event":"booking_created"}`)
	validSig := computeHMAC(secret, payload)

	tests := []struct {
		name      string
		secret    string
		payload   []byte
		sigHeader string
		wantValid bool
	}{
		{
			name:      "Valid raw hex signature",
			secret:    secret,
			payload:   payload,
			sigHeader: validSig,
			wantValid: true,
		},
		{
			name:      "Valid formatted t=...,v1=... signature",
			secret:    secret,
			payload:   payload,
			sigHeader: "t=1728000000,v1=" + validSig,
			wantValid: true,
		},
		{
			name:      "Invalid hex signature",
			secret:    secret,
			payload:   payload,
			sigHeader: "invalid_sig_hex_123456",
			wantValid: false,
		},
		{
			name:      "Empty secret",
			secret:    "",
			payload:   payload,
			sigHeader: validSig,
			wantValid: false,
		},
		{
			name:      "Empty payload",
			secret:    secret,
			payload:   []byte{},
			sigHeader: validSig,
			wantValid: false,
		},
		{
			name:      "Empty signature header",
			secret:    secret,
			payload:   payload,
			sigHeader: "",
			wantValid: false,
		},
	}

	svc := NewService(NewMemoryStore(), nil, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.VerifySignature(tt.secret, tt.payload, tt.sigHeader)
			if got != tt.wantValid {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestService_ProcessInboundEvent_TableDriven(t *testing.T) {
	store := NewMemoryStore()
	bus := eventbus.NewMemoryBus()
	defer bus.Close()

	tests := []struct {
		name           string
		req            *InboundEventRequest
		invAvailable   bool
		wantErr        error
		expectConflict bool
	}{
		{
			name: "Successful inbound event processing",
			req: &InboundEventRequest{
				Provider:          "TRAVELOKA",
				EventID:           "trv-evt-1001",
				EventType:         "reservation_created",
				ExternalReference: "TRV-8812",
				RoomTypeID:        "01900000-0000-7000-8000-000000000001",
				CheckIn:           "2026-10-10",
				CheckOut:          "2026-10-12",
				Rooms:             1,
				GuestName:         "Budi Santoso",
				GuestEmail:        "budi@example.com",
				TotalPayoutIDR:    1500000,
			},
			invAvailable:   true,
			wantErr:        nil,
			expectConflict: false,
		},
		{
			name: "Duplicate event ID returns ErrDuplicateEvent",
			req: &InboundEventRequest{
				Provider:          "TRAVELOKA",
				EventID:           "trv-evt-1001", // duplicate!
				EventType:         "reservation_created",
				ExternalReference: "TRV-8812",
				RoomTypeID:        "01900000-0000-7000-8000-000000000001",
				CheckIn:           "2026-10-10",
				CheckOut:          "2026-10-12",
			},
			invAvailable: true,
			wantErr:      ErrDuplicateEvent,
		},
		{
			name: "Unknown partner returns ErrPartnerNotFound",
			req: &InboundEventRequest{
				Provider: "UNKNOWN_OTA",
				EventID:  "unk-1002",
			},
			invAvailable: true,
			wantErr:      ErrPartnerNotFound,
		},
		{
			name: "Nil request returns ErrInvalidEventPayload",
			req:  nil,
			wantErr: ErrInvalidEventPayload,
		},
		{
			name: "Invalid check_in date returns error",
			req: &InboundEventRequest{
				Provider: "AGODA",
				EventID:  "agd-1003",
				CheckIn:  "invalid-date",
				CheckOut: "2026-10-12",
			},
			invAvailable: true,
			wantErr:      errors.New("invalid check_in format"),
		},
		{
			name: "Invalid check_out date returns error",
			req: &InboundEventRequest{
				Provider: "AGODA",
				EventID:  "agd-1004",
				CheckIn:  "2026-10-10",
				CheckOut: "invalid-date",
			},
			invAvailable: true,
			wantErr:      errors.New("invalid check_out format"),
		},
		{
			name: "Allotment exhausted triggers quarantine and returns ErrAllotmentExhausted",
			req: &InboundEventRequest{
				Provider:          "AGODA",
				EventID:           "agd-1005",
				EventType:         "reservation_created",
				ExternalReference: "AGD-9988",
				RoomTypeID:        "01900000-0000-7000-8000-000000000002",
				CheckIn:           "2026-10-20",
				CheckOut:          "2026-10-22",
				Rooms:             2,
				GuestName:         "Jessica Mila",
			},
			invAvailable:   false,
			wantErr:        ErrAllotmentExhausted,
			expectConflict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := &mockInventoryChecker{available: tt.invAvailable}
			svc := NewService(store, inv, bus)

			var rawPayload []byte
			if tt.req != nil {
				rawPayload = []byte(`{"raw":true}`)
			}

			ctx := context.Background()
			err := svc.ProcessInboundEvent(ctx, tt.req, rawPayload)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && tt.wantErr.Error() != "invalid check_in format" && tt.wantErr.Error() != "invalid check_out format" {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tt.expectConflict {
				issues, err := svc.ListSyncIssues(ctx)
				if err != nil {
					t.Fatalf("ListSyncIssues error: %v", err)
				}
				if len(issues) == 0 {
					t.Errorf("expected conflict issue recorded, got 0 issues")
				}
			}
		})
	}
}

func TestService_GetPartner(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	p, err := svc.GetPartner(ctx, "TRAVELOKA")
	if err != nil {
		t.Fatalf("GetPartner failed: %v", err)
	}
	if p.ProviderCode != "TRAVELOKA" {
		t.Errorf("expected TRAVELOKA, got %s", p.ProviderCode)
	}

	_, err = svc.GetPartner(ctx, "NON_EXISTENT")
	if !errors.Is(err, ErrPartnerNotFound) {
		t.Errorf("expected ErrPartnerNotFound, got %v", err)
	}
}

func TestService_Concurrency(t *testing.T) {
	store := NewMemoryStore()
	bus := eventbus.NewMemoryBus()
	defer bus.Close()
	svc := NewService(store, &mockInventoryChecker{available: true}, bus)

	ctx := context.Background()
	var wg sync.WaitGroup
	numEvents := 25

	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			req := &InboundEventRequest{
				Provider:          "TRAVELOKA",
				EventID:           hex.EncodeToString([]byte{byte(idx)}),
				EventType:         "reservation_created",
				ExternalReference: hex.EncodeToString([]byte{byte(idx)}),
				RoomTypeID:        "01900000-0000-7000-8000-000000000001",
				CheckIn:           "2026-10-10",
				CheckOut:          "2026-10-12",
				Rooms:             1,
			}
			_ = svc.ProcessInboundEvent(ctx, req, []byte(`{}`))
		}()
	}
	wg.Wait()
}

func TestService_ResolveSyncIssue_TableDriven(t *testing.T) {
	ctx := context.Background()
	targetRoomType := "01900000-0000-7000-8000-000000000002"

	tests := []struct {
		name          string
		setupIssue    *SyncIssue
		req           *ResolveIssueRequest
		staff         string
		wantErr       error
		wantStatus    string
		checkResolved bool
	}{
		{
			name: "Complimentary upgrade success",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000001"),
				Provider:          "AGODA",
				ExternalReference: "AGD-1001",
				EventType:         "reservation_created",
				Status:            StatusQuarantinedConflict,
				Reason:            "rooms sold out",
			},
			req: &ResolveIssueRequest{
				Action:           ActionComplimentaryUpgrade,
				TargetRoomTypeID: targetRoomType,
				Notes:            "Upgraded by Front Desk",
			},
			staff:         "receptionist_rini",
			wantErr:       nil,
			wantStatus:    StatusResolved,
			checkResolved: true,
		},
		{
			name: "Complimentary upgrade missing target room type",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000002"),
				Provider:          "AGODA",
				ExternalReference: "AGD-1002",
				EventType:         "reservation_created",
				Status:            StatusQuarantinedConflict,
				Reason:            "rooms sold out",
			},
			req: &ResolveIssueRequest{
				Action: ActionComplimentaryUpgrade,
			},
			staff:   "receptionist_rini",
			wantErr: ErrMissingTargetRoomType,
		},
		{
			name: "Reject and cancel success",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000003"),
				Provider:          "TRAVELOKA",
				ExternalReference: "TRV-1003",
				EventType:         "reservation_created",
				Status:            StatusQuarantinedConflict,
				Reason:            "overbooking",
			},
			req: &ResolveIssueRequest{
				Action: ActionRejectAndCancel,
				Notes:  "Cannot accommodate",
			},
			staff:      "receptionist_budi",
			wantErr:    nil,
			wantStatus: StatusRejected,
		},
		{
			name: "Force overbook confirmed success",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000004"),
				Provider:          "AGODA",
				ExternalReference: "AGD-1004",
				EventType:         "reservation_created",
				Status:            StatusQuarantinedConflict,
				Reason:            "overbooking",
			},
			req: &ResolveIssueRequest{
				Action: ActionForceOverbookConfirmed,
				Notes:  "GM approved emergency management room",
			},
			staff:      "gm_hendra",
			wantErr:    nil,
			wantStatus: StatusOverbookedOverridden,
		},
		{
			name: "Invalid action fails",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000005"),
				Provider:          "AGODA",
				ExternalReference: "AGD-1005",
				EventType:         "reservation_created",
				Status:            StatusQuarantinedConflict,
			},
			req: &ResolveIssueRequest{
				Action: "UNKNOWN_ACTION",
			},
			staff:   "receptionist_rini",
			wantErr: ErrInvalidAction,
		},
		{
			name: "Already resolved issue returns ErrIssueAlreadyResolved",
			setupIssue: &SyncIssue{
				ID:                uuid.MustParse("019234a5-c999-7000-8000-000000000006"),
				Provider:          "AGODA",
				ExternalReference: "AGD-1006",
				EventType:         "reservation_created",
				Status:            StatusResolved,
			},
			req: &ResolveIssueRequest{
				Action: ActionRejectAndCancel,
			},
			staff:   "receptionist_rini",
			wantErr: ErrIssueAlreadyResolved,
		},
		{
			name: "Issue not found",
			setupIssue: &SyncIssue{
				ID: uuid.MustParse("019234a5-c999-7000-8000-000000000007"),
			},
			req: &ResolveIssueRequest{
				Action: ActionRejectAndCancel,
			},
			staff:   "receptionist_rini",
			wantErr: ErrIssueNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			bus := eventbus.NewMemoryBus()
			defer bus.Close()
			svc := NewService(store, nil, bus)

			targetID := tt.setupIssue.ID
			if tt.name != "Issue not found" {
				_ = store.SaveSyncIssue(ctx, tt.setupIssue)
			}

			resolved, err := svc.ResolveSyncIssue(ctx, targetID, tt.req, tt.staff)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resolved.Status != tt.wantStatus {
				t.Errorf("expected status %s, got %s", tt.wantStatus, resolved.Status)
			}
			if resolved.ResolvedBy != tt.staff {
				t.Errorf("expected resolved_by %s, got %s", tt.staff, resolved.ResolvedBy)
			}
			if tt.checkResolved {
				if resolved.UpgradeRoomTypeID == nil || resolved.UpgradeRoomTypeID.String() != targetRoomType {
					t.Errorf("expected upgrade_room_type_id %s, got %v", targetRoomType, resolved.UpgradeRoomTypeID)
				}
			}
		})
	}
}

