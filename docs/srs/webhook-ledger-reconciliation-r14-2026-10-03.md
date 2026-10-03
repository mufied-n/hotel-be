# SRS — Webhook Ledger & Amount Reconciliation (BE-R14)

**Fitur:** Webhook Amount, Currency & Ledger Verification (Anti-Underpayment & Out-of-Order Expiry)  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** [PRD](../prd/webhook-ledger-reconciliation-r14-2026-10-03.md) · BE-R14  

---

## 1. Spesifikasi Kebutuhan Fungsional (Functional Requirements)

### FR-01: Verifikasi Nilai Transaksi & Mata Uang
- **Input:** `XenditWebhookPayload` yang memuat `external_id`, `amount`, `currency`, `id`, `status`.
- **Prosedur:**
  1. Ambil detail booking berdasarkan `payload.ExternalID` menggunakan `BookingSvc.Get(ctx, externalID)`.
  2. Jika booking tidak ditemukan: kembalikan HTTP 404 Not Found (`BOOKING_NOT_FOUND`).
  3. Jika `payload.Status` adalah `"PAID"` atau `"SETTLED"`:
     - Bandingkan `payload.Amount` dengan `booking.TotalPriceMinor`. Jika tidak sama persis: kembalikan HTTP 422 Unprocessable Entity (`PAYMENT_AMOUNT_MISMATCH`).
     - Jika `payload.Currency != ""` dan `strings.ToUpper(payload.Currency) != strings.ToUpper(booking.Currency)`: kembalikan HTTP 422 Unprocessable Entity (`PAYMENT_CURRENCY_MISMATCH`).

### FR-02: Verifikasi Invoice Reference terhadap Buku Besar (`PaymentAttempt`)
- **Prosedur:**
  - Ambil riwayat percobaan pembayaran menggunakan method baru `BookingSvc.GetPaymentAttempts(ctx, externalID)`.
  - Jika terdapat riwayat attempt dan setidaknya satu attempt memiliki `ProviderReference != ""`:
    - Periksa apakah `payload.ID` cocok dengan salah satu `ProviderReference`.
    - Jika tidak ada attempt yang cocok: kembalikan HTTP 422 Unprocessable Entity (`INVOICE_ID_MISMATCH`).

### FR-03: Proteksi Out-of-Order Expiry Callback
- **Prosedur:**
  - Jika `payload.Status == "EXPIRED"`:
    - Periksa status booking saat ini.
    - Jika `booking.Status == StatusConfirmed`: jangan panggil `BookingSvc.Cancel()`. Segera kembalikan HTTP 200 OK dengan payload:
      ```json
      {
        "status": "ignored",
        "message": "booking already confirmed, stale expiry event ignored"
      }
      ```
    - Jika booking masih pending: panggil `BookingSvc.Cancel()`. Bila cancel gagal, laporkan error HTTP 500 (`CANCEL_FAILED`) agar provider melakukan retry.

### FR-04: Idempotensi Webhook Callback
- **Prosedur:**
  - Jika `booking.Status == StatusConfirmed` dan event yang masuk adalah `PAID` atau `SETTLED`:
    - Kembalikan HTTP 200 OK:
      ```json
      {
        "status": "ok",
        "message": "booking already confirmed (idempotent replay)"
      }
      ```
    - Tanpa membuat duplikasi outbox event notifikasi.

---

## 2. Kontrak Respons HTTP (Error Responses)

### Response 422: Nominal Tidak Cocok
```json
{
  "status": 422,
  "code": "PAYMENT_AMOUNT_MISMATCH",
  "error": "payment amount mismatch: expected 1100000, got 50000",
  "title": "Unprocessable Entity",
  "detail": "payment amount mismatch: expected 1100000, got 50000"
}
```

### Response 422: Invoice ID Tidak Cocok
```json
{
  "status": 422,
  "code": "INVOICE_ID_MISMATCH",
  "error": "invoice ID does not match recorded payment attempt",
  "title": "Unprocessable Entity",
  "detail": "invoice ID does not match recorded payment attempt"
}
```
