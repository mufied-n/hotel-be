# Product Requirements Document (PRD)
# Real-Time Hospitality Event Hub: NATS JetStream, Live Front Desk SSE, Multi-Channel Webhooks & Notifier
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `PRD-F10-F11-F07-REALTIME-EVENT-HUB-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - SRS: [`docs/srs/realtime-hospitality-event-hub-and-channel-sync-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/realtime-hospitality-event-hub-and-channel-sync-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/realtime-hospitality-event-hub-and-channel-sync-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/realtime-hospitality-event-hub-and-channel-sync-architecture-2026-10-04.md)
  - Walkthrough Tracking: [`docs/walkthrough/realtime-hospitality-event-hub-and-channel-sync-walkthrough-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/realtime-hospitality-event-hub-and-channel-sync-walkthrough-2026-10-04.md)
- **Target Role Pengguna:** `guest`, `receptionist`, `housekeeping`, `revenue_mgr`, `finance`, `gm_admin`, `ota_partner`

---

## 1. Latar Belakang & Urgensi Bisnis

Hotel **Pulang ke Uttara** adalah hotel butik bintang 4 dengan 95 kamar di Jl. Kaliurang Km 5, Sleman, Yogyakarta. Hotel ini melayani tamu direct booking (web resmi hotel) dan tamu kanal eksternal (Online Travel Agent / OTA seperti Traveloka, Tiket.com, Agoda, Booking.com).

Saat ini, sistem pemesanan telah memiliki fungsionalitas inti: katalog kamar, kuotasi harga, dynamic rates & stop-sell (Kandidat A), serta generator PDF Voucher & Faktur Pajak Sleman PBJT (Kandidat B). Namun, seluruh interaksi pasca-transaksi dan koordinasi operasional masih bersifat **statik / polling**:

### 1.1 Masalah Operasional Saat Ini (Pain Points)
1. **Kecemasan Tamu pada Layar Pembayaran (*Guest Payment Anxiety*):**
   Saat tamu membayar via QRIS atau Virtual Account di ponsel mereka, layar laptop/web tamu tidak mengetahui status pembayaran secara instan tanpa melakukan refresh manual atau polling berulang yang membebani server.
2. **Keterlambatan Konfirmasi ke Tamu (*Delayed Guest Communication*):**
   Tamu Indonesia mengharapkan konfirmasi instan di WhatsApp mereka sesaat setelah membayar. Ketiadaan pengiriman otomatis WhatsApp dan Email dengan lampiran e-voucher memaksa tamu menghubungi hotel atau mengecek email secara berkala.
3. **Inefisiensi Meja Depan (*Front Desk Blind Spots*):**
   Staf resepsionis harus terus-menerus menekan tombol F5 (*Refresh*) di layar komputer mereka untuk mengetahui pemesanan baru yang masuk dari web atau kanal OTA. Hal ini menimbulkan keterlambatan dalam alokasi kamar dan penyambutan tamu.
4. **Bahaya Overbooking Lintas Kanal (*Zero-Latency Channel Sync Gap*):**
   Ketika kamar terakhir laku di website hotel pada malam hari, ketiadaan mekanisme push webhook ke kanal luar membuat kamar tersebut masih terbuka di OTA selama beberapa menit/jam, memicu risiko *double-booking* fatal.

### 1.2 Dampak Bisnis yang Diharapkan (Expected Outcomes)
* **Zero Guest Anxiety:** Layar pembayaran tamu secara otomatis beralih (*instant green check*) dalam waktu $\le 50\text{ ms}$ setelah webhook pembayaran terkonfirmasi via SSE.
* **100% Pengiriman Multi-Kanal Instan:** Notifikasi WhatsApp ramah beserta tautan langsung ke PDF Voucher A4 dan email HTML berlampiran PDF faktur pajak diterima tamu dalam waktu $\le 3\text{ detik}$.
* **Live Front Desk Ops:** Dashboard meja depan diperbarui secara *real-time* via SSE tanpa reload browser, mencakup kedatangan baru, status kebersihan kamar dari housekeeping, dan peringatan konflik kuota.
* **Zero Double-Booking:** Penutupan kuota otomatis (*stop-sell push*) ke mitra OTA dalam waktu $\le 1\text{ detik}$ setelah kamar terakhir terjual.

---

## 2. Profil Persona & Matriks Kebutuhan

| Persona | Kebutuhan Utama | Fitur Terkait |
| :--- | :--- | :--- |
| **`guest` (Tamu Web)** | Menunggu konfirmasi pembayaran QRIS/VA secara otomatis di layar, menerima pesan WhatsApp ringkas dengan link PDF voucher, dan email resmi. | Guest SSE Stream (`/guest/bookings/:id/live-status`), WA & Email Dispatcher |
| **`receptionist` (Meja Depan)** | Melihat daftar tamu datang secara live, menerima bunyi alert saat ada booking baru, memantau kamar siap huni dari housekeeping. | Front Desk Live SSE Stream (`/front-desk/live-stream`) |
| **`housekeeping`** | Memperbarui status kebersihan kamar (Dirty $\rightarrow$ Clean $\rightarrow$ Inspected), otomatis tersiar ke resepsionis. | Room Status Event Broadcaster |
| **`revenue_mgr`** | Mengontrol ketersediaan lintas kanal secara otomatis, mencegah overbooking saat high-season. | Two-Way Channel Webhook & Outbox Sync |
| **`ota_partner`** | Mengirimkan reservasi dari OTA ke hotel via webhook, menerima pembaruan kuota ketersediaan. | Inbound Webhook (`/channel-events`) & Outbound Inventory Notification |

---

## 3. Matriks Otorisasi & Hak Akses (Casbin RBAC Matrix)

| Endpoint | Method | Role yang Diizinkan | Deskripsi Otorisasi |
| :--- | :---: | :--- | :--- |
| `/api/v1/guest/bookings/:id/live-status` | `GET` | `guest` (pemilik sesi) | Streaming SSE status real-time pembayaran dan reservasi tamu (Anti-IDOR). |
| `/api/v1/front-desk/live-stream` | `GET` | `receptionist`, `gm_admin` | Streaming SSE siaran operasional seluruh hotel bagi staf meja depan. |
| `/api/v1/channel-events` | `POST` | `ota_partner` (Signed HMAC) | Menerima event pemesanan masuk dari mitra OTA / Channel Manager. |
| `/api/v1/staff/channel-sync-issues` | `GET` | `revenue_mgr`, `gm_admin` | Melihat daftar insiden konflik atau kegagalan sinkronisasi kanal. |

---

## 4. Kebutuhan Produk & Acceptance Criteria (PRD-AC)

### AC-01: Guest Real-Time Payment SSE Stream
- **Deskripsi:** Saat tamu membuka halaman pembayaran, browser membuka koneksi SSE ke `/api/v1/guest/bookings/:id/live-status`.
- **Kriteria Penerimaan:**
  1. Koneksi SSE mengirim header `Content-Type: text/event-stream`, `Cache-Control: no-cache`.
  2. Saat pembayaran lunas (`CONFIRMED`), server langsung mem-push event `payment_confirmed` memuat nomor referensi, status, dan link redirect dalam waktu $\le 100\text{ ms}$.
  3. Saat batas waktu hold 30 menit kedaluwarsa, server mem-push event `booking_expired`.
  4. Akses ke ID reservasi orang lain tanpa token sesi yang sah wajib ditolak HTTP 403/404 (Anti-IDOR).

### AC-02: Front Desk Live Operational SSE Stream
- **Deskripsi:** Resepsionis membuka satu koneksi SSE di `/api/v1/front-desk/live-stream`.
- **Kriteria Penerimaan:**
  1. Hanya staf berotorisasi (`receptionist`, `gm_admin`) yang dapat membuka stream.
  2. Menerima event siaran multi-topik: `booking_created`, `booking_confirmed`, `room_cleaned`, `channel_booking_received`.
  3. Heartbeat komentar HTTP (`:\n\n`) dikirim setiap 15 detik untuk menjaga koneksi tetap aktif di balik proxy.

### AC-03: Durable Multi-Channel Notification Dispatcher (WA & Email)
- **Deskripsi:** Setiap pemesanan berstatus `CONFIRMED` otomatis memicu pengiriman pesan WhatsApp dan Email.
- **Kriteria Penerimaan:**
  1. Pesan WhatsApp diformat ramah berbahasa Indonesia dengan rincian tipe kamar, tanggal menginap, nama tamu, dan tautan e-voucher PDF.
  2. Email konfirmasi berformat HTML responsif memuat rincian biaya, lampiran file PDF Voucher, dan Faktur Pajak Sleman PBJT.
  3. Menggunakan pola *Durable Pull Consumer* NATS JetStream dengan retry otomatis dan backoff eksponensial jika provider eksternal mengalami gangguan.

### AC-04: Two-Way Channel Webhook & Inventory Protection
- **Deskripsi:** Menghubungkan inventaris hotel dengan mitra OTA secara *real-time*.
- **Kriteria Penerimaan:**
  1. Endpoint `POST /api/v1/channel-events` menerima webhook reservasi masuk, memvalidasi tanda tangan HMAC, dan mengeksekusi dekremen stok kamar secara atomik di PostgreSQL.
  2. Jika kuota kamar habis di database lokal, event penutupan kuota (*stop-sell*) otomatis dipublikasikan ke kanal eksternal.
  3. Jika terjadi konflik kuota (stok = 0), event dikarantina dengan status `QUARANTINED_CONFLICT` dan alert langsung dikirim ke SSE Front Desk.

---

## 5. Analisis Anti-Overengineering (Ponytail Review)

1. **Pemanfaatan NATS JetStream:** Menggunakan satu container resmi `nats:2.10-alpine` super ringan (~20MB RAM) yang mengintegrasikan Pub/Sub instan dan penyimpanan pesan durable dalam satu protokol, tanpa memerlukan antrean terpisah seperti Kafka, Celery, atau RabbitMQ.
2. **SSE Bawaan Gin (Zero WebSocket Bloat):** Untuk komunikasi server-ke-klien, Server-Sent Events (SSE) menggunakan protokol HTTP/1.1 murni bawaan browser (`EventSource`) tanpa perlu runtime WebSocket kompleks atau protokol upgrade.
3. **Reuse Aset Dokumen:** Menggunakan generator `maroto/v2` PDF Voucher & Tax Invoice yang sudah teruji di internal tanpa memanggil dependensi baru.
