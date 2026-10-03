# Technical Architecture: Ketahanan Timeout Gateway Pembayaran & Integritas Buku Besar (BE-R13)

**Nomor Dokumen:** ARCH-PULANG-BE-R13  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait Audit & PRD:** `BE-R13`, `PRD-PULANG-BE-R13`, `SRS-PULANG-BE-R13`

---

## 1. Arsitektur Komponen & Diagram Alur

### 1.1 Diagram Keputusan Penanganan Error Gateway
```mermaid
flowchart TD
    Start["Guest Checkout (POST /api/v1/bookings)"] --> Tx["1. DB Transaction:\nInsert Booking & Decrement Inventory"]
    Tx --> PreAttempt["2. Pre-Gateway Intent:\nInsert payment_attempts (status='initiated')"]
    PreAttempt --> CallGW["3. External Call: PaymentGateway.CreateCharge"]
    
    CallGW -- "Success (200 OK)" --> UpdateSuccess["4a. Update payment_attempts:\nstatus='initiated', reference=res.ID\nReturn 201 Created"]
    CallGW -- "Gateway Error" --> Eval{"IsGatewayTimeout(err)?"}
    
    Eval -- "YES (Timeout / 502 / 504 / Net Error)" --> TimeoutPath["4b. Status='unknown_timeout'\nJANGAN CANCEL BOOKING\nPertahankan kamar & hold\nReturn 504 GATEWAY_TIMEOUT"]
    
    Eval -- "NO (Definitive Failure / 400 Bad Request)" --> DefFailPath["4c. Status='failed'\nJalankan Kompensasi: s.Cancel(bookingID)\nKembalikan stok kamar\nReturn 502 PAYMENT_FAILED"]
    
    DefFailPath --> CompCheck{"Kompensasi s.Cancel Berhasil?"}
    CompCheck -- "NO" --> LogCritical["Catat Log ERROR: booking.compensation.cancel_failed\n(Audit trail tersimpan di DB)"]
    CompCheck -- "YES" --> CompDone["Stok kamar aman kembali ke ketersediaan publik"]
```

### 1.2 Sequence Diagram: Penanganan Timeout vs Webhook Recovery
```mermaid
sequenceDiagram
    autonumber
    actor Guest as Tamu Hotel
    participant Router as API Transport
    participant Svc as booking.Service
    participant Ledger as PostgresPaymentAttemptStore
    participant GW as Xendit Payment Gateway
    participant Webhook as Xendit Webhook Callback

    Guest->>Router: POST /api/v1/bookings
    Router->>Svc: Create(ctx, in)
    Svc->>Ledger: RecordAttempt(attemptID, status="initiated")
    Svc->>GW: CreateCharge(booking, amount)
    Note over Svc, GW: Jaringan terputus / Timeout 10s
    GW-->>Svc: DeadlineExceeded / Network Reset
    Svc->>Ledger: UpdateAttemptByID(attemptID, status="unknown_timeout")
    Note over Svc: Booking TIDAK dibatalkan. Kamar tetap tertahan 30 menit.
    Svc-->>Router: ErrPaymentGatewayTimeout
    Router-->>Guest: 504 Gateway Timeout (Kamar tetap tersimpan)

    Note over Guest, Webhook: Tamu menerima link/VA atau menyelesaikan pembayaran
    Webhook->>Router: POST /api/v1/payments/xendit/webhook (PAID)
    Router->>Svc: Confirm(bookingID)
    Svc->>Ledger: UpdateAttemptStatus(bookingID, status="success")
    Svc-->>Router: Confirmed OK
    Note over Svc: Booking sukses tanpa overbooking!
```

---

## 2. Struktur Data & Kueri SQL

### 2.1 Ekstensi Antarmuka `PaymentAttemptStore`
```go
package booking

type PaymentAttemptStore interface {
    RecordAttempt(ctx context.Context, attempt PaymentAttempt) error
    UpdateAttemptStatus(ctx context.Context, bookingID string, status string) error
    UpdateAttemptByID(ctx context.Context, attemptID string, status string, providerReference string, payload map[string]any) error
    GetAttemptsByBookingID(ctx context.Context, bookingID string) ([]PaymentAttempt, error)
}
```

### 2.2 Kueri SQL Pembaruan Spesifik (`UpdateAttemptByID`)
```sql
UPDATE payment_attempts
SET status = $1,
    provider_reference = CASE WHEN $2 <> '' THEN $2 ELSE provider_reference END,
    payload = CASE WHEN $3::jsonb IS NOT NULL THEN $3::jsonb ELSE payload END,
    updated_at = $4
WHERE id = $5;
```

---

## 3. Analisis Anti-Overengineering (Ponytail Principles)

1. **YAGNI (You Aren't Gonna Need It):**
   - Tidak perlu memperkenalkan 2-phase commit terdistribusi (*distributed transaction / saga coordinator*) rumit untuk integrasi payment gateway. Cukup gunakan pola pencatatan *intent* pra-call, evaluasi error sederhana (`IsGatewayTimeout`), dan non-destructive hold preservation.
2. **Ketergantungan Minimal:**
   - Deteksi timeout dilakukan dengan inspeksi error standar Go (`errors.Is(err, context.DeadlineExceeded)`) dan pencocokan pola error jaringan tanpa menambahkan dependency pihak ketiga baru.
3. **Pemanfaatan Kolom yang Sudah Ada:**
   - Tabel `payment_attempts` sudah memiliki kolom `id UUID PRIMARY KEY`, `status`, dan `payload JSONB`. Tidak perlu menambahkan migrasi skema database baru; cukup gunakan kapabilitas yang telah tersedia secara optimal.
