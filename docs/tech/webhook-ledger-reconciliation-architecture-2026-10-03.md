# Technical Architecture — Webhook Ledger & Amount Reconciliation (BE-R14)

**Fitur:** Webhook Amount, Currency & Ledger Verification  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** [PRD](../prd/webhook-ledger-reconciliation-r14-2026-10-03.md) · [SRS](../srs/webhook-ledger-reconciliation-r14-2026-10-03.md) · BE-R14  

---

## 1. Sequence Diagram: Validasi & Rekonsiliasi Webhook Xendit

```mermaid
sequenceDiagram
    autonumber
    actor Gateway as Xendit Payment Gateway
    participant Router as Gin Router (/api/v1/webhooks/xendit)
    participant XenditAdapter as payment.XenditGateway
    participant BookingService as booking.Service
    participant AttemptStore as booking.PaymentAttemptStore

    Gateway->>Router: POST /api/v1/webhooks/xendit (x-callback-token, Payload JSON)
    Router->>XenditAdapter: VerifyWebhook(token, bodyBytes)
    alt Token Callback Tidak Valid
        XenditAdapter-->>Router: ErrInvalidWebhookToken
        Router-->>Gateway: HTTP 401 Unauthorized
    end
    XenditAdapter-->>Router: Payload (ID, ExternalID, Amount, Currency, Status)

    Router->>BookingService: Get(ctx, ExternalID)
    alt Booking Tidak Ditemukan
        BookingService-->>Router: ErrNotFound
        Router-->>Gateway: HTTP 404 Not Found (BOOKING_NOT_FOUND)
    end
    BookingService-->>Router: Booking Record (Status, TotalPriceMinor, Currency)

    alt Status == PAID / SETTLED
        Note over Router: 1. Verifikasi Amount
        alt payload.Amount != booking.TotalPriceMinor
            Router-->>Gateway: HTTP 422 Unprocessable Entity (PAYMENT_AMOUNT_MISMATCH)
        end
        Note over Router: 2. Verifikasi Currency
        alt payload.Currency != "" && != booking.Currency
            Router-->>Gateway: HTTP 422 Unprocessable Entity (PAYMENT_CURRENCY_MISMATCH)
        end
        Note over Router: 3. Verifikasi Invoice ID Ledger
        Router->>BookingService: GetPaymentAttempts(ctx, ExternalID)
        BookingService->>AttemptStore: GetAttemptsByBookingID(ctx, ExternalID)
        AttemptStore-->>BookingService: []PaymentAttempt
        BookingService-->>Router: []PaymentAttempt
        alt Attempts Ada & payload.ID Tidak Cocok
            Router-->>Gateway: HTTP 422 Unprocessable Entity (INVOICE_ID_MISMATCH)
        end
        Note over Router: 4. Idempotent Confirmation
        alt booking.Status == StatusConfirmed
            Router-->>Gateway: HTTP 200 OK ("already confirmed, idempotent replay")
        else booking.Status == StatusPending
            Router->>BookingService: Confirm(ctx, ExternalID)
            BookingService-->>Router: OK
            Router-->>Gateway: HTTP 200 OK ("booking confirmed")
        end
    else Status == EXPIRED
        Note over Router: 5. Proteksi Out-of-Order Expiry
        alt booking.Status == StatusConfirmed
            Router-->>Gateway: HTTP 200 OK ("stale expiry event ignored")
        else
            Router->>BookingService: Cancel(ctx, ExternalID)
            Router-->>Gateway: HTTP 200 OK ("booking cancelled")
        end
    end
```

---

## 2. Rincian Modifikasi Kode

1. **`internal/adapter/payment/xendit.go`:**
   - Tambahkan field `Currency string` pada struct `XenditWebhookPayload`.
2. **`internal/booking/service.go`:**
   - Tambahkan method `GetPaymentAttempts(ctx context.Context, bookingID string) ([]PaymentAttempt, error)`.
3. **`internal/api/router.go`:**
   - Perbarui handler `xenditWebhook`:
     - Panggil `d.BookingSvc.Get(ctx, payload.ExternalID)` terlebih dahulu.
     - Lakukan validasi `Amount`, `Currency`, dan `Invoice ID`.
     - Cegah pembatalan booking berstatus `StatusConfirmed` saat event `EXPIRED` diterima.
4. **`internal/api/webhook_test.go`:**
   - Tambahkan skenario table test:
     - `Amount mismatch rejected with 422`
     - `Currency mismatch rejected with 422`
     - `Invoice ID mismatch rejected with 422`
     - `Stale EXPIRED on confirmed booking returns 200 ignored without cancelling`
     - `Booking not found returns 404`

---

## 3. Analisis Anti-Overengineering (Ponytail Review)

- **Menggunakan Komponen yang Sudah Ada:** Menggunakan kembali `PaymentAttemptStore` yang telah diimplementasikan sejak BE-G11 tanpa membuat skema database baru.
- **Fail-Safe HTTP Codes:** Menggunakan HTTP 422 Unprocessable Entity untuk kegagalan semantik data (mismatch), sehingga gateway dapat membedakan error autentikasi (401), sintaks (400), dan semantik tagihan (422).
