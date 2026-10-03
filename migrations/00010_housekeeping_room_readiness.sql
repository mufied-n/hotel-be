-- +goose Up
-- Modul: Housekeeping Room Status & Readiness Lifecycle
-- Standard AHLA & Dual-Stage Cleanliness Protocol

-- 1. Penambahan atribut operasional dan kebersihan pada tabel rooms
ALTER TABLE rooms
    ADD COLUMN IF NOT EXISTS cleanliness_status VARCHAR(32) NOT NULL DEFAULT 'inspected'
        CHECK (cleanliness_status IN ('vacant_dirty', 'cleaning', 'vacant_clean', 'inspected', 'occupied', 'out_of_service', 'out_of_order')),
    ADD COLUMN IF NOT EXISTS maintenance_notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_by VARCHAR(64) NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_rooms_cleanliness ON rooms(cleanliness_status);

-- 2. Aturan Hak Akses Casbin untuk Staf Housekeeping & Front Desk
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'housekeeping', '/api/v1/housekeeping/rooms', 'GET'),
    ('p', 'housekeeping', '/api/v1/housekeeping/rooms/:id/status', 'PUT'),
    ('p', 'receptionist', '/api/v1/housekeeping/rooms', 'GET'),
    ('p', 'gm_admin', '/api/v1/housekeeping/*', '*')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE v1 LIKE '/api/v1/housekeeping%';
DROP INDEX IF EXISTS idx_rooms_cleanliness;
ALTER TABLE rooms
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS maintenance_notes,
    DROP COLUMN IF EXISTS cleanliness_status;
