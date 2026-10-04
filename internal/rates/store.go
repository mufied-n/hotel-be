package rates

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryQuoteStore implementasi thread-safe in-memory quote repository dengan masa berlaku 15 menit.
type MemoryQuoteStore struct {
	mu     sync.RWMutex
	quotes map[string]LockedQuote
	ttl    time.Duration
}

// NewMemoryQuoteStore membuat instance baru MemoryQuoteStore.
func NewMemoryQuoteStore(ttl time.Duration) *MemoryQuoteStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &MemoryQuoteStore{
		quotes: make(map[string]LockedQuote),
		ttl:    ttl,
	}
}

// SaveQuote menyimpan penawaran harga ke dalam in-memory map.
func (s *MemoryQuoteStore) SaveQuote(_ context.Context, q LockedQuote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotes[q.ID] = q
	return nil
}

// GetQuote mengambil penawaran harga berdasarkan ID dengan validasi kedaluwarsa.
func (s *MemoryQuoteStore) GetQuote(_ context.Context, id string) (LockedQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.quotes[id]
	if !ok {
		return LockedQuote{}, ErrQuoteNotFound
	}
	if time.Now().After(q.ExpiresAt) {
		return LockedQuote{}, ErrQuoteExpired
	}
	return q, nil
}

// MemoryCalendarStore implementasi thread-safe in-memory untuk RateCalendarStore.
type MemoryCalendarStore struct {
	mu        sync.RWMutex
	overrides map[string]CalendarOverride // key: "roomTypeID:date:planCode"
}

// NewMemoryCalendarStore membuat instance baru MemoryCalendarStore.
func NewMemoryCalendarStore() *MemoryCalendarStore {
	return &MemoryCalendarStore{
		overrides: make(map[string]CalendarOverride),
	}
}

// GetCalendar mengambil daftar override kalender in-memory.
func (m *MemoryCalendarStore) GetCalendar(_ context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]CalendarOverride, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if planCode == "" {
		planCode = "RO"
	}

	var results []CalendarOverride
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		for _, o := range m.overrides {
			if o.Date.Format("2006-01-02") == dateStr && o.RatePlanCode == planCode {
				if roomTypeID == nil || *roomTypeID == o.RoomTypeID {
					results = append(results, o)
				}
			}
		}
	}

	return results, nil
}

// BulkUpsertOverrides memodifikasi override kalender in-memory secara massal.
func (m *MemoryCalendarStore) BulkUpsertOverrides(_ context.Context, req BulkCalendarUpdateRequest) (int64, error) {
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return 0, fmt.Errorf("invalid start_date format (expected YYYY-MM-DD): %w", err)
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return 0, fmt.Errorf("invalid end_date format (expected YYYY-MM-DD): %w", err)
	}
	if endDate.Before(startDate) {
		return 0, fmt.Errorf("end_date must be greater than or equal to start_date")
	}

	if req.RatePlanCode == "" {
		req.RatePlanCode = "RO"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var affected int64
	for _, roomID := range req.RoomTypeIDs {
		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			dateKey := d.Format("2006-01-02")
			mapKey := fmt.Sprintf("%s:%s:%s", roomID, dateKey, req.RatePlanCode)

			existing, ok := m.overrides[mapKey]
			if !ok {
				existing = CalendarOverride{
					RoomTypeID:   roomID,
					RatePlanCode: req.RatePlanCode,
					Date:         d,
					MinLOS:       1,
					MaxLOS:       30,
				}
			}

			if req.PriceOverrideIDR != nil {
				existing.PriceOverrideIDR = req.PriceOverrideIDR
				existing.EffectivePrice = *req.PriceOverrideIDR
			}
			if req.IsStopSell != nil {
				existing.IsStopSell = *req.IsStopSell
			}
			if req.IsCTA != nil {
				existing.IsCTA = *req.IsCTA
			}
			if req.IsCTD != nil {
				existing.IsCTD = *req.IsCTD
			}
			if req.MinLOS != nil {
				existing.MinLOS = *req.MinLOS
			}
			if req.MaxLOS != nil {
				existing.MaxLOS = *req.MaxLOS
			}
			if req.AllotmentLimit != nil {
				existing.AllotmentLimit = req.AllotmentLimit
			}

			m.overrides[mapKey] = existing
			affected++
		}
	}

	return affected, nil
}

// MemoryPromoStore implementasi thread-safe in-memory untuk PromoStore.
type MemoryPromoStore struct {
	mu        sync.RWMutex
	campaigns map[string]PromoCampaign
}

// NewMemoryPromoStore membuat instance baru MemoryPromoStore.
func NewMemoryPromoStore() *MemoryPromoStore {
	return &MemoryPromoStore{
		campaigns: make(map[string]PromoCampaign),
	}
}

// GetByCode mengambil kampanye promo dari in-memory store.
func (m *MemoryPromoStore) GetByCode(_ context.Context, code string) (PromoCampaign, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	code = strings.TrimSpace(strings.ToUpper(code))
	c, ok := m.campaigns[code]
	if !ok {
		return PromoCampaign{}, ErrPromoNotFound
	}
	return c, nil
}

// ListCampaigns mengambil seluruh daftar promo.
func (m *MemoryPromoStore) ListCampaigns(_ context.Context) ([]PromoCampaign, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []PromoCampaign
	for _, c := range m.campaigns {
		list = append(list, c)
	}
	return list, nil
}

// CreateCampaign menyimpan promo campaign baru.
func (m *MemoryPromoStore) CreateCampaign(_ context.Context, p PromoCampaign) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	code := strings.TrimSpace(strings.ToUpper(p.Code))
	if p.ID == "" {
		p.ID = "promo-" + code
	}
	p.Code = code
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	m.campaigns[code] = p
	return p.ID, nil
}

// UpdateCampaign memperbarui status promo in-memory.
func (m *MemoryPromoStore) UpdateCampaign(_ context.Context, id string, isActive bool, quotaTotal int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for code, c := range m.campaigns {
		if c.ID == id {
			c.IsActive = isActive
			if quotaTotal >= c.QuotaUsed {
				c.QuotaTotal = quotaTotal
			}
			c.UpdatedAt = time.Now()
			m.campaigns[code] = c
			return nil
		}
	}
	return ErrPromoNotFound
}

// ReserveQuotaAtomic mengurangi kuota promo secara atomik.
func (m *MemoryPromoStore) ReserveQuotaAtomic(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	code = strings.TrimSpace(strings.ToUpper(code))
	c, ok := m.campaigns[code]
	if !ok {
		return ErrPromoNotFound
	}
	if !c.IsActive || time.Now().Before(c.ValidFrom) || time.Now().After(c.ValidTo) {
		return ErrPromoExpired
	}
	if c.QuotaUsed >= c.QuotaTotal {
		return ErrPromoQuotaExhausted
	}
	c.QuotaUsed++
	c.UpdatedAt = time.Now()
	m.campaigns[code] = c
	return nil
}

// ReleaseQuotaAtomic mengembalikan kuota promo in-memory.
func (m *MemoryPromoStore) ReleaseQuotaAtomic(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	code = strings.TrimSpace(strings.ToUpper(code))
	c, ok := m.campaigns[code]
	if !ok {
		return ErrPromoNotFound
	}
	if c.QuotaUsed > 0 {
		c.QuotaUsed--
		c.UpdatedAt = time.Now()
		m.campaigns[code] = c
	}
	return nil
}
