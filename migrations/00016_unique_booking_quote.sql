-- +goose Up
-- Satu quote terkunci hanya boleh menghasilkan satu booking (BE-R06/R08 follow-up).
-- Juga menutup duplikasi bila reservasi idempotency kedaluwarsa sebelum respons tercatat.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_quote_id ON bookings (quote_id) WHERE quote_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_bookings_quote_id;
