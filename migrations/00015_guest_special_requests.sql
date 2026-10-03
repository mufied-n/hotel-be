-- +goose Up
-- SQL in this section is executed after the migration is applied.

CREATE TABLE IF NOT EXISTS booking_special_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    category VARCHAR(32) NOT NULL CHECK (category IN (
        'early_arrival', 'late_departure', 'high_floor', 'quiet_room',
        'bed_type', 'celebration_setup', 'baby_crib', 'dietary_allergy', 'other'
    )),
    department VARCHAR(16) NOT NULL CHECK (department IN ('front_desk', 'housekeeping')),
    description TEXT NOT NULL,
    target_time VARCHAR(8) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged', 'fulfilled', 'declined')),
    staff_notes TEXT NOT NULL DEFAULT '',
    handled_by VARCHAR(64) NOT NULL DEFAULT '',
    handled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_special_requests_booking ON booking_special_requests(booking_id);
CREATE INDEX IF NOT EXISTS idx_special_requests_dept_status ON booking_special_requests(department, status);

-- Casbin RBAC Policies for Receptionist & Housekeeping
INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5)
VALUES
    ('p', 'receptionist', '/api/v1/front-desk/special-requests', 'GET', '', '', ''),
    ('p', 'receptionist', '/api/v1/front-desk/special-requests/:id/status', 'PUT', '', '', ''),
    ('p', 'housekeeping', '/api/v1/front-desk/special-requests', 'GET', '', '', ''),
    ('p', 'housekeeping', '/api/v1/front-desk/special-requests/:id/status', 'PUT', '', '', '')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- Feature Flag: ff_guest_special_requests
INSERT INTO feature_flags (key, name, description, enabled, allowed_roles)
VALUES (
    'ff_guest_special_requests',
    'Guest Special Requests Management',
    'Pengajuan dan pemenuhan permintaan khusus tamu terstruktur',
    true,
    '{}'
) ON CONFLICT (key) DO UPDATE SET allowed_roles = EXCLUDED.allowed_roles;

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

DELETE FROM feature_flags WHERE key = 'ff_guest_special_requests';
DELETE FROM casbin_rule 
WHERE v1 IN ('/api/v1/front-desk/special-requests', '/api/v1/front-desk/special-requests/:id/status');
DROP TABLE IF EXISTS booking_special_requests CASCADE;
