package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/channel"
	"github.com/example/hotel-booking/internal/platform/eventbus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockInventoryForHandler struct {
	available bool
}

func (m *mockInventoryForHandler) CheckAvailability(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms int) error {
	if !m.available {
		return errors.New("sold out")
	}
	return nil
}

func (m *mockInventoryForHandler) CheckAvailabilityWithBuffer(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time, rooms, safetyBuffer int) error {
	if !m.available {
		return errors.New("sold out or safety buffer reached")
	}
	return nil
}

func signPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestHandleChannelWebhook_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "trv_secret_webhook_signature_key_2026"
	validReq := channel.InboundEventRequest{
		Provider:          "TRAVELOKA",
		EventID:           "test-evt-001",
		EventType:         "reservation_created",
		ExternalReference: "TRV-001",
		RoomTypeID:        "01900000-0000-7000-8000-000000000001",
		CheckIn:           "2026-10-10",
		CheckOut:          "2026-10-12",
		Rooms:             1,
		GuestName:         "Budi",
		TotalPayoutIDR:    1000000,
	}
	validBody, _ := json.Marshal(validReq)
	validSig := signPayload(secret, validBody)

	tests := []struct {
		name       string
		provider   string
		sig        string
		body       []byte
		invAvail   bool
		channelNil bool
		wantStatus int
	}{
		{
			name:       "Nil channel service returns 501",
			channelNil: true,
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "Missing X-Channel-Provider header returns 400",
			provider:   "",
			sig:        validSig,
			body:       validBody,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Unknown provider returns 401",
			provider:   "UNKNOWN_PROVIDER",
			sig:        validSig,
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Missing X-Channel-Signature header returns 401",
			provider:   "TRAVELOKA",
			sig:        "",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Invalid signature returns 401",
			provider:   "TRAVELOKA",
			sig:        "invalid_hex_signature_abc123",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Invalid JSON payload returns 400",
			provider:   "TRAVELOKA",
			sig:        signPayload(secret, []byte(`{invalid json`)),
			body:       []byte(`{invalid json`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Valid webhook returns 202 Accepted",
			provider:   "TRAVELOKA",
			sig:        validSig,
			body:       validBody,
			invAvail:   true,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "Duplicate webhook returns 200 OK with DUPLICATE_ACCEPTED",
			provider:   "TRAVELOKA",
			sig:        validSig,
			body:       validBody,
			invAvail:   true,
			wantStatus: http.StatusOK, // Second time in sequence
		},
	}

	store := channel.NewMemoryStore()
	bus := eventbus.NewMemoryBus()
	defer bus.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chanSvc *channel.Service
			if !tt.channelNil {
				chanSvc = channel.NewService(store, &mockInventoryForHandler{available: tt.invAvail}, bus)
			}

			deps := Deps{
				ChannelSvc: chanSvc,
			}

			r := gin.New()
			r.POST("/channel-events", HandleChannelWebhook(deps))

			req := httptest.NewRequest(http.MethodPost, "/channel-events", bytes.NewReader(tt.body))
			if tt.provider != "" {
				req.Header.Set("X-Channel-Provider", tt.provider)
			}
			if tt.sig != "" {
				req.Header.Set("X-Channel-Signature", tt.sig)
			}
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleChannelWebhook_AllotmentConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "trv_secret_webhook_signature_key_2026"
	validReq := channel.InboundEventRequest{
		Provider:          "TRAVELOKA",
		EventID:           "evt-conflict-99",
		EventType:         "reservation_created",
		ExternalReference: "TRV-CONFLICT-99",
		RoomTypeID:        "01900000-0000-7000-8000-000000000001",
		CheckIn:           "2026-10-10",
		CheckOut:          "2026-10-12",
	}
	body, _ := json.Marshal(validReq)
	sig := signPayload(secret, body)

	store := channel.NewMemoryStore()
	bus := eventbus.NewMemoryBus()
	defer bus.Close()

	// Inventory NOT available -> triggers 409
	chanSvc := channel.NewService(store, &mockInventoryForHandler{available: false}, bus)
	deps := Deps{ChannelSvc: chanSvc}

	r := gin.New()
	r.POST("/channel-events", HandleChannelWebhook(deps))

	req := httptest.NewRequest(http.MethodPost, "/channel-events", bytes.NewReader(body))
	req.Header.Set("X-Channel-Provider", "TRAVELOKA")
	req.Header.Set("X-Channel-Signature", sig)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}
}

func TestHandleListChannelSyncIssues_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := channel.NewMemoryStore()
	_ = store.SaveSyncIssue(context.Background(), &channel.SyncIssue{
		Provider: "AGODA",
		Reason:   "Allotment conflict test",
	})
	chanSvc := channel.NewService(store, nil, nil)

	r := gin.New()
	r.GET("/issues", HandleListChannelSyncIssues(Deps{ChannelSvc: chanSvc}))

	req := httptest.NewRequest(http.MethodGet, "/issues", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Test nil service
	r2 := gin.New()
	r2.GET("/issues", HandleListChannelSyncIssues(Deps{ChannelSvc: nil}))
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, req)
	if w2.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w2.Code)
	}
}

func TestHandleGetChannelPartner_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := channel.NewMemoryStore()
	chanSvc := channel.NewService(store, nil, nil)
	deps := Deps{ChannelSvc: chanSvc}

	r := gin.New()
	r.GET("/partners/:code", HandleGetChannelPartner(deps))

	// Found
	req := httptest.NewRequest(http.MethodGet, "/partners/TRAVELOKA", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Not Found
	req2 := httptest.NewRequest(http.MethodGet, "/partners/NON_EXISTENT", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w2.Code)
	}

	// Nil service
	r3 := gin.New()
	r3.GET("/partners/:code", HandleGetChannelPartner(Deps{ChannelSvc: nil}))
	w3 := httptest.NewRecorder()
	r3.ServeHTTP(w3, req)
	if w3.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w3.Code)
	}
}

func TestHandleResolveChannelSyncIssue_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		issueID      string
		body         string
		nilService   bool
		wantStatus   int
		wantCode     string
	}{
		{
			name:       "Complimentary upgrade success",
			issueID:    "019234a5-c999-7000-8000-000000000001",
			body:       `{"action":"COMPLIMENTARY_UPGRADE","target_room_type_id":"01900000-0000-7000-8000-000000000002","notes":"Upgraded"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Nil service returns 501",
			issueID:    "019234a5-c999-7000-8000-000000000001",
			body:       `{"action":"REJECT_AND_CANCEL"}`,
			nilService: true,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "NOT_IMPLEMENTED",
		},
		{
			name:       "Invalid UUID parameter returns 400",
			issueID:    "invalid-uuid-format",
			body:       `{"action":"REJECT_AND_CANCEL"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ID",
		},
		{
			name:       "Invalid JSON body returns 400",
			issueID:    "019234a5-c999-7000-8000-000000000001",
			body:       `{invalid-json}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "Missing target_room_type_id returns 400",
			issueID:    "019234a5-c999-7000-8000-000000000002",
			body:       `{"action":"COMPLIMENTARY_UPGRADE"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MISSING_TARGET_ROOM_TYPE",
		},
		{
			name:       "Invalid action returns 400",
			issueID:    "019234a5-c999-7000-8000-000000000002",
			body:       `{"action":"SOME_UNKNOWN_ACTION"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ACTION",
		},
		{
			name:       "Not found issue returns 404",
			issueID:    "019234a5-c999-7000-8000-000000000099",
			body:       `{"action":"REJECT_AND_CANCEL"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "Reject and cancel success",
			issueID:    "019234a5-c999-7000-8000-000000000003",
			body:       `{"action":"REJECT_AND_CANCEL","notes":"Rejected"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Force overbook confirmed success",
			issueID:    "019234a5-c999-7000-8000-000000000004",
			body:       `{"action":"FORCE_OVERBOOK_CONFIRMED","notes":"Approved by GM"}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := channel.NewMemoryStore()
			bus := eventbus.NewMemoryBus()
			defer bus.Close()

			// Pre-populate issues for happy paths
			issueUUID, _ := uuid.Parse(tt.issueID)
			if tt.wantStatus == http.StatusOK || tt.wantCode == "MISSING_TARGET_ROOM_TYPE" || tt.wantCode == "INVALID_ACTION" {
				_ = store.SaveSyncIssue(context.Background(), &channel.SyncIssue{
					ID:                issueUUID,
					Provider:          "AGODA",
					ExternalReference: "EXT-1001",
					EventType:         "reservation_created",
					Status:            channel.StatusQuarantinedConflict,
					Reason:            "sold out",
				})
			}

			var chanSvc *channel.Service
			if !tt.nilService {
				chanSvc = channel.NewService(store, nil, bus)
			}

			r := gin.New()
			r.POST("/issues/:id/resolve", HandleResolveChannelSyncIssue(Deps{ChannelSvc: chanSvc}))

			req := httptest.NewRequest(http.MethodPost, "/issues/"+tt.issueID+"/resolve", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantCode != "" {
				var resp map[string]interface{}
				_ = json.Unmarshal(w.Body.Bytes(), &resp)
				if resp["code"] != tt.wantCode {
					t.Errorf("expected error code %s, got %v", tt.wantCode, resp["code"])
				}
			}
		})
	}
}

