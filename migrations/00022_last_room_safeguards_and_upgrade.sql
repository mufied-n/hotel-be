-- +goose Up
-- 1. Tambahkan safety_buffer pada channel_partners (default 1 kamar terlindungi untuk direct booking)
ALTER TABLE channel_partners 
ADD COLUMN IF NOT EXISTS safety_buffer INT NOT NULL DEFAULT 1;

-- 2. Tambahkan kolom resolusi pada channel_sync_issues untuk complimentary upgrade dan audit trail
ALTER TABLE channel_sync_issues
ADD COLUMN IF NOT EXISTS upgrade_room_type_id UUID REFERENCES room_types(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

-- 3. Kebijakan RBAC Casbin untuk Meja Depan dan Resolusi Sinkronisasi Kanal
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
    ('p', 'receptionist', '/api/v1/staff/channel-sync-issues', 'GET'),
    ('p', 'receptionist', '/api/v1/staff/channel-sync-issues/:id/resolve', 'POST'),
    ('p', 'revenue_mgr', '/api/v1/staff/channel-sync-issues/:id/resolve', 'POST'),
    ('p', 'gm_admin', '/api/v1/staff/channel-sync-issues/:id/resolve', 'POST')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 = '/api/v1/staff/channel-sync-issues/:id/resolve';
ALTER TABLE channel_sync_issues DROP COLUMN IF EXISTS notes;
ALTER TABLE channel_sync_issues DROP COLUMN IF EXISTS upgrade_room_type_id;
ALTER TABLE channel_partners DROP COLUMN IF EXISTS safety_buffer;
