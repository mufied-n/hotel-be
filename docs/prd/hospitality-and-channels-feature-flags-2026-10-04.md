# PRD: Hospitality & Multi-Channel Feature Flags System
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Fitur ID:** `FEAT-HOSPITALITY-FEATURE-FLAGS`
- **Tanggal:** 4 Oktober 2026
- **Status:** APPROVED / IN IMPLEMENTATION
- **Target Pengguna:** General Manager (`gm_admin`), Revenue Manager (`revenue_mgr`), Staf Front Office (`receptionist`), IT/DevOps

---

## 1. Latar Belakang & Konteks Bisnis

Seiring bertambahnya kapabilitas sistem pemesanan Pulang ke Uttara (95 kamar) dengan integrasi kanal pihak ketiga (OTA Agoda/Booking.com), streaming real-time Server-Sent Events (SSE), generator dokumen resmi PDF (e-voucher QR code dan faktur PBJT 10% Sleman), perlindungan stok kamar terakhir (LRDA safety buffer & dynamic hold), dan notifikasi WhatsApp modular, hotel membutuhkan **mekanisme kendali operasional terpusat (*kill-switch*)**.

Dalam skenario operasional nyata di hotel bintang 4:
1. **Pemeliharaan / Gangguan Kanal OTA:** Jika sistem mitra OTA (misal Agoda) mengalami *bug* atau banjir reservasi anomali, manajemen hotel membutuhkan tombol darurat (*emergency kill-switch*) untuk mematikan penerimaan webhook kanal tanpa mematikan seluruh server web hotel.
2. **Lonjakan Beban Jaringan (Network Saturation):** Jika koneksi internet hotel melambat saat musim puncak (*peak season*), streaming SSE dapat dinonaktifkan sementara dan diarahkan kembali ke mode hemat *bandwidth*.
3. **Penyelarasan Kalender Tarif:** Revenue Manager memerlukan wewenang mengaktifkan atau menonaktifkan fitur modifikasi tarif massal (*bulk calendar update*) saat audit harga harian.
4. **Proteksi Kamar Terakhir & Resolusi Upgrade:** Fitur resolusi overbooking meja depan dapat dikontrol secara dinamis berdasarkan kebijakan manajemen.
5. **Peralihan Notifikasi WhatsApp:** Pengiriman pesan otomatis WhatsApp dapat dihentikan sementara jika kuota gateway habis tanpa mengganggu alur konfirmasi pembayaran tamu.

---

## 2. Matriks Feature Flags Baru

| Flag Key | Nama Fitur | Deskripsi Operasional | Default | Peran Diizinkan (`allowed_roles`) | Endpoint Terkait |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `ff_dynamic_rates_calendar` | Dynamic Rates & Stop-Sell Calendar | Kalender tarif musiman, batas stop-sell, dan alokasi stok massal | `TRUE` | `revenue_mgr`, `gm_admin` | `GET/PUT /api/v1/revenue/calendar*` |
| `ff_official_pdf_voucher` | Official PDF Voucher & PBJT Invoice | Penerbitan PDF A4 e-voucher QR code dan faktur pajak daerah PBJT 10% | `TRUE` | `{}` (Publik / Tamu / Staf) | `GET /bookings/:id/*.pdf`, `/guest/bookings/:id/*.pdf` |
| `ff_realtime_event_hub` | Real-Time Live SSE Stream | Streaming real-time live status pembayaran tamu dan monitor meja depan | `TRUE` | `{}` (Tamu / Front Desk) | `GET /guest/bookings/:id/live-status`, `GET /front-desk/live-stream` |
| `ff_channel_sync_integration` | Multi-Channel OTA Sync | Penerimaan webhook masuk OTA dan pemantauan daftar isu karantina | `TRUE` | `{}` (Webhook publik & Staf) | `POST /channel-events`, `GET /staff/channel-*` |
| `ff_last_room_safeguards` | Last-Room Safeguards & Upgrade | Safety buffer LRDA, hold dinamis 15m, dan 1-click complimentary upgrade | `TRUE` | `receptionist`, `revenue_mgr`, `gm_admin` | `POST /staff/channel-sync-issues/:id/resolve` |
| `ff_whatsapp_notifier` | Modular WhatsApp Dispatcher | Pengiriman otomatis notifikasi konfirmasi pemesanan via WhatsApp | `TRUE` | `{}` (Background Worker) | Outbox Worker / EventBus Dispatcher |

---

## 3. Kriteria Keberhasilan & Acceptance Criteria

1. **AC-1 (Zero-Downtime Live Toggling):** GM Admin dapat mematikan atau mengaktifkan fitur secara instan via `PUT /api/v1/admin/feature-flags/:key` tanpa me-restart server backend.
2. **AC-2 (Graceful Degradation):** Ketika suatu flag bernilai `false`, permintaan HTTP pada endpoint terkait segera mengembalikan respons terstandarisasi **HTTP 503 Service Unavailable** dengan kode `FEATURE_DISABLED` dan pesan informatif.
3. **AC-3 (Role-Scoped Access Control):** Jika suatu flag memiliki batasan `allowed_roles` (misal `ff_dynamic_rates_calendar`), pengguna dengan peran di luar daftar tersebut ditolak **HTTP 403 Forbidden** meskipun flag berstatus `enabled: true`.
4. **AC-4 (Persistence & Audit Trail):** Seluruh status flag disimpan secara persisten di tabel PostgreSQL `feature_flags` dan mencatat `updated_by` serta `updated_at`.
