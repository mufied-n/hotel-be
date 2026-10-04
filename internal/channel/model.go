package channel

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPartnerNotFound        = errors.New("channel: partner not found")
	ErrInvalidSignature       = errors.New("channel: invalid webhook signature")
	ErrDuplicateEvent         = errors.New("channel: duplicate event already processed")
	ErrAllotmentExhausted     = errors.New("channel: room allotment exhausted for requested dates")
	ErrInvalidEventPayload    = errors.New("channel: invalid event payload")
	ErrIssueNotFound          = errors.New("channel: sync issue not found")
	ErrIssueAlreadyResolved   = errors.New("channel: sync issue already resolved")
	ErrInvalidAction          = errors.New("channel: invalid resolution action")
	ErrTargetRoomUnavailable  = errors.New("channel: target room type unavailable for upgrade")
	ErrMissingTargetRoomType  = errors.New("channel: target_room_type_id is required for upgrade")
)

const (
	ActionComplimentaryUpgrade    = "COMPLIMENTARY_UPGRADE"
	ActionRejectAndCancel         = "REJECT_AND_CANCEL"
	ActionForceOverbookConfirmed  = "FORCE_OVERBOOK_CONFIRMED"

	StatusQuarantinedConflict = "QUARANTINED_CONFLICT"
	StatusResolved            = "RESOLVED"
	StatusRejected            = "REJECTED"
	StatusOverbookedOverridden = "OVERBOOKED_OVERRIDDEN"
)

// Partner merepresentasikan konfigurasi mitra OTA / Channel Manager.
type Partner struct {
	ID            uuid.UUID `json:"id"`
	ProviderCode  string    `json:"provider_code"`
	Name          string    `json:"name"`
	WebhookSecret string    `json:"-"`
	SafetyBuffer  int       `json:"safety_buffer"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EventInbox mencatat setiap webhook masuk dari mitra OTA secara idempoten.
type EventInbox struct {
	ID                uuid.UUID  `json:"id"`
	Provider          string     `json:"provider"`
	EventID           string     `json:"event_id"`
	EventType         string     `json:"event_type"`
	ExternalReference string     `json:"external_reference"`
	Payload           []byte     `json:"payload"`
	Status            string     `json:"status"` // PENDING, PROCESSED, FAILED, QUARANTINED
	ErrorReason       string     `json:"error_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
}

// SyncIssue mencatat insiden konflik inventaris atau anomali kuota kanal.
type SyncIssue struct {
	ID                 uuid.UUID  `json:"id"`
	Provider           string     `json:"provider"`
	ExternalReference  string     `json:"external_reference"`
	EventType          string     `json:"event_type"`
	RoomTypeID         *uuid.UUID `json:"room_type_id,omitempty"`
	Status             string     `json:"status"` // QUARANTINED_CONFLICT, RESOLVED, REJECTED, OVERBOOKED_OVERRIDDEN
	Reason             string     `json:"reason"`
	UpgradeRoomTypeID  *uuid.UUID `json:"upgrade_room_type_id,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	ResolvedBy         string     `json:"resolved_by,omitempty"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// ResolveIssueRequest adalah DTO permintaan penyelesaian insiden karantina sinkronisasi kanal.
type ResolveIssueRequest struct {
	Action           string `json:"action"` // COMPLIMENTARY_UPGRADE, REJECT_AND_CANCEL, FORCE_OVERBOOK_CONFIRMED
	TargetRoomTypeID string `json:"target_room_type_id,omitempty"`
	Notes            string `json:"notes,omitempty"`
}


// InboundEventRequest DTO payload permintaan webhook dari mitra OTA.
type InboundEventRequest struct {
	Provider          string    `json:"provider"`
	EventID           string    `json:"event_id"`
	EventType         string    `json:"event_type"`
	ExternalReference string    `json:"external_reference"`
	RoomTypeID        string    `json:"room_type_id"`
	CheckIn           string    `json:"check_in"`
	CheckOut          string    `json:"check_out"`
	Rooms             int       `json:"rooms"`
	GuestName         string    `json:"guest_name"`
	GuestEmail        string    `json:"guest_email"`
	GuestPhone        string    `json:"guest_phone"`
	TotalPayoutIDR    int64     `json:"total_payout_idr"`
}
