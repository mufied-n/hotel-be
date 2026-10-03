# Technical Architecture & Design — Feature Flags & Runtime Configuration System
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen ID:** `TECH-FEATURE-FLAGS-2026-10-03`
- **Versi:** 1.0.0
- **Tanggal:** 2026-10-03
- **Status:** Approved / In Execution
- **PRD Pasangan:** [`docs/prd/feature-flags-system-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/feature-flags-system-2026-10-03.md)
- **SRS Pasangan:** [`docs/srs/feature-flags-system-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/feature-flags-system-2026-10-03.md)

---

## 1. Arsitektur Komponen & Diagram Alur

Sistem feature flag dirancang menggunakan arsitektur **In-Memory Lock-Free Snapshot** dengan persistensi PostgreSQL dan sinkronisasi hybrid:

```mermaid
flowchart TD
    subgraph Storage["1. Persistence & Coordination Tier"]
        DB[("PostgreSQL 18\ntabel feature_flags")]
        PubSub[("Valkey 8 / Redis\nChannel: pku:feature_flags:updated")]
    end

    subgraph PlatformLayer["2. Platform Feature Flag Engine (internal/platform/featureflag)"]
        Manager["featureflag.Manager\n(Store & Coordinator)"]
        AtomicSnap[("atomic.Pointer[Snapshot]\n(RAM Lock-Free Cache)")]
        SyncWorker["Background Sync Goroutine\n(Ticker 30s + Valkey Subscriber)"]
    end

    subgraph TransportLayer["3. Transport Layer (internal/api)"]
        Middleware["RequireFeature(ff, key)\nChi Middleware"]
        AdminHandler["handleAdminListFlags()\nhandleAdminUpdateFlag()"]
        OtherHandlers["searchRooms(), createBooking(), dsb."]
    end

    subgraph CallerContext["4. Context & Identity"]
        Ctx["r.Context()\n(AuthContext.Role dari IdentifySubject)"]
    end

    DB -->|Cold start read| Manager
    Manager -->|Initial load| AtomicSnap
    SyncWorker -->|Reload on event / tick| Manager
    PubSub -.->|Instant message| SyncWorker
    AdminHandler -->|PUT update flag| DB
    AdminHandler -->|Publish event| PubSub
    Middleware -->|IsEnabled(ctx, key)| Manager
    Manager -->|Atomic load| AtomicSnap
    Ctx -.->|Provides user role| Manager
    Middleware -->|Allowed| OtherHandlers
```

---

## 2. Struktur Data & Model Go Idiomatis

### Package `internal/platform/featureflag`

```go
package featureflag

import (
	"context"
	"sync/atomic"
	"time"
)

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

// Snapshot adalah map in-memory yang di-swap secara atomic.
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
```

---

## 3. Logika Evaluasi Bertingkat (Lock-Free Hot Path)

Evaluasi dilakukan murni pada CPU register dan pointer RAM:

```go
func (s *Service) IsEnabled(ctx context.Context, key string) bool {
	snapPtr := s.snapshot.Load()
	if snapPtr == nil {
		return false // Fail-closed
	}
	snap := *snapPtr
	f, ok := snap[key]
	if !ok {
		return false // Flag tidak dikenal -> tolak
	}

	// 1. Cek toggle global
	if !f.Enabled {
		return false
	}

	// 2. Cek pembatasan role jika ada
	if len(f.AllowedRoles) == 0 {
		return true // Berlaku untuk semua
	}

	// 3. Ekstraksi role dari context
	callerRole := extractRoleFromContext(ctx)
	for _, r := range f.AllowedRoles {
		if r == callerRole {
			return true
		}
	}
	return false
}
```

---

## 4. Skema Database & Goose Migration (`00013_feature_flags.sql`)

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS feature_flags (
    key           VARCHAR(64) PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    description   TEXT NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    allowed_roles TEXT[] NOT NULL DEFAULT '{}',
    updated_by    VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed 17 Feature Flags Resmi Pulang ke Uttara
INSERT INTO feature_flags (key, name, description, enabled, allowed_roles) VALUES
('ff_catalog_write', 'Catalog Mutation CRUD', 'Mengizinkan operasi POST, PUT, DELETE pada katalog kamar', TRUE, ARRAY['revenue_mgr', 'gm_admin']),
('ff_multi_variant_search', 'Multi-Variant Room Search', 'Pencarian multi-kamar dan multi-varian kontinu', TRUE, '{}'),
('ff_quote_locking_engine', 'Quote Locking Engine', 'Kuotasi harga terkunci ber-TTL 15 menit', TRUE, '{}'),
('ff_promotions_engine', 'Promo Code Discounts', 'Aplikasi diskon promo kamar', TRUE, '{}'),
('ff_checkout_idempotency', 'Checkout Idempotency Guard', 'Deduplikasi transaksi checkout via Idempotency-Key', TRUE, '{}'),
('ff_pii_masking_guard', 'Guest PII Data Masking', 'Masking data pribadi tamu pada respons publik', TRUE, '{}'),
('ff_strict_cancellation_policy', 'Strict Cancellation Rules', 'Penegakan kebijakan pembatalan non-refundable dan cutoff 48 jam', TRUE, '{}'),
('ff_xendit_payment_gateway', 'Xendit Invoice Gateway', 'Integrasi pembuatan invoice dan webhook Xendit', TRUE, '{}'),
('ff_resend_email_notifier', 'Resend Email Notifier', 'Pengiriman email konfirmasi dan OTP via Resend', TRUE, '{}'),
('ff_guest_portal_auth', 'Guest Passwordless Auth', 'Permintaan challenge dan verifikasi OTP sesi tamu', TRUE, '{}'),
('ff_guest_my_bookings', 'Guest My Bookings Portal', 'Akses daftar reservasi privat milik tamu', TRUE, '{}'),
('ff_booking_artifacts_receipt', 'Printable Invoice Receipt', 'Penerbitan kuitansi resmi berformat INV/PKU/...', TRUE, '{}'),
('ff_booking_artifacts_icalendar', 'iCalendar RFC 5545 Sync', 'Generasi berkas kalender .ics menginap', TRUE, '{}'),
('ff_finance_reconciliation', 'Finance Cases & Summary', 'Pencatatan sengketa pembayaran dan ringkasan rekonsiliasi', TRUE, ARRAY['finance', 'gm_admin']),
('ff_gateway_automated_refund', 'Gateway Automated Refund', 'Eksekusi pengembalian dana otomatis via Xendit API', TRUE, ARRAY['finance', 'gm_admin']),
('ff_housekeeping_board', 'Housekeeping Room Board', 'Pemantauan status kebersihan 95 kamar hotel', TRUE, ARRAY['housekeeping', 'receptionist', 'gm_admin']),
('ff_room_readiness_checkin_guard', 'Check-In Room Readiness', 'Memvalidasi kamar berstatus inspected sebelum check-in', TRUE, '{}')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS feature_flags;
```

---

## 5. Analisis Anti-Overengineering (Ponytail Audit)

1. **No External Microservice / Sidecar:** Tidak menggunakan Unleash / LaunchDarkly server yang memakan port dan resource terpisah.
2. **Standard Library Synchronization:** Menggunakan `atomic.Pointer` bawaan Go dan client Valkey/Redis yang sudah tersedia di binary server.
3. **Fail-Safe Startup:** Jika database tidak dapat dijangkau saat booting, sistem memuat konfigurasi *hardcoded in-memory fallback* yang aman sehingga monolith tidak *crash loop*.
