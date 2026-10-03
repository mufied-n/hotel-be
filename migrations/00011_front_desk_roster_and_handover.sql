-- +goose Up
-- Modul: Front Desk Daily Operations Roster & Shift Handover Board
-- Operasional Terpadu Meja Depan & Pencatatan Digital Handover Logbook

-- 1. Pembuatan tabel front_desk_handover_notes (Append-Only)
CREATE TABLE IF NOT EXISTS front_desk_handover_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shift VARCHAR(16) NOT NULL CHECK (shift IN ('morning', 'afternoon', 'night')),
    cash_float_minor BIGINT NOT NULL DEFAULT 0,
    pending_issues TEXT NOT NULL DEFAULT '',
    vip_guest_notes TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(64) NOT NULL,
    actor_role VARCHAR(64) NOT NULL DEFAULT 'receptionist',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_handover_notes_created_at ON front_desk_handover_notes(created_at DESC);

-- 2. Aturan Hak Akses Casbin untuk Modul Front Desk Roster & Handover
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'receptionist', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'receptionist', '/api/v1/front-desk/handover-notes', 'GET'),
    ('p', 'receptionist', '/api/v1/front-desk/handover-notes', 'POST'),
    ('p', 'housekeeping', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'revenue_mgr', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'gm_admin', '/api/v1/front-desk/*', '*')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE v1 LIKE '/api/v1/front-desk%';
DROP TABLE IF EXISTS front_desk_handover_notes;
