# PRD — F04: Booking Confirmation Artifacts (Printable Invoice & iCalendar .ics)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Spesifikasi:
- **SRS Pasangan:** [SRS-F04-Artifacts](../srs/booking-confirmation-artifacts-f04-2026-10-03.md)
- **Tech Architecture:** [TECH-F04-Artifacts](../tech/booking-confirmation-artifacts-f04-architecture-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F04](../walkthrough/booking-confirmation-artifacts-f04-walkthrough-2026-10-03.md)

---

## 1. Latar Belakang & Konteks Bisnis

Setelah tamu menyelesaikan pembayaran melalui payment gateway (F05) dan dapat mengakses reservasi mereka secara mandiri melalui "Booking Saya" (F03), tamu membutuhkan dua jenis artefak konfirmasi resmi:
1. **Printable Invoice & Booking Voucher (Kuitansi Resmi & Bukti Reservasi):**
   * Tamu membutuhkan dokumen resmi yang memuat rincian pemesanan, perincian tagihan (kamar, sarapan, diskon promo, pajak daerah PB1 10%, service charge), status **PAID (Lunas)**, dan nomor invoice resmi.
   * Dokumen ini sangat esensial bagi **corporate travellers / tamu dinas** untuk kebutuhan klaim reimbursement kantor, serta sebagai bukti reservasi cetak saat datang ke meja resepsionis (*front desk*).
2. **Sinkronisasi Kalender Digital (iCalendar `.ics` RFC 5545):**
   * Tamu membutuhkan kemudahan menambahkan jadwal menginap secara otomatis ke aplikasi kalender mereka (Google Calendar, Apple Calendar di iOS/macOS, Microsoft Outlook).
   * File `.ics` memuat waktu check-in (14:00 WIB), check-out (12:00 WIB), alamat lengkap Pulang ke Uttara, nomor kontak resepsionis, dan notifikasi pengingat (*VALARM*) 24 jam sebelum check-in.

---

## 2. Profil Persona & Hak Akses

| Persona | Hak Akses | Kebutuhan Utama |
|---|---|---|
| **Tamu Terverifikasi (`guest`)** | Sesi terautentikasi (F02/F03) | Mengunduh invoice resmi & kalender `.ics` untuk reservasinya sendiri. Dilarang mengakses reservasi tamu lain (anti-IDOR). |
| **Resepsionis (`receptionist`)** | RBAC Staf Casbin | Melihat rincian invoice / voucher tamu untuk memvalidasi kedatangan saat check-in. |
| **Finance (`finance`)** | RBAC Staf Casbin | Mengaudit invoice resmi dan memastikan rincian pembayaran cocok dengan mutasi kas/gateway. |
| **General Manager (`gm_admin`)** | Full Access | Audit menyeluruh seluruh artefak konfirmasi. |
| **Publik Tanpa Sesi** | Ditolak (HTTP 401) | Mencegah kebocoran data pribadi (UU PDP No. 27/2022). |

---

## 3. Fitur & Aturan Bisnis (Business Rules)

1. **BR-F04-01: Status Kelayakan Penerbitan Artefak (Eligibility Gate):**
   * Printable Invoice dan file iCalendar **hanya dapat diterbitkan** jika reservasi berstatus `confirmed`, `checked_in`, atau `checked_out`.
   * Jika reservasi masih `pending`, sistem menolak permintaan dengan pesan bahwa invoice resmi hanya tersedia setelah pembayaran diverifikasi.
   * Jika reservasi `cancelled` atau `expired`, invoice resmi tidak dapat diterbitkan sebagai bukti bayar sah.
2. **BR-F04-02: Format Nomor Invoice Resmi:**
   * Nomor invoice berformat stabil: `INV/PKU/<YYYYMM>/<REFERENCE>`. Contoh: `INV/PKU/202610/PKU-20261003-8F2A`.
3. **BR-F04-03: Kepatuhan Standar RFC 5545 (iCalendar):**
   * Timezone: `Asia/Jakarta` (WIB, UTC+7).
   * Event Start: Tanggal Check-in pukul 14:00 WIB.
   * Event End: Tanggal Check-out pukul 12:00 WIB.
   * UID Event bersifat stabil dan deterministik: `booking-<id>@pulangkeuttara.id`.
   * Reminder Alarm (`VALARM`): 1 hari sebelum check-in (`TRIGGER:-P1D`).
4. **BR-F04-04: Kepatuhan Privasi & Anti-IDOR (UU PDP No. 27/2022):**
   * Endpoint guest memvalidasi kesesuaian antara email sesi tamu dan email pada data reservasi.
   * Jika booking ID milik email lain, sistem mengembalikan HTTP 404 (bukan 403) untuk mencegah enumerasi ID.
   * Header respons menyertakan `Cache-Control: no-store, private` agar dokumen tidak disimpan pada intermediate caching proxies.

---

## 4. Kriteria Keberterimaan (Acceptance Criteria)

- [x] **AC-F04-01:** Endpoint `GET /api/v1/guest/bookings/{id}/receipt` mengembalikan payload JSON DTO lengkap memuat info hotel, data menginap, rincian tamu, rincian kamar, breakdown harga (room, breakfast, discount, PB1, total), dan status pembayaran.
- [x] **AC-F04-02:** Endpoint `GET /api/v1/guest/bookings/{id}/calendar.ics` menghasilkan stream berkas `.ics` dengan mime-type `text/calendar; charset=utf-8` dan content-disposition attachment.
- [x] **AC-F04-03:** Berkas `.ics` mematuhi spesifikasi RFC 5545 (CRLF line endings, SUMMARY, DESCRIPTION, LOCATION, DTSTART/DTEND Asia/Jakarta, dan VALARM -P1D).
- [x] **AC-F04-04:** Permintaan invoice / kalender pada reservasi berstatus `pending` menghasilkan HTTP 400 `RECEIPT_NOT_AVAILABLE`.
- [x] **AC-F04-05:** Permintaan invoice / kalender dari sesi tamu terhadap booking ID milik orang lain menghasilkan HTTP 404 `BOOKING_NOT_FOUND`.
- [x] **AC-F04-06:** Tanpa header `X-Guest-Session`, seluruh endpoint privat menghasilkan HTTP 401 `UNAUTHORIZED`.
- [x] **AC-F04-07:** Implementasi mengikuti prinsip **Ponytail**: Pure Go standard library (`strings.Builder`, `time`), zero external dependencies, performa tinggi, dan mudah dirawat.
