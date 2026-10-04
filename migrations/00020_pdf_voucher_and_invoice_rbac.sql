-- +goose Up
-- Policy Casbin untuk PDF Confirmation Voucher, Faktur Pajak PBJT Sleman, dan Front Desk Voucher Verification (Kandidat B)
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
    ('p', 'receptionist', '/api/v1/bookings/:id/voucher.pdf', 'GET'),
    ('p', 'receptionist', '/api/v1/front-desk/verify-voucher', 'GET'),
    ('p', 'finance', '/api/v1/bookings/:id/invoice.pdf', 'GET')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 IN (
    '/api/v1/bookings/:id/voucher.pdf',
    '/api/v1/bookings/:id/invoice.pdf',
    '/api/v1/front-desk/verify-voucher'
);
