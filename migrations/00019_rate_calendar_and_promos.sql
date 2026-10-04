-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE IF NOT EXISTS rate_calendar_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_type_id UUID NOT NULL REFERENCES room_types(id) ON DELETE CASCADE,
    rate_plan_code VARCHAR(32) NOT NULL DEFAULT 'RO',
    date DATE NOT NULL,
    price_override_idr BIGINT NULL,
    is_stop_sell BOOLEAN NOT NULL DEFAULT FALSE,
    is_cta BOOLEAN NOT NULL DEFAULT FALSE,
    is_ctd BOOLEAN NOT NULL DEFAULT FALSE,
    min_los INT NOT NULL DEFAULT 1,
    max_los INT NOT NULL DEFAULT 30,
    allotment_limit INT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rate_cal_room_date_plan UNIQUE (room_type_id, date, rate_plan_code)
);

CREATE INDEX IF NOT EXISTS idx_rate_cal_range 
ON rate_calendar_overrides (room_type_id, date, rate_plan_code);

CREATE TABLE IF NOT EXISTS promo_campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(32) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    discount_type VARCHAR(16) NOT NULL DEFAULT 'PERCENT',
    discount_value INT NOT NULL,
    max_discount_idr BIGINT NULL,
    min_stay_nights INT NOT NULL DEFAULT 1,
    quota_total INT NOT NULL DEFAULT 100,
    quota_used INT NOT NULL DEFAULT 0,
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ NOT NULL,
    applicable_room_types UUID[] NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_promo_quota_valid CHECK (quota_used <= quota_total)
);

CREATE INDEX IF NOT EXISTS idx_promo_code_active 
ON promo_campaigns (code) WHERE is_active = TRUE;

-- Casbin RBAC Policies for Revenue Management
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
('p', 'revenue_mgr', '/api/v1/revenue/calendar', 'GET'),
('p', 'revenue_mgr', '/api/v1/revenue/calendar/bulk', 'PUT'),
('p', 'revenue_mgr', '/api/v1/revenue/promos', 'GET'),
('p', 'revenue_mgr', '/api/v1/revenue/promos', 'POST'),
('p', 'revenue_mgr', '/api/v1/revenue/promos/:id', 'PUT'),
('p', 'gm_admin', '/api/v1/revenue/calendar', 'GET'),
('p', 'gm_admin', '/api/v1/revenue/calendar/bulk', 'PUT'),
('p', 'gm_admin', '/api/v1/revenue/promos', 'GET'),
('p', 'gm_admin', '/api/v1/revenue/promos', 'POST'),
('p', 'gm_admin', '/api/v1/revenue/promos/:id', 'PUT'),
('p', 'receptionist', '/api/v1/revenue/calendar', 'GET')
ON CONFLICT DO NOTHING;

-- Seed default promo code OCTOBREAK for backward compatibility
INSERT INTO promo_campaigns (code, name, discount_type, discount_value, max_discount_idr, min_stay_nights, quota_total, quota_used, valid_from, valid_to, is_active)
VALUES (
    'OCTOBREAK',
    'Promo Musim Gugur Oktober 15%',
    'PERCENT',
    15,
    1000000,
    1,
    1000,
    0,
    '2026-10-01 00:00:00+07',
    '2026-10-31 23:59:59+07',
    TRUE
) ON CONFLICT (code) DO NOTHING;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DELETE FROM casbin_rule WHERE v1 LIKE '/api/v1/revenue/%';
DROP TABLE IF EXISTS promo_campaigns;
DROP TABLE IF EXISTS rate_calendar_overrides;
