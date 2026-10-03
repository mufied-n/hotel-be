package featureflag

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var (
	ErrFlagNotFound = errors.New("feature flag not found")
)

type contextKey string

const (
	roleContextKey contextKey = "feature_flag_role"
)

// WithRole menyematkan role ke dalam context untuk evaluasi feature flag.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleContextKey, role)
}

// RoleFromContext mengekstrak role dari context.
func RoleFromContext(ctx context.Context) string {
	if r, ok := ctx.Value(roleContextKey).(string); ok && r != "" {
		return r
	}
	// Fallback jika role diset menggunakan string literal "user_role"
	if r, ok := ctx.Value("user_role").(string); ok && r != "" {
		return r
	}
	return ""
}

// Flag merepresentasikan konfigurasi satu feature flag.
type Flag struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Enabled      bool      `json:"enabled"`
	AllowedRoles []string  `json:"allowed_roles"` // Kosong = berlaku untuk semua role
	UpdatedBy    string    `json:"updated_by"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Snapshot adalah map flag in-memory yang di-swap secara atomic.
type Snapshot map[string]Flag

// Manager mendefinisikan kontrak interface engine feature flag.
type Manager interface {
	IsEnabled(ctx context.Context, key string) bool
	Get(ctx context.Context, key string) (Flag, bool)
	List(ctx context.Context) []Flag
	Update(ctx context.Context, key string, enabled bool, allowedRoles []string, updatedBy string) (Flag, error)
	Reload(ctx context.Context) error
	Close() error
}

// DefaultFlags mengembalikan daftar 17 feature flags resmi Pulang ke Uttara.
func DefaultFlags() map[string]Flag {
	now := time.Now().UTC()
	return map[string]Flag{
		"ff_catalog_write": {
			Key:          "ff_catalog_write",
			Name:         "Catalog Mutation CRUD",
			Description:  "Mengizinkan operasi POST, PUT, DELETE pada katalog kamar",
			Enabled:      true,
			AllowedRoles: []string{"revenue_mgr", "gm_admin"},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_multi_variant_search": {
			Key:          "ff_multi_variant_search",
			Name:         "Multi-Variant Room Search",
			Description:  "Pencarian multi-kamar dan multi-varian kontinu",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_quote_locking_engine": {
			Key:          "ff_quote_locking_engine",
			Name:         "Quote Locking Engine",
			Description:  "Kuotasi harga terkunci ber-TTL 15 menit",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_promotions_engine": {
			Key:          "ff_promotions_engine",
			Name:         "Promo Code Discounts",
			Description:  "Aplikasi diskon promo kamar",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_checkout_idempotency": {
			Key:          "ff_checkout_idempotency",
			Name:         "Checkout Idempotency Guard",
			Description:  "Deduplikasi transaksi checkout via Idempotency-Key",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_pii_masking_guard": {
			Key:          "ff_pii_masking_guard",
			Name:         "Guest PII Data Masking",
			Description:  "Masking data pribadi tamu pada respons publik",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_strict_cancellation_policy": {
			Key:          "ff_strict_cancellation_policy",
			Name:         "Strict Cancellation Rules",
			Description:  "Penegakan kebijakan pembatalan non-refundable dan cutoff 48 jam",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_xendit_payment_gateway": {
			Key:          "ff_xendit_payment_gateway",
			Name:         "Xendit Invoice Gateway",
			Description:  "Integrasi pembuatan invoice dan webhook Xendit",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_resend_email_notifier": {
			Key:          "ff_resend_email_notifier",
			Name:         "Resend Email Notifier",
			Description:  "Pengiriman email konfirmasi dan OTP via Resend",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_guest_portal_auth": {
			Key:          "ff_guest_portal_auth",
			Name:         "Guest Passwordless Auth",
			Description:  "Permintaan challenge dan verifikasi OTP sesi tamu",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_guest_my_bookings": {
			Key:          "ff_guest_my_bookings",
			Name:         "Guest My Bookings Portal",
			Description:  "Akses daftar reservasi privat milik tamu",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_booking_artifacts_receipt": {
			Key:          "ff_booking_artifacts_receipt",
			Name:         "Printable Invoice Receipt",
			Description:  "Penerbitan kuitansi resmi berformat INV/PKU/...",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_booking_artifacts_icalendar": {
			Key:          "ff_booking_artifacts_icalendar",
			Name:         "iCalendar RFC 5545 Sync",
			Description:  "Generasi berkas kalender .ics menginap",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_finance_reconciliation": {
			Key:          "ff_finance_reconciliation",
			Name:         "Finance Cases & Summary",
			Description:  "Pencatatan sengketa pembayaran dan ringkasan rekonsiliasi",
			Enabled:      true,
			AllowedRoles: []string{"finance", "gm_admin"},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_gateway_automated_refund": {
			Key:          "ff_gateway_automated_refund",
			Name:         "Gateway Automated Refund",
			Description:  "Eksekusi pengembalian dana otomatis via Xendit API",
			Enabled:      true,
			AllowedRoles: []string{"finance", "gm_admin"},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_housekeeping_board": {
			Key:          "ff_housekeeping_board",
			Name:         "Housekeeping Room Board",
			Description:  "Pemantauan status kebersihan 95 kamar hotel",
			Enabled:      true,
			AllowedRoles: []string{"housekeeping", "receptionist", "gm_admin"},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
		"ff_room_readiness_checkin_guard": {
			Key:          "ff_room_readiness_checkin_guard",
			Name:         "Check-In Room Readiness",
			Description:  "Memvalidasi kamar berstatus inspected sebelum check-in",
			Enabled:      true,
			AllowedRoles: []string{},
			UpdatedBy:    "system",
			UpdatedAt:    now,
		},
	}
}

// MemoryManager adalah implementasi in-memory lock-free Manager (cocok untuk pengujian unit & fallback).
type MemoryManager struct {
	snapshot atomic.Pointer[Snapshot]
}

// NewMemoryManager membuat instance MemoryManager dengan data awal.
func NewMemoryManager(initial map[string]Flag) *MemoryManager {
	m := &MemoryManager{}
	snap := make(Snapshot)
	if initial != nil {
		for k, v := range initial {
			snap[k] = v
		}
	} else {
		snap = DefaultFlags()
	}
	m.snapshot.Store(&snap)
	return m
}

// IsEnabled mengevaluasi status feature flag berdasarkan snapshot RAM dan context caller.
func (m *MemoryManager) IsEnabled(ctx context.Context, key string) bool {
	snapPtr := m.snapshot.Load()
	if snapPtr == nil {
		return false
	}
	snap := *snapPtr
	f, ok := snap[key]
	if !ok {
		return false
	}

	// 1. Cek toggle global
	if !f.Enabled {
		return false
	}

	// 2. Cek filter peran (jika kosong = berlaku untuk semua)
	if len(f.AllowedRoles) == 0 {
		return true
	}

	// 3. Verifikasi role pemanggil
	callerRole := RoleFromContext(ctx)
	if callerRole == "" {
		return false
	}
	for _, r := range f.AllowedRoles {
		if r == callerRole {
			return true
		}
	}
	return false
}

// Get mengambil detail konfigurasi satu flag.
func (m *MemoryManager) Get(_ context.Context, key string) (Flag, bool) {
	snapPtr := m.snapshot.Load()
	if snapPtr == nil {
		return Flag{}, false
	}
	snap := *snapPtr
	f, ok := snap[key]
	return f, ok
}

// List mengambil seluruh daftar konfigurasi flag.
func (m *MemoryManager) List(_ context.Context) []Flag {
	snapPtr := m.snapshot.Load()
	if snapPtr == nil {
		return nil
	}
	snap := *snapPtr
	res := make([]Flag, 0, len(snap))
	for _, f := range snap {
		res = append(res, f)
	}
	return res
}

// Update memperbarui konfigurasi flag di memori.
func (m *MemoryManager) Update(_ context.Context, key string, enabled bool, allowedRoles []string, updatedBy string) (Flag, error) {
	snapPtr := m.snapshot.Load()
	if snapPtr == nil {
		return Flag{}, ErrFlagNotFound
	}
	snap := *snapPtr
	f, ok := snap[key]
	if !ok {
		return Flag{}, fmt.Errorf("%w: %s", ErrFlagNotFound, key)
	}

	// Salin copy-on-write
	newSnap := make(Snapshot, len(snap))
	for k, v := range snap {
		newSnap[k] = v
	}

	f.Enabled = enabled
	if allowedRoles != nil {
		f.AllowedRoles = allowedRoles
	}
	f.UpdatedBy = updatedBy
	f.UpdatedAt = time.Now().UTC()

	newSnap[key] = f
	m.snapshot.Store(&newSnap)
	return f, nil
}

// Reload adalah no-op pada MemoryManager.
func (m *MemoryManager) Reload(_ context.Context) error {
	return nil
}

// Close adalah no-op pada MemoryManager.
func (m *MemoryManager) Close() error {
	return nil
}
