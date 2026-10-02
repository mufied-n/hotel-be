-- +goose Up
-- Batch BE-D: Checkout, Idempotensi & Pemulihan Pembayaran (BE-G07, BE-G09, BE-G11, BE-G12)
-- Sesuai standar UU PDP No. 27/2022, OWASP API Security, PCI-DSS v4.0 SAQ A, dan IETF Idempotency-Key.

-- 1. Penambahan field profil tamu dan hold expiry pada tabel bookings (BE-G07, BE-G12)
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS guest_phone VARCHAR(32),
    ADD COLUMN IF NOT EXISTS estimated_arrival_time VARCHAR(8),
    ADD COLUMN IF NOT EXISTS special_requests VARCHAR(500),
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- 2. Tabel penyimpanan Idempotency-Key untuk proteksi jaringan (BE-G09, IETF draft)
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(64) PRIMARY KEY,
    request_hash VARCHAR(64) NOT NULL,
    response_code INT NOT NULL,
    response_body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires_at ON idempotency_keys(expires_at);

-- 3. Buku besar percobaan pembayaran (payment attempts ledger) untuk rekonsiliasi finansial (BE-G11, PCI-DSS SAQ A)
CREATE TABLE IF NOT EXISTS payment_attempts (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    provider_reference VARCHAR(128) NOT NULL,
    amount_minor BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status VARCHAR(32) NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_booking_id ON payment_attempts(booking_id);
CREATE INDEX IF NOT EXISTS idx_payment_attempts_reference ON payment_attempts(provider_reference);

-- +goose Down
DROP TABLE IF EXISTS payment_attempts;
DROP TABLE IF EXISTS idempotency_keys;
ALTER TABLE bookings
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS special_requests,
    DROP COLUMN IF EXISTS estimated_arrival_time,
    DROP COLUMN IF EXISTS guest_phone;
