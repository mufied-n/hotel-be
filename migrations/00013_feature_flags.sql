-- +goose Up
-- Tabel feature_flags menyimpan toggle konfigurasi runtime dan batasan peran (role scoping)
CREATE TABLE IF NOT EXISTS feature_flags (
    key           VARCHAR(64) PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    description   TEXT NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    allowed_roles TEXT[] NOT NULL DEFAULT '{}',
    updated_by    VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed 17 Feature Flags Resmi Pulang ke Uttara (default: aktif / TRUE)
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

-- Tambahkan Casbin rules untuk endpoint administrasi feature flags
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
('p', 'gm_admin', '/api/v1/admin/feature-flags', 'GET'),
('p', 'gm_admin', '/api/v1/admin/feature-flags/*', 'PUT')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE v1 LIKE '/api/v1/admin/feature-flags%';
DROP TABLE IF EXISTS feature_flags;
