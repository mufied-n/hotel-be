-- +goose Up
-- F14: Rekonsiliasi Finansial & Otomasi Gateway Refund (Xendit Refund API)

-- 1. Tabel Buku Besar Pengembalian Dana (Payment Refunds Ledger)
CREATE TABLE IF NOT EXISTS payment_refunds (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    payment_attempt_id UUID REFERENCES payment_attempts(id) ON DELETE SET NULL,
    reference_id VARCHAR(64) UNIQUE NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    reason TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    provider VARCHAR(32) NOT NULL DEFAULT 'xendit',
    provider_refund_id VARCHAR(128),
    actor_id VARCHAR(64) NOT NULL,
    actor_role VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_refunds_booking_id ON payment_refunds(booking_id);
CREATE INDEX IF NOT EXISTS idx_payment_refunds_reference_id ON payment_refunds(reference_id);

-- 2. Tabel Kasus Pembayaran & Sengketa (Payment Cases)
CREATE TABLE IF NOT EXISTS payment_cases (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID REFERENCES bookings(id) ON DELETE SET NULL,
    case_type VARCHAR(32) NOT NULL, -- late_payment, amount_mismatch, duplicate_payment
    status VARCHAR(32) NOT NULL DEFAULT 'open', -- open, investigating, resolved, dismissed
    amount_minor BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    provider_reference VARCHAR(128) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    resolved_by VARCHAR(64),
    resolved_at TIMESTAMPTZ,
    resolution_action VARCHAR(32), -- refunded, reallocated_room, manual_adjustment
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_cases_booking_id ON payment_cases(booking_id);
CREATE INDEX IF NOT EXISTS idx_payment_cases_status ON payment_cases(status);

-- 3. Aturan Hak Akses Casbin untuk Staf Finance
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'finance', '/api/v1/finance/refunds', 'POST'),
    ('p', 'finance', '/api/v1/finance/cases', 'GET'),
    ('p', 'finance', '/api/v1/finance/cases/:id/resolve', 'POST'),
    ('p', 'finance', '/api/v1/finance/reconciliations', 'GET'),
    ('p', 'gm_admin', '/api/v1/finance/*', '*')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE v1 LIKE '/api/v1/finance%';
DROP TABLE IF EXISTS payment_cases;
DROP TABLE IF EXISTS payment_refunds;
