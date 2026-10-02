-- +goose Up
-- Casbin RBAC storage and initial staff users for Pulang ke Uttara (95 rooms)

CREATE TABLE IF NOT EXISTS casbin_rule (
    id SERIAL PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL,
    v0 VARCHAR(100) DEFAULT '',
    v1 VARCHAR(100) DEFAULT '',
    v2 VARCHAR(100) DEFAULT '',
    v3 VARCHAR(100) DEFAULT '',
    v4 VARCHAR(100) DEFAULT '',
    v5 VARCHAR(100) DEFAULT '',
    CONSTRAINT uq_casbin_rule UNIQUE (ptype, v0, v1, v2, v3, v4, v5)
);

CREATE INDEX IF NOT EXISTS idx_casbin_rule_lookup ON casbin_rule (ptype, v0, v1);

CREATE TABLE IF NOT EXISTS staff_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed staf awal Pulang ke Uttara
INSERT INTO staff_users (username, role, full_name) VALUES
    ('admin_gm', 'gm_admin', 'General Manager Uttara'),
    ('fo_receptionist', 'receptionist', 'Front Desk Officer 1'),
    ('hk_lead', 'housekeeping', 'Housekeeping Lead'),
    ('rev_mgr', 'revenue_mgr', 'Yield & Revenue Manager'),
    ('fin_officer', 'finance', 'Finance Officer')
ON CONFLICT (username) DO NOTHING;

-- Seed Policy Casbin
INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES
    -- Guest Permissions
    ('p', 'guest', '/api/v1/availability', 'GET', '', '', ''),
    ('p', 'guest', '/api/v1/bookings', 'POST', '', '', ''),
    ('p', 'guest', '/api/v1/bookings/:id', 'GET', '', '', ''),
    ('p', 'guest', '/api/v1/bookings/:id/cancel', 'POST', '', '', ''),
    ('p', 'guest', '/fake-pay/:ref', 'POST', '', '', ''),

    -- Receptionist Permissions (Check-in, Check-out, No-Show)
    ('p', 'receptionist', '/api/v1/bookings/:id/check-in', 'POST', '', '', ''),
    ('p', 'receptionist', '/api/v1/bookings/:id/check-out', 'POST', '', '', ''),
    ('p', 'receptionist', '/api/v1/bookings/:id/no-show', 'POST', '', '', ''),

    -- Housekeeping Permissions
    ('p', 'housekeeping', '/api/v1/rooms/housekeeping', 'GET', '', '', ''),
    ('p', 'housekeeping', '/api/v1/rooms/:room_number/housekeeping', 'PUT', '', '', ''),

    -- Revenue Manager Permissions
    ('p', 'revenue_mgr', '/api/v1/availability', 'GET', '', '', ''),
    ('p', 'revenue_mgr', '/api/v1/bookings/:id', 'GET', '', '', ''),
    ('p', 'revenue_mgr', '/api/v1/rates', 'PUT', '', '', ''),
    ('p', 'revenue_mgr', '/api/v1/inventory/blocks', 'POST', '', '', ''),

    -- Finance Permissions
    ('p', 'finance', '/api/v1/reports/*', 'GET', '', '', ''),
    ('p', 'finance', '/api/v1/bookings/:id/refund', 'POST', '', '', ''),

    -- General Manager (Super Admin: full access to /api/v1/* and beyond)
    ('p', 'gm_admin', '/api/v1/*', '*', '', '', ''),

    -- Role Inheritance: receptionist mewarisi akses guest
    ('g', 'receptionist', 'guest', '', '', '', '')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS staff_users;
DROP TABLE IF EXISTS casbin_rule;
