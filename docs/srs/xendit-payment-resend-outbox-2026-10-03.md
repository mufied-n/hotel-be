# Software Requirements Specification (SRS)
# Integrasi Payment Gateway Xendit & Notifikasi Outbox Resend
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `SRS-XENDIT-RESEND-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / Ready for Execution  

---

## 1. Spesifikasi Antarmuka Eksternal & API

### 1.1 Xendit Invoice Integration

#### Endpoint: `POST https://api.xendit.co/v2/invoices`
* **Authentication:** HTTP Basic Auth (`Authorization: Basic base64(XENDIT_SECRET_KEY + ":")`)
* **Headers:**
  * `Content-Type: application/json`
* **Request JSON Schema:**
  ```json
  {
    "external_id": "01900000-0000-7000-8000-000000000001",
    "amount": 1100000,
    "payer_email": "tamu@example.com",
    "description": "Hotel Pulang ke Uttara - Reservasi 01900000-0000-7000-8000-000000000001",
    "invoice_duration": 1800,
    "currency": "IDR",
    "success_redirect_url": "https://pulangkeuttara.com/booking/status/01900000-0000-7000-8000-000000000001?payment=success",
    "failure_redirect_url": "https://pulangkeuttara.com/booking/status/01900000-0000-7000-8000-000000000001?payment=failed"
  }
  ```
* **Success Response Schema (HTTP 200/201):**
  ```json
  {
    "id": "6724a8f9024f92d42e316abc",
    "external_id": "01900000-0000-7000-8000-000000000001",
    "invoice_url": "https://checkout.xendit.co/web/6724a8f9024f92d42e316abc",
    "status": "PENDING",
    "amount": 1100000,
    "expiry_date": "2026-10-03T10:30:00.000Z"
  }
  ```

#### Webhook Endpoint: `POST /api/v1/webhooks/xendit`
* **Otentikasi:** Header `x-callback-token` wajib cocok dengan `XENDIT_WEBHOOK_TOKEN`.
* **Payload JSON:**
  ```json
  {
    "id": "6724a8f9024f92d42e316abc",
    "external_id": "01900000-0000-7000-8000-000000000001",
    "status": "PAID",
    "amount": 1100000,
    "payment_method": "QRIS",
    "paid_at": "2026-10-03T10:05:00.000Z"
  }
  ```
* **HTTP Responses:**
  * `200 OK`: `{"status": "ok", "message": "webhook processed"}` (saat sukses diverifikasi).
  * `401 Unauthorized`: `{"type": "/problems/unauthorized", "title": "Unauthorized", "status": 401, "detail": "invalid webhook token"}` (saat token tidak cocok).
  * `400 Bad Request`: `{"type": "/problems/bad-request", "title": "Bad Request", "status": 400, "detail": "invalid payload"}`.

---

### 1.2 Resend Email Integration

#### Endpoint: `POST https://api.resend.com/emails`
* **Authentication:** `Authorization: Bearer RESEND_API_KEY`
* **Headers:**
  * `Content-Type: application/json`
  * `Idempotency-Key: email-confirmed-<booking_id>`
* **Request JSON Schema:**
  ```json
  {
    "from": "Pulang ke Uttara <reservations@pulangkeuttara.com>",
    "to": ["tamu@example.com"],
    "subject": "Konfirmasi Pemesanan Kamar - Pulang ke Uttara [01900000-0000-7000-8000-000000000001]",
    "html": "<!DOCTYPE html><html>...</html>"
  }
  ```
* **Success Response Schema (HTTP 200/201):**
  ```json
  {
    "id": "49a3999c-0ce1-4ea6-ab68-af69c7d77944"
  }
  ```

---

## 2. Kebutuhan Fungsional (Functional Requirements)

* **FR-PAY-01:** Saat `booking.Service.Create` dipanggil, jika gateway adalah Xendit, invoice dibuat via HTTP request dengan timeout 10 detik. Jika Xendit merespons error (5xx atau 4xx), booking hold di-rollback dan error dikembalikan ke caller.
* **FR-PAY-02:** Router HTTP mendaftarkan rute publik `POST /api/v1/webhooks/xendit`. Rute ini tidak memerlukan otentikasi JWT staf, melainkan memverifikasi header `x-callback-token`.
* **FR-PAY-03:** Jika webhook menerima status `PAID` atau `SETTLED`, sistem:
  1. Memanggil `svc.Confirm(ctx, payload.ExternalID)`.
  2. Mencatat ke `payment_attempts` dengan status `successful`, payment method, dan reference ID Xendit.
  3. Mengembalikan HTTP 200 OK.
* **FR-PAY-04:** Jika booking sudah berstatus `confirmed` (misal karena replay webhook dari Xendit), `Confirm` bersifat idempoten dan mengembalikan 200 OK.
* **FR-PAY-05:** Jika status invoice adalah `EXPIRED`, sistem memanggil `svc.Cancel(ctx, payload.ExternalID)` untuk mengembalikan ketersediaan kamar dan mencatat attempt `expired`.
* **FR-NOTIF-01:** Worker `OutboxRelay` yang memproses topic `booking.confirmed` memanggil `notifier.SendBookingConfirmed(ctx, b)`.
* **FR-NOTIF-02:** Adapter Resend menyusun template HTML responsif dengan styling brand Pulang ke Uttara, menginjeksi header `Idempotency-Key`, dan mengirim email ke tamu.
* **FR-NOTIF-03:** Jika Resend mengembalikan status 200/201, transaksi outbox di-commit sebagai `done`. Jika terjadi error jaringan atau 5xx, outbox mengembalikan error sehingga relay menjadwalkan retry dengan backoff eksponensial.
* **FR-CFG-01:** Environment variables baru ditambahkan ke `platform.Config`:
  * `XENDIT_BASE_URL` (default: `https://api.xendit.co`)
  * `XENDIT_SECRET_KEY` (secret key Xendit)
  * `XENDIT_WEBHOOK_TOKEN` (verification token webhook Xendit)
  * `RESEND_BASE_URL` (default: `https://api.resend.com`)
  * `RESEND_API_KEY` (API key Resend)
  * `RESEND_FROM_EMAIL` (default: `Pulang ke Uttara <reservations@pulangkeuttara.com>`)
  * `APP_BASE_URL` (default: `http://localhost:3000`)
