package channel

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store mendefinisikan kontrak persistensi untuk integrasi kanal OTA.
type Store interface {
	GetPartnerByCode(ctx context.Context, code string) (*Partner, error)
	SaveEventInbox(ctx context.Context, evt *EventInbox) error
	GetEventInbox(ctx context.Context, provider, eventID string) (*EventInbox, error)
	GetEventInboxByReference(ctx context.Context, provider, externalRef string) (*EventInbox, error)
	UpdateEventInboxStatus(ctx context.Context, id uuid.UUID, status, reason string) error
	SaveSyncIssue(ctx context.Context, issue *SyncIssue) error
	GetSyncIssue(ctx context.Context, id uuid.UUID) (*SyncIssue, error)
	ListSyncIssues(ctx context.Context) ([]*SyncIssue, error)
	ResolveSyncIssue(ctx context.Context, id uuid.UUID, status string, upgradeRoomTypeID *uuid.UUID, resolvedBy, notes string) (*SyncIssue, error)
	ResolveWithUpgrade(ctx context.Context, id uuid.UUID, targetRoomTypeID uuid.UUID, checkIn, checkOut time.Time, rooms int, resolvedBy, notes string) (*SyncIssue, error)
}

// MemoryStore adalah implementasi in-memory thread-safe dari Store untuk pengujian.
type MemoryStore struct {
	mu       sync.RWMutex
	partners map[string]*Partner
	inbox    map[string]*EventInbox // key: provider:eventID
	issues   []*SyncIssue
}

// NewMemoryStore membuat instance MemoryStore baru dengan seed default.
func NewMemoryStore() *MemoryStore {
	now := time.Now().UTC()
	return &MemoryStore{
		partners: map[string]*Partner{
			"TRAVELOKA": {
				ID:            uuid.MustParse("01900000-0000-7000-8000-000000000001"),
				ProviderCode:  "TRAVELOKA",
				Name:          "Traveloka Indonesia",
				WebhookSecret: "trv_secret_webhook_signature_key_2026",
				SafetyBuffer:  1,
				IsActive:      true,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			"AGODA": {
				ID:            uuid.MustParse("01900000-0000-7000-8000-000000000002"),
				ProviderCode:  "AGODA",
				Name:          "Agoda Global Partner",
				WebhookSecret: "agd_secret_webhook_signature_key_2026",
				SafetyBuffer:  1,
				IsActive:      true,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
		inbox: make(map[string]*EventInbox),
	}
}

func (m *MemoryStore) GetPartnerByCode(ctx context.Context, code string) (*Partner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.partners[code]
	if !ok || !p.IsActive {
		return nil, ErrPartnerNotFound
	}
	return p, nil
}

func (m *MemoryStore) SaveEventInbox(ctx context.Context, evt *EventInbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := evt.Provider + ":" + evt.EventID
	if _, exists := m.inbox[key]; exists {
		return ErrDuplicateEvent
	}
	if evt.ID == uuid.Nil {
		evt.ID = uuid.New()
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	m.inbox[key] = evt
	return nil
}

func (m *MemoryStore) GetEventInbox(ctx context.Context, provider, eventID string) (*EventInbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := provider + ":" + eventID
	evt, ok := m.inbox[key]
	if !ok {
		return nil, nil
	}
	return evt, nil
}

func (m *MemoryStore) GetEventInboxByReference(ctx context.Context, provider, externalRef string) (*EventInbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, evt := range m.inbox {
		if evt.Provider == provider && evt.ExternalReference == externalRef {
			return evt, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) UpdateEventInboxStatus(ctx context.Context, id uuid.UUID, status, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, evt := range m.inbox {
		if evt.ID == id {
			evt.Status = status
			evt.ErrorReason = reason
			evt.ProcessedAt = &now
			return nil
		}
	}
	return nil
}

func (m *MemoryStore) SaveSyncIssue(ctx context.Context, issue *SyncIssue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue.ID == uuid.Nil {
		issue.ID = uuid.New()
	}
	if issue.CreatedAt.IsZero() {
		issue.CreatedAt = time.Now().UTC()
	}
	m.issues = append(m.issues, issue)
	return nil
}

func (m *MemoryStore) GetSyncIssue(ctx context.Context, id uuid.UUID) (*SyncIssue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, i := range m.issues {
		if i.ID == id {
			return i, nil
		}
	}
	return nil, ErrIssueNotFound
}

func (m *MemoryStore) ListSyncIssues(ctx context.Context) ([]*SyncIssue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*SyncIssue, len(m.issues))
	copy(res, m.issues)
	return res, nil
}

func (m *MemoryStore) ResolveSyncIssue(ctx context.Context, id uuid.UUID, status string, upgradeRoomTypeID *uuid.UUID, resolvedBy, notes string) (*SyncIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, i := range m.issues {
		if i.ID == id {
			if i.Status != StatusQuarantinedConflict {
				return nil, ErrIssueAlreadyResolved
			}
			i.Status = status
			i.UpgradeRoomTypeID = upgradeRoomTypeID
			i.ResolvedBy = resolvedBy
			i.ResolvedAt = &now
			i.Notes = notes
			return i, nil
		}
	}
	return nil, ErrIssueNotFound
}

func (m *MemoryStore) ResolveWithUpgrade(ctx context.Context, id uuid.UUID, targetRoomTypeID uuid.UUID, checkIn, checkOut time.Time, rooms int, resolvedBy, notes string) (*SyncIssue, error) {
	return m.ResolveSyncIssue(ctx, id, StatusResolved, &targetRoomTypeID, resolvedBy, notes)
}
