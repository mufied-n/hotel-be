# Product Requirements Document (PRD) — Pemulihan Tautan Pembayaran Lintas Sesi (Payment Link Recovery) (BE-R15)

**Nomor Dokumen:** PRD-PULANG-BE-R15-2026-10-03  
**Target Rilis:** v1.0.0-rc1  
**Status:** Approved  
**Author:** AI Engineering & Product Agent  
**Terkait:** [`BE-R15`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md), BE-G11, BE-G12, F03, F05  

---

## 1. Konteks Bisnis & Latar Belakang

Hotel *Pulang ke Uttara* (Yogyakarta, 95 kamar) menyediakan layanan reservasi daring independen. Saat tamu melakukan pemesanan kamar (*checkout*), sistem menerbitkan reservasi berstatus `pending` dengan jaminan kamar (*hold*) selama 30 menit dan membuat tagihan pembayaran (*charge/invoice*) ke gateway Xendit. 

Pada implementasi sebelumnya:
1. Tautan pembayaran (`payment_url`) dan referensi tagihan hanya dikembalikan sekali (*one-shot*) saat pemanggilan `POST /api/v1/bookings`.
2. Jika tamu me-refresh browser, mengalami putus koneksi, berpindah perangkat (misal dari desktop ke smartphone), atau kembali ke portal "Booking Saya" (`/my-bookings`), backend belum menyediakan endpoint pemulihan (*recovery*) untuk mendapatkan tautan pembayaran invoice yang sama.
3. Meskipun atribut `allowed_actions.can_pay` bernilai `true`, antarmuka pengguna (Frontend) tidak dapat mengarahkan tamu ke kanal pembayaran karena ketiadaan `payment_url`.
4. Jika tamu mencoba menekan tombol bayar berulang kali tanpa mekanisme resolusi cerdas, sistem berisiko menerbitkan tagihan ganda (*phantom duplicate invoices*) yang membingungkan tamu dan menyulitkan rekonsiliasi finansial.

---

## 2. Persona Pengguna & Matriks Otorisasi

| Persona | Kebutuhan Akses | Kewenangan |
| :--- | :--- | :--- |
| **Tamu Pemesan (`guest`)** | Mengakses kembali tautan pembayaran untuk reservasi pending miliknya setelah berpindah sesi/perangkat. | Akses privat via `X-Guest-Token` atau cookie sesi tamu terautentikasi (`guest_session`). Dilarang mengakses reservasi tamu lain (IDOR defense). |
| **Resepsionis / Staf (`receptionist`, `gm_admin`)** | Membantu tamu yang mengalami kendala pembayaran dengan menyalin/mengirimkan kembali tautan invoice aktif. | Akses via Bearer token staf dengan izin baca reservasi. |
| **Tamu Anonim / Pihak Ketiga** | Mencoba menebak ID booking untuk melihat invoice. | **Ditolak tegas (HTTP 404 / 403)**. |

---

## 3. Matriks Status Reservasi vs Akses Tautan Pembayaran

| Status Booking | Kondisi Hold | Ketersediaan Payment URL | Kode Respons HTTP |
| :--- | :--- | :--- | :--- |
| **`pending`** | Masih berlaku (`now < expires_at`) | **Tersedia** (Reused invoice URL dari attempt aktif) | HTTP 200 OK |
| **`pending`** | Kedaluwarsa (`now >= expires_at`) | **Ditolak** (Hold kedaluwarsa, kamar dilepas) | HTTP 410 Gone / 409 Conflict (`HOLD_EXPIRED`) |
| **`confirmed`** | Sudah lunas | **Ditolak** (Tagihan telah diselesaikan) | HTTP 409 Conflict (`BOOKING_NOT_PENDING`) |
| **`cancelled`** | Dibatalkan | **Ditolak** (Reservasi telah dibatalkan) | HTTP 409 Conflict (`BOOKING_NOT_PENDING`) |
| **`checked_in` / `checked_out`** | Operasional selesai | **Ditolak** | HTTP 409 Conflict (`BOOKING_NOT_PENDING`) |

---

## 4. Kriteria Keberterimaan (Acceptance Criteria)

- **AC-01: Resolusi Tautan Pembayaran Eksisting (Idempotent Recovery)**  
  Sistem mengambil `payment_url` dari buku besar percobaan pembayaran (`payment_attempts`) yang aktif tanpa membuat tagihan/invoice baru ke gateway. Jumlah entri *attempts* tidak bertambah saat operasi pembacaan (GET).
- **AC-02: Perlindungan Akses & IDOR Defense**  
  Endpoint `GET /api/v1/bookings/:id/payment` dan `GET /api/v1/guest/bookings/:id` hanya dapat diakses oleh pemilik sah reservasi (melalui verifikasi token kepemilikan `guest_token`, cookie sesi `guest_session`, atau otentikasi staf resmi). Percobaan akses tanpa token sah menghasilkan respons HTTP 404 Not Found atau HTTP 403 Forbidden.
- **AC-03: Penegakan Batas Waktu Hold (Strict Hold Expiration Enforcement)**  
  Jika reservasi berstatus `pending` namun waktu server telah melampaui `expires_at`, permintaan pemulihan tautan pembayaran ditolak dengan HTTP 410 Gone dan kode error `HOLD_EXPIRED`.
- **AC-04: Konsistensi Antarmuka Booking Portal Tamu (`BookingDetail`)**  
  Pada DTO `BookingDetail` (`/api/v1/guest/bookings/:id`), field `payment_url` terisi otomatis jika dan hanya jika `allowed_actions.can_pay` bernilai `true`. Jika `can_pay` bernilai `false`, field `payment_url` kosong/omitted.
- **AC-05: Fallback Otomatis saat Invoice Awal Belum Terbit**  
  Jika reservasi masih dalam batas hold tetapi percobaan pembayaran sebelumnya mengalami gateway timeout sebelum menerima URL, sistem memicu penerbitan invoice gateway secara aman dan memperbarui attempt record tanpa membuat duplikasi inventaris kamar.

---

## 5. Kebutuhan Non-Fungsional (NFR)

1. **Performa:** Waktu respons endpoint pemulihan tautan pembayaran < 50ms untuk pembacaan cache/database lokal.
2. **Keamanan Finansial:** Tidak mengekspos PAN, CVV, atau kredensial rahasia gateway ke antarmuka pengguna; hanya URL checkout resmi gateway Xendit yang dikembalikan.
3. **Auditability:** Setiap upaya penerbitan ulang atau resolusi tagihan tercatat dalam log audit terstruktur (`slog`).
