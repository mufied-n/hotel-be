# Software Requirements Specification (SRS)
# Real-Time Hospitality Event Hub: NATS JetStream, Live Front Desk SSE, Multi-Channel Webhooks & Notifier
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `SRS-F10-F11-F07-REALTIME-EVENT-HUB-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - PRD: [`docs/prd/realtime-hospitality-event-hub-and-channel-sync-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/realtime-hospitality-event-hub-and-channel-sync-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/realtime-hospitality-event-hub-and-channel-sync-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/realtime-hospitality-event-hub-and-channel-sync-architecture-2026-10-04.md)
  - Walkthrough Tracking: [`docs/walkthrough/realtime-hospitality-event-hub-and-channel-sync-walkthrough-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/realtime-hospitality-event-hub-and-channel-sync-walkthrough-2026-10-04.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

* **FR-HUB-01 (NATS JetStream Event Broker & Stream Topology):** Sistem wajib menyediakan satu bus event terpusat berbasis NATS JetStream dengan stream bernama `HOSPITALITY_EVENTS` yang menangani subjek bertingkat `hospitality.booking.*.*`, `hospitality.room.*.*`, dan `hospitality.channel.*.*` lengkap dengan *file storage persistence* dan *deduplication window*.
* **FR-HUB-02 (PostgreSQL Outbox Publisher Bridge):** Sistem wajib mengalirkan event dari transaksi PostgreSQL ke NATS JetStream secara atomik melalui tabel `events_outbox` dengan menyertakan header `Nats-Msg-Id` untuk mencegah *dual-write hazard*.
* **FR-HUB-03 (Guest Real-Time Payment SSE Stream):** Sistem wajib menyediakan endpoint HTTP streaming `GET /api/v1/guest/bookings/:id/live-status` berformat `text/event-stream` untuk mendorong pembaruan status transaksi (`payment_confirmed`, `booking_expired`, `booking_cancelled`) ke browser tamu secara instan tanpa polling.
* **FR-HUB-04 (Front Desk Live Operational SSE Stream):** Sistem wajib menyediakan endpoint HTTP streaming `GET /api/v1/front-desk/live-stream` bagi peran resepsionis dan manajemen umum untuk menerima siaran pemesanan baru, check-in, dan perubahan status kamar secara *real-time*.
* **FR-HUB-05 (Durable WhatsApp Notification Worker):** Sistem wajib menyediakan worker konsumen NATS yang memproses event `hospitality.booking.*.confirmed` dan mengirimkan pesan WhatsApp terformat ramah ke nomor tamu beserta tautan e-voucher PDF resmi.
* **FR-HUB-06 (Durable Rich Email Notification Worker):** Sistem wajib menyediakan worker konsumen NATS yang memproses event `hospitality.booking.*.confirmed` dan mengirimkan email konfirmasi HTML resmi dengan lampiran berkas PDF Confirmation Voucher dan Faktur Pajak Sleman PBJT.
* **FR-HUB-07 (Inbound OTA Channel Webhook Ingestion):** Sistem wajib menyediakan endpoint `POST /api/v1/channel-events` untuk menerima notifikasi pemesanan dari mitra OTA, memverifikasi tanda tangan HMAC-SHA256, dan mengeksekusi dekremen stok kamar secara atomik di database PostgreSQL.
* **FR-HUB-08 (Outbound Channel Inventory Sync):** Sistem wajib mempublikasikan event penyesuaian stok ke kanal luar ketika kamar terjual habis di website resmi hotel (*stop-sell push notification*).
* **FR-HUB-09 (Security & Anti-IDOR Isolation):** Endpoint SSE tamu wajib memvalidasi kepemilikan token sesi pemesanan tamu (`gst_sess_` atau `X-Guest-Token`). Tamu dilarang keras mendengarkan event reservasi milik tamu lain.

---

## 2. Spesifikasi Antarmuka HTTP & Event Stream (Contracts)

---

### 2.1 GET `/api/v1/guest/bookings/:id/live-status`
Membuka koneksi HTTP streaming Server-Sent Events (SSE) untuk memantau status pembayaran dan siklus hidup reservasi spesifik tamu.

* **Headers:**
  - `Authorization: Bearer gst_sess_<token>` (atau `X-Guest-Token: <token>`)
  - `Accept: text/event-stream`
* **Path Parameters:**
  - `id` (UUID, required): ID reservasi booking milik tamu.
* **Response Headers `200 OK`:**
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache, no-store, must-revalidate`
  - `Connection: keep-alive`
  - `Transfer-Encoding: chunked`
  - `X-Accel-Buffering: no` (Mencegah Nginx / Reverse Proxy mem-buffer stream)
* **Event Format (Text/Event-Stream):**
  ```text
  event: payment_confirmed
  data: {"status":"CONFIRMED","booking_id":"019234a5-b123-7000-8000-000000000001","reference":"PKU-202610-8849","voucher_url":"/api/v1/guest/bookings/019234a5-b123-7000-8000-000000000001/voucher.pdf","invoice_url":"/api/v1/guest/bookings/019234a5-b123-7000-8000-000000000001/invoice.pdf"}

  event: booking_expired
  data: {"status":"EXPIRED","booking_id":"019234a5-b123-7000-8000-000000000001","message":"Batas waktu pembayaran 30 menit telah habis."}

  event: ping
  data: {"time":"2026-10-04T10:20:00Z"}
  ```

---

### 2.2 GET `/api/v1/front-desk/live-stream`
Membuka koneksi HTTP streaming SSE operasional meja depan untuk staf resepsionis dan manajemen umum.

* **Headers:**
  - `Authorization: Bearer stf_jwt_<token>`
  - `Accept: text/event-stream`
* **Otorisasi RBAC:** Hanya role `receptionist` dan `gm_admin`.
* **Response Headers `200 OK`:**
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `Transfer-Encoding: chunked`
* **Event Format (Multi-Topic Events):**
  ```text
  event: booking_created
  data: {"booking_id":"019234a5-b123-7000-8000-000000000001","reference":"PKU-8849","guest_name":"Budi Santoso","room_type":"Superior King","check_in":"2026-10-10","check_out":"2026-10-12","source":"WEB_DIRECT"}

  event: room_status_updated
  data: {"room_id":"01900000-0000-7000-8000-000000000101","room_number":"302","status":"CLEAN","updated_by":"Housekeeping Agus"}

  event: channel_conflict
  data: {"provider":"TRAVELOKA","external_reference":"TRV-9901","room_type":"Executive Suite","conflict_reason":"ALLOTMENT_EXHAUSTED"}
  ```

---

### 2.3 POST `/api/v1/channel-events`
Menerima webhook pemesanan kamar atau pembaruan dari mitra OTA / Channel Manager.

* **Headers:**
  - `Content-Type: application/json`
  - `X-Channel-Provider: TRAVELOKA` (atau `AGODA`, `BOOKING_COM`, `SITEMINDER`)
  - `X-Channel-Signature: t=1728000000,v1=f4678e5ec5c4427ba2dda9e60b5724fd` (HMAC-SHA256)
  - `Idempotency-Key: evt_trv_20261004_99218`
* **Request Payload `application/json`:**
  ```json
  {
    "provider": "TRAVELOKA",
    "event_id": "evt_trv_20261004_99218",
    "event_type": "reservation_created",
    "external_reference": "TRV-2026-99218",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-15",
    "check_out": "2026-10-17",
    "rooms": 1,
    "guest_name": "Siti Rahmawati",
    "guest_email": "siti.rahma@example.com",
    "guest_phone": "+6281234567890",
    "total_payout_idr": 1850000
  }
  ```
* **Response `202 Accepted`:**
  ```json
  {
    "status": "ACCEPTED",
    "event_id": "evt_trv_20261004_99218",
    "message": "Event telah diterima dan sedang diproses secara asinkron",
    "timestamp": "2026-10-04T10:20:00Z"
  }
  ```
* **Error Response `400 Bad Request` / `401 Unauthorized` / `409 Conflict`:**
  - `INVALID_SIGNATURE`: Tanda tangan HMAC tidak valid atau telah dimodifikasi.
  - `ALLOTMENT_UNAVAILABLE`: Stok kamar untuk rentang tanggal tersebut sudah habis di hotel.

---

### 2.4 GET `/api/v1/staff/channel-sync-issues`
Melihat riwayat anomali atau insiden konflik sinkronisasi inventaris kanal eksternal.

* **Headers:** `Authorization: Bearer stf_jwt_<token>`
* **Otorisasi RBAC:** Role `revenue_mgr`, `gm_admin`.
* **Response `200 OK`:**
  ```json
  {
    "issues": [
      {
        "id": "019234a5-c999-7000-8000-000000000001",
        "provider": "AGODA",
        "external_reference": "AGD-88219",
        "event_type": "reservation_created",
        "room_type_id": "01900000-0000-7000-8000-000000000002",
        "status": "QUARANTINED_CONFLICT",
        "reason": "Kamar sold out di website hotel 2 menit sebelum event diterima",
        "created_at": "2026-10-04T02:15:00Z"
      }
    ]
  }
  ```

---

## 3. Spesifikasi Hirarki Subjek NATS JetStream

Stream: `HOSPITALITY_EVENTS`  
Storage: `FileStorage`  
Retention: `LimitsPolicy`  
Max Age: `7 days`  
Duplicates Window: `5 minutes`

| Subjek NATS | Diterbitkan Oleh | Dikonsumsi Oleh |
| :--- | :--- | :--- |
| `hospitality.booking.<booking_id>.created` | Core Booking | Front Desk SSE |
| `hospitality.booking.<booking_id>.confirmed` | Payment Webhook / Fake-Pay | Guest SSE, Front Desk SSE, WhatsApp Worker, Email Worker, OTA Sync Worker |
| `hospitality.booking.<booking_id>.expired` | Expiry Worker | Guest SSE, Front Desk SSE |
| `hospitality.booking.<booking_id>.cancelled` | Cancellation Engine | Guest SSE, Front Desk SSE, OTA Sync Worker |
| `hospitality.room.<room_id>.status_changed` | Housekeeping Service | Front Desk SSE |
| `hospitality.channel.<provider>.received` | Channel Webhook Importer | Channel Processing Worker, Front Desk SSE |
