# Technical Architecture & Engineering Design
# Integrasi Payment Gateway Xendit & Notifikasi Outbox Resend
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `TECH-XENDIT-RESEND-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / Ready for Execution  

---

## 1. Arsitektur Komponen & Hexagonal Ports

```mermaid
flowchart TD
    subgraph Core["Domain Core (internal/booking)"]
        BS["booking.Service"]
        PG["booking.PaymentGateway (Port)"]
        NT["booking.Notifier (Port)"]
        PA["booking.PaymentAttemptStore (Port)"]
    end

    subgraph Adapters["Adapters (internal/adapter)"]
        XG["payment.XenditGateway"]
        FG["payment.FakeGateway"]
        RN["notifier.ResendNotifier"]
        LN["notifier.LogNotifier"]
    end

    subgraph Transport["Transport Layer (internal/api)"]
        Router["HTTP Router"]
        WH["Xendit Webhook Handler\nPOST /api/v1/webhooks/xendit"]
    end

    subgraph Workers["Workers Layer (internal/workers)"]
        OR["OutboxRelay Worker"]
        OBT[("outbox table")]
    end

    subgraph External["External Cloud Services"]
        XAPI["Xendit REST API\n(api.xendit.co)"]
        RAPI["Resend REST API\n(api.resend.com)"]
    end

    BS --> PG
    BS --> NT
    BS --> PA

    XG -.->|implements| PG
    FG -.->|implements| PG
    RN -.->|implements| NT
    LN -.->|implements| NT

    XG -->|HTTP POST /v2/invoices| XAPI
    RN -->|HTTP POST /emails| RAPI

    Router --> WH
    WH -->|verify x-callback-token| BS
    WH -->|record attempt| PA

    OR -->|poll pending events| OBT
    OR -->|booking.confirmed| NT
```

---

## 2. Diagram Alur Transaksi & Webhook (Sequence Diagram)

```mermaid
sequenceDiagram
    autonumber
    actor Guest as Tamu Pemesan
    participant Web as Nuxt 4 Webapp
    participant API as Go Backend API
    participant DB as PostgreSQL 18
    participant Xendit as Xendit Gateway
    participant Resend as Resend API
    participant Relay as Outbox Relay Worker

    Guest->>Web: Klik "Lanjut ke Pembayaran"
    Web->>API: POST /api/v1/bookings (Idempotency-Key)
    API->>DB: Begin Tx (Hold kamar, catat booking status=pending)
    API->>Xendit: POST /v2/invoices (Basic Auth)
    Xendit-->>API: 201 Created (invoice_url, id)
    API->>DB: Catat payment_attempts (status=initiated) & Commit Tx
    API-->>Web: 201 Created (payment_url, booking_id)
    Web-->>Guest: Redirect ke Xendit Checkout Hosted Page

    Guest->>Xendit: Bayar via QRIS / Virtual Account
    Xendit->>API: POST /api/v1/webhooks/xendit (x-callback-token, status=PAID)
    API->>API: Verify x-callback-token (Constant-Time Compare)
    API->>DB: Begin Tx
    API->>DB: Update bookings SET status='confirmed'
    API->>DB: Insert outbox (topic='booking.confirmed')
    API->>DB: Update payment_attempts (status='successful')
    API->>DB: Commit Tx
    API-->>Xendit: 200 OK {"status": "ok"}

    loop Every 2 Seconds
        Relay->>DB: Poll outbox (FOR UPDATE SKIP LOCKED)
        Relay->>Resend: POST /emails (Idempotency-Key, HTML template)
        Resend-->>Relay: 200 OK (id)
        Relay->>DB: Update outbox SET status='done'
    end

    Resend-->>Guest: Email Konfirmasi Booking Diterima
```

---

## 3. Desain Komponen & Pola Rekayasa

### 3.1 Xendit Gateway Adapter (`internal/adapter/payment/xendit.go`)
* **Struct:**
  ```go
  type XenditGateway struct {
      BaseURL       string
      SecretKey     string
      WebhookToken  string
      AppBaseURL    string
      Client        *http.Client
      Log           *slog.Logger
  }
  ```
* **Kepatuhan OWASP API:**
  Verifikasi `x-callback-token` menggunakan `subtle.ConstantTimeCompare([]byte(headerToken), []byte(g.WebhookToken)) == 1` untuk mencegah kerentanan *timing attack*.

### 3.2 Resend Notifier Adapter (`internal/adapter/notifier/resend.go`)
* **Struct:**
  ```go
  type ResendNotifier struct {
      BaseURL   string
      APIKey    string
      FromEmail string
      Client    *http.Client
      Log       *slog.Logger
  }
  ```
* **Template HTML Brand:**
  Dirancang dengan tipografi bersih, palet warna hotel (#2D2B2A, #F7F5F0, #9B4A2C), menampilkan nomor reservasi, tipe kamar, durasi menginap, serta kontak bantuan tamu.

### 3.3 Anti-Overengineering (Prinsip Ponytail)
* Murni menggunakan standard library Go `net/http` dengan `http.Client{Timeout: 10 * time.Second}`.
* Menggunakan `httptest.Server` untuk unit testing adapter tanpa koneksi eksternal langsung saat CI/test suite.
* Konfigurasi graceful fallback: Jika kredensial Xendit/Resend kosong pada mode development, server otomatis memakai `FakeGateway` dan `LogNotifier` sehingga developer tidak dipaksa memiliki API key nyata untuk menjalankan server lokal.
