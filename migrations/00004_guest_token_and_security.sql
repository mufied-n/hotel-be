-- +goose Up
-- Menambahkan kolom guest_token untuk verifikasi hak milik pemesanan & proteksi PII (BE-G13)
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS guest_token VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_bookings_guest_token ON bookings(id, guest_token);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_guest_token;
ALTER TABLE bookings DROP COLUMN IF EXISTS guest_token;
