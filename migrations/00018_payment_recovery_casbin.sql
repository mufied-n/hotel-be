-- +goose Up
-- Policy Casbin untuk pemulihan tautan pembayaran tamu (BE-R15)
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES ('p', 'guest', '/api/v1/bookings/:id/payment', 'GET')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 = '/api/v1/bookings/:id/payment' AND v2 = 'GET';
