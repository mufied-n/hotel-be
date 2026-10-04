-- +goose Up
-- 1. Tabel Konfigurasi Mitra Kanal OTA
CREATE TABLE IF NOT EXISTS channel_partners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code VARCHAR(32) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    webhook_secret VARCHAR(128) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed mitra kanal pengujian bawaan
INSERT INTO channel_partners (provider_code, name, webhook_secret, is_active) VALUES
    ('TRAVELOKA', 'Traveloka Indonesia', 'trv_secret_webhook_signature_key_2026', true),
    ('AGODA', 'Agoda Global Partner', 'agd_secret_webhook_signature_key_2026', true)
ON CONFLICT (provider_code) DO NOTHING;

-- 2. Tabel Kotak Masuk Event Kanal (Idempotent Inbox)
CREATE TABLE IF NOT EXISTS channel_event_inbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(32) NOT NULL,
    event_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    external_reference VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    error_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT uq_channel_event UNIQUE (provider, event_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_event_inbox_status ON channel_event_inbox (status, created_at);

-- 3. Tabel Karantina & Insiden Konflik Inventaris Kanal
CREATE TABLE IF NOT EXISTS channel_sync_issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(32) NOT NULL,
    external_reference VARCHAR(128) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    room_type_id UUID REFERENCES room_types(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'QUARANTINED_CONFLICT',
    reason TEXT NOT NULL,
    resolved_by VARCHAR(64),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_sync_issues_status ON channel_sync_issues (status, created_at);

-- 4. Casbin RBAC Rules untuk SSE Meja Depan dan Manajemen Sinkronisasi Kanal
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
    ('p', 'receptionist', '/api/v1/front-desk/live-stream', 'GET'),
    ('p', 'revenue_mgr', '/api/v1/staff/channel-sync-issues', 'GET'),
    ('p', 'revenue_mgr', '/api/v1/staff/channel-partners/:code', 'GET')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 IN (
    '/api/v1/front-desk/live-stream',
    '/api/v1/staff/channel-sync-issues',
    '/api/v1/staff/channel-partners/:code'
);

DROP TABLE IF EXISTS channel_sync_issues;
DROP TABLE IF EXISTS channel_event_inbox;
DROP TABLE IF EXISTS channel_partners;
