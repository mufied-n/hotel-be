# TECH Architecture — F14: Rekonsiliasi Finansial & Otomasi Gateway Refund
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Terkait:
- **PRD Rujukan:** [PRD-F14-Finance](../prd/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **SRS Rujukan:** [SRS-F14-Finance](../srs/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F14](../walkthrough/finance-reconciliation-and-refunds-f14-walkthrough-2026-10-03.md)

---

## 1. Arsitektur Komponen & Aliran Data

Integrasi modul keuangan dan refund dalam arsitektur heksagonal hotel booking engine:

```mermaid
flowchart TD
    Staff["Staf Finance / GM"] -->|POST /api/v1/finance/refunds| Router["api.Router (RBAC Casbin Fail-Closed)"]
    Webhook["Xendit Webhook (Late Payment)"] -->|POST /api/v1/webhooks/xendit| XenditWH["api.xenditWebhook"]
    Guest["Tamu Webapp"] -->|GET /api/v1/guest/bookings/{id}/refund-status| GuestAuth["api.requireGuestSession"]

    Router --> FinSvc["finance.Service (Refund & Reconciliation)"]
    XenditWH --> FinSvc
    GuestAuth --> FinSvc

    FinSvc --> FinStore["finance.Store (PostgresStore)"]
    FinSvc --> XenGW["payment.XenditGateway (CreateRefund)"]
    
    FinStore --> DB[("PostgreSQL\n(payment_refunds, payment_cases, bookings, payment_attempts)")]
    XenGW --> XenAPI["Xendit API (POST /refunds)"]
```

---

## 2. Skema Database Relasional (`migrations/00009_finance_reconciliation_and_refunds.sql`)

```sql
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
```

---

## 3. Integrasi Xendit Refund API Client

Diimplementasikan pada `internal/adapter/payment/xendit.go`:
* **Endpoint:** `POST https://api.xendit.co/refunds`
* **Headers:**
  * `Authorization: Basic <base64(secret_key + ":")>`
  * `Content-Type: application/json`
  * `Idempotency-Key: <reference_id>`
* **Payload:**
```json
{
  "reference_id": "rfnd-01924b12-001",
  "invoice_id": "inv_12345",
  "currency": "IDR",
  "amount": 158950000,
  "reason": "CANCELLATION"
}
```
* **Status Mapping:**
  * `SUCCEEDED` / `PENDING` $\rightarrow$ `status = "succeeded"`
  * Response Error / Non-2xx $\rightarrow$ Dicatat sebagai `failed` tanpa mengurangi saldo hotel dua kali.

---

## 4. Mekanisme Pencegahan Over-Refund (Database Atomicity & Locks)

Sebelum memanggil gateway eksternal, sistem melakukan locking dalam transaksi:
```sql
SELECT b.total_price_minor, COALESCE(SUM(r.amount_minor), 0) as total_refunded
FROM bookings b
LEFT JOIN payment_refunds r ON r.booking_id = b.id AND r.status IN ('pending', 'succeeded')
WHERE b.id = $1
GROUP BY b.id, b.total_price_minor
FOR UPDATE OF b;
```
Bila `total_refunded + requested_amount > total_price_minor`, transaksi langsung di-abort dengan error `INVALID_REFUND_AMOUNT`.
