-- +goose Up
-- Autentikasi staf (BE-R01): password bcrypt, lockout, dan sesi bertoken (hanya hash yang disimpan).
-- Akun seed dari 00003 tetap tanpa password sehingga tidak dapat login sampai operator mengaturnya
-- (go run ./cmd/staffadmin set-password <username>).
ALTER TABLE staff_users
    ADD COLUMN IF NOT EXISTS password_hash TEXT,
    ADD COLUMN IF NOT EXISTS failed_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS staff_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    staff_id UUID NOT NULL REFERENCES staff_users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_staff_sessions_staff_id ON staff_sessions (staff_id);

-- +goose Down
DROP TABLE IF EXISTS staff_sessions;
ALTER TABLE staff_users
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_attempts,
    DROP COLUMN IF EXISTS password_hash;
