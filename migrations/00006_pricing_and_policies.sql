-- +goose Up
-- Paritas Batch BE-C: Dynamic Rate Plans, Money Breakdown, Quote Lock & Cancellation Policies (BE-G04, BE-G05, BE-G06, BE-G08, BE-G19)

-- 1. Tambah kolom snapshot harga, rate plan, kebijakan pembatalan, dan persetujuan tamu pada tabel bookings
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS quote_id UUID;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS rate_plan_code VARCHAR(32) NOT NULL DEFAULT 'room_only';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS cancellation_policy VARCHAR(32) NOT NULL DEFAULT 'flexible_48h';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS cancellation_desc TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS room_subtotal_minor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS breakfast_charge_minor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS discount_minor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS tax_minor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'IDR';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS terms_accepted BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS terms_accepted_at TIMESTAMPTZ;

-- 2. Kebijakan Akses Casbin untuk Endpoint Quotes Publik
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'guest', '/api/v1/quotes', 'POST')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 = '/api/v1/quotes';
ALTER TABLE bookings DROP COLUMN IF EXISTS terms_accepted_at;
ALTER TABLE bookings DROP COLUMN IF EXISTS terms_accepted;
ALTER TABLE bookings DROP COLUMN IF EXISTS currency;
ALTER TABLE bookings DROP COLUMN IF EXISTS tax_minor;
ALTER TABLE bookings DROP COLUMN IF EXISTS discount_minor;
ALTER TABLE bookings DROP COLUMN IF EXISTS breakfast_charge_minor;
ALTER TABLE bookings DROP COLUMN IF EXISTS room_subtotal_minor;
ALTER TABLE bookings DROP COLUMN IF EXISTS cancellation_desc;
ALTER TABLE bookings DROP COLUMN IF EXISTS cancellation_policy;
ALTER TABLE bookings DROP COLUMN IF EXISTS rate_plan_code;
ALTER TABLE bookings DROP COLUMN IF EXISTS quote_id;
