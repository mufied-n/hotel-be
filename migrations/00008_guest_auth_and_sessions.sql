-- +goose Up
-- Tabel untuk menyimpan tantangan kode OTP tamu sementara secara aman (OWASP ASVS V2)
CREATE TABLE IF NOT EXISTS guest_auth_challenges (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    email        TEXT NOT NULL,
    code_hash    TEXT NOT NULL,
    attempts     INT  NOT NULL DEFAULT 0,
    max_attempts INT  NOT NULL DEFAULT 3,
    expires_at   TIMESTAMPTZ NOT NULL,
    verified_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_guest_challenges_email ON guest_auth_challenges(email, created_at DESC);

-- Tabel untuk menyimpan sesi tamu terverifikasi dengan token hash SHA-256 (OWASP ASVS V3)
CREATE TABLE IF NOT EXISTS guest_sessions (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    guest_email    TEXT NOT NULL,
    token_hash     TEXT NOT NULL UNIQUE,
    expires_at     TIMESTAMPTZ NOT NULL,
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_guest_sessions_token_hash ON guest_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_guest_sessions_email ON guest_sessions(guest_email);

-- Optimasi indeks pencarian booking berdasarkan email tamu untuk Booking Saya (UU PDP No. 27/2022)
CREATE INDEX IF NOT EXISTS idx_bookings_guest_email ON bookings(guest_email, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_guest_email;
DROP TABLE IF EXISTS guest_sessions;
DROP TABLE IF EXISTS guest_auth_challenges;
