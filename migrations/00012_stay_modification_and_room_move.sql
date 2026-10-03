-- +goose Up
-- SQL in this section is executed after the migration is applied.

CREATE TABLE IF NOT EXISTS room_move_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    from_room_number TEXT NOT NULL REFERENCES rooms(room_number),
    to_room_number TEXT NOT NULL REFERENCES rooms(room_number),
    move_date DATE NOT NULL DEFAULT CURRENT_DATE,
    reason_category VARCHAR(32) NOT NULL, -- maintenance_defect, noise_complaint, upgrade, guest_request
    notes TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_room_move_booking ON room_move_logs(booking_id);
CREATE INDEX IF NOT EXISTS idx_room_move_created_at ON room_move_logs(created_at DESC);

-- Aturan Casbin RBAC untuk Modul Stay Modification (Room Move & Stay Extension)
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'receptionist', '/api/v1/bookings/:id/room-move', 'POST'),
    ('p', 'receptionist', '/api/v1/bookings/:id/extend-stay', 'POST'),
    ('p', 'receptionist', '/api/v1/bookings/:id/room-moves', 'GET'),
    ('p', 'finance', '/api/v1/bookings/:id/room-moves', 'GET')
ON CONFLICT DO NOTHING;

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

DELETE FROM casbin_rule 
WHERE (v0 = 'receptionist' AND v1 IN ('/api/v1/bookings/:id/room-move', '/api/v1/bookings/:id/extend-stay', '/api/v1/bookings/:id/room-moves'))
   OR (v0 = 'finance' AND v1 = '/api/v1/bookings/:id/room-moves');

DROP TABLE IF EXISTS room_move_logs;
