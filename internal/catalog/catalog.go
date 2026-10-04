// Package catalog mengelola informasi kamar hotel, varian jual, fasilitas, dan foto
// untuk properti Pulang ke Uttara (desain arsitektur §2, BE-G01).
package catalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"uuid"
)

// Photo merepresentasikan aset foto kamar dan teks alternatif untuk aksesibilitas.
type Photo struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

// RoomVariant merepresentasikan varian kamar jual resmi Pulang ke Uttara (BE-G01).
// Membedakan family konten (misal: Superior) dengan sellable room variant fisik (King vs Twin).
type RoomVariant struct {
	ID             string   `json:"id"`
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	FamilyName     string   `json:"family_name"`
	BedType        string   `json:"bed_type"`
	RoomSizeSqm    int      `json:"room_size_sqm"`
	MaxCapacity    int      `json:"max_capacity"`
	MaxAdults      int      `json:"max_adults"`
	MaxChildren    int      `json:"max_children"`
	Description    string   `json:"description"`
	BasePriceMinor int64    `json:"base_price_minor"`
	Amenities      []string `json:"amenities"`
	Photos         []Photo  `json:"photos"`
}

// ErrVariantNotFound dikembalikan bila varian kamar tidak ditemukan.
var (
	ErrVariantNotFound = errors.New("catalog: room variant not found")
	ErrDuplicateCode   = errors.New("catalog: room variant code already exists")
	ErrInvalidVariant  = errors.New("catalog: invalid room variant data")
	ErrCannotDelete    = errors.New("catalog: cannot delete variant in active use")
)

const (
	// MaxPhotosPerVariant membatasi jumlah foto agar payload dan halaman tetap ringan.
	MaxPhotosPerVariant = 20
	// MaxPhotoAltLen membatasi panjang teks alternatif foto.
	MaxPhotoAltLen = 200
)

// ValidateVariant memeriksa data varian kamar dan foto. Foto hanya boleh berupa
// path lokal "/asset/rooms/..." (tanpa "..") atau URL https absolut.
func ValidateVariant(v RoomVariant) error {
	if strings.TrimSpace(v.Code) == "" || strings.TrimSpace(v.Name) == "" || v.MaxCapacity < 1 || v.BasePriceMinor <= 0 {
		return ErrInvalidVariant
	}
	if len(v.Photos) > MaxPhotosPerVariant {
		return ErrInvalidVariant
	}
	for _, p := range v.Photos {
		url := strings.TrimSpace(p.URL)
		local := strings.HasPrefix(url, "/asset/rooms/") && !strings.Contains(url, "..") && !strings.Contains(url, "\\")
		remote := strings.HasPrefix(url, "https://") && len(url) > len("https://")
		if !local && !remote {
			return ErrInvalidVariant
		}
		if len([]rune(p.Alt)) > MaxPhotoAltLen {
			return ErrInvalidVariant
		}
	}
	return nil
}

// Store mendefinisikan port data untuk membaca dan mengelola katalog kamar.
type Store interface {
	ListVariants(ctx context.Context) ([]RoomVariant, error)
	GetVariant(ctx context.Context, idOrCode string) (RoomVariant, error)
	CreateVariant(ctx context.Context, v RoomVariant) (RoomVariant, error)
	UpdateVariant(ctx context.Context, id string, v RoomVariant) (RoomVariant, error)
	DeleteVariant(ctx context.Context, id string) error
}

// MemoryStore mengimplementasikan Store murni in-memory untuk pengujian unit cepat (anti-overengineering / Ponytail).
type MemoryStore struct {
	mu       sync.RWMutex
	variants []RoomVariant
}

// NewMemoryStore membuat instance MemoryStore baru.
func NewMemoryStore(variants []RoomVariant) *MemoryStore {
	cp := make([]RoomVariant, len(variants))
	copy(cp, variants)
	return &MemoryStore{variants: cp}
}

func (m *MemoryStore) ListVariants(_ context.Context) ([]RoomVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]RoomVariant, len(m.variants))
	copy(out, m.variants)
	return out, nil
}

func (m *MemoryStore) GetVariant(_ context.Context, idOrCode string) (RoomVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.variants {
		if v.ID == idOrCode || v.Code == idOrCode {
			return v, nil
		}
	}
	if idOrCode == "std" && len(m.variants) > 0 {
		return m.variants[0], nil
	}
	return RoomVariant{}, ErrVariantNotFound
}

func (m *MemoryStore) CreateVariant(_ context.Context, v RoomVariant) (RoomVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ValidateVariant(v); err != nil {
		return RoomVariant{}, err
	}
	for _, existing := range m.variants {
		if strings.EqualFold(existing.Code, v.Code) {
			return RoomVariant{}, ErrDuplicateCode
		}
	}
	if v.ID == "" {
		v.ID = uuid.NewV7().String()
	}
	if v.Amenities == nil {
		v.Amenities = []string{}
	}
	if v.Photos == nil {
		v.Photos = []Photo{}
	}
	m.variants = append(m.variants, v)
	return v, nil
}

func (m *MemoryStore) UpdateVariant(_ context.Context, id string, v RoomVariant) (RoomVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ValidateVariant(v); err != nil {
		return RoomVariant{}, err
	}
	idx := -1
	for i, existing := range m.variants {
		if existing.ID == id || existing.Code == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return RoomVariant{}, ErrVariantNotFound
	}
	for i, existing := range m.variants {
		if i != idx && strings.EqualFold(existing.Code, v.Code) {
			return RoomVariant{}, ErrDuplicateCode
		}
	}
	v.ID = m.variants[idx].ID // preserve internal ID
	if v.Amenities == nil {
		v.Amenities = []string{}
	}
	if v.Photos == nil {
		v.Photos = []Photo{}
	}
	m.variants[idx] = v
	return v, nil
}

func (m *MemoryStore) DeleteVariant(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := -1
	for i, existing := range m.variants {
		if existing.ID == id || existing.Code == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrVariantNotFound
	}
	m.variants = append(m.variants[:idx], m.variants[idx+1:]...)
	return nil
}
