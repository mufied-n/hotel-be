# Product Requirements Document (PRD) — Batch BE-C: Tarif, Paket, Quote Engine & Kebijakan Pembatalan
**Fitur:** Dynamic Rate Plans, Money Contract, 15-Minute Quote Lock Engine, & Cancellation Policy Enforcement  
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `PRD-BATCH-C-2026-10-03`  
**Status:** Approved for Implementation  
**Target Rilis:** Q4 2026  
**Referensi Gap Audit:** `BE-G04`, `BE-G05`, `BE-G06`, `BE-G08`, `BE-G19` ([`docs/gap/01-catalog-search-pricing.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/01-catalog-search-pricing.md), [`docs/gap/02-checkout-policy.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/02-checkout-policy.md))

---

## 1. Latar Belakang & Masalah Bisnis

Pada arsitektur awal pemesanan kamar Pulang ke Uttara:
1. **Tidak Ada Pilihan Paket / Rate Plan (`BE-G04`)**: Tamu hanya mendapatkan satu harga tunggal tanpa opsi memilih paket *Room Only (RO)* vs *Bed & Breakfast (BB)*. Promo vendor seperti `OCTOBREAK` tidak dapat diaplikasikan.
2. **Kontrak Moneter Rentan Kesalahan Multi-Mata Uang & Pajak (`BE-G05`, `BE-G19`)**: Penggunaan field ambigu `RateMinor` tanpa kejelasan mata uang (`IDR`), eksponen nol, serta ketiadaan rincian pajak PB1 (10% PBJT perhotelan Yogyakarta), biaya layanan, dan diskon.
3. **Harga Berubah di Tengah Transaksi Checkout Tanpa Izin (`BE-G06`)**: Endpoint pencarian dan booking create menghitung harga secara terpisah. Jika Revenue Manager mengubah harga kamar saat tamu sedang mengisi form data diri, total harga melonjak saat bayar tanpa review persetujuan tamu.
4. **Kebijakan Pembatalan Tidak Ditegakkan & Persetujuan Syarat Tamu Lemah (`BE-G08`)**: Semua pesanan berstatus *confirmed* dapat dibatalkan begitu saja oleh tamu publik dan stok dikembalikan, melanggar kebijakan tarif *Non-Refundable*. Persetujuan syarat (*terms & conditions consent*) tidak disimpan di database.

---

## 2. Persona Pengguna & Matriks Kebutuhan

| Persona | Peran | Kebutuhan Utama |
| :--- | :--- | :--- |
| **Guest (Tamu Publik)** | Pengunjung Website | Memilih antara paket Room Only dan Bed & Breakfast; memasukkan kode promo resmi; melihat rincian harga transparan (harga dasar, sarapan, diskon, pajak 10%); mendapatkan kepastian harga terkunci selama 15 menit proses checkout; mengetahui kebijakan pembatalan kamar secara tegas. |
| **Revenue Manager** | Manajemen Tarif | Mengatur selisih harga sarapan (Rp 100.000 / orang / malam); mengelola kode promo aktif (misal `OCTOBREAK` diskon 15%); menentukan rate plan mana yang bersifat *Non-Refundable* vs *Flexible 48-Hour*. |
| **Receptionist / Front Desk** | Operasional Staf | Melihat riwayat rincian tagihan (subtotal, sarapan, diskon promo, pajak PB1) dan status kebijakan pembatalan tamu saat check-in. |
| **Finance / GM Admin** | Keuangan & Kepatuhan | Memiliki bukti persetujuan syarat (*consent timestamp*), kepatuhan pajak daerah Yogyakarta PB1 10%, dan pencegahan *chargeback* pembatalan ilegal. |

---

## 3. Matriks Fitur & Hak Akses

| ID Fitur | Deskripsi | Guest | Receptionist | Revenue Mgr | GM Admin |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **FT-C01** | Pilih Rate Plan (`room_only` vs `bed_breakfast`) | **R** | **R** | **R/W** | **R/W** |
| **FT-C02** | Validasi & Aplikasi Promo Code (`OCTOBREAK`) | **R** | **R** | **R/W** | **R/W** |
| **FT-C03** | Rincian Struktur Uang (`Money` IDR, Pajak 10% PB1) | **R** | **R** | **R** | **R** |
| **FT-C04** | Quote Lock Engine (15-menit TTL token harga) | **R/W** | **R** | **R** | **R** |
| **FT-C05** | Penegakan Kebijakan Pembatalan (*Non-Refundable* vs *Flexible*) | **R** | **R/W** | **R** | **R/W** |
| **FT-C06** | Validasi Persetujuan Syarat & Kebijakan Privasi | **W** | **R** | **R** | **R** |

---

## 4. Kriteria Keberhasilan (Acceptance Criteria)

1. **AC-C01 (Rate Plans & Sarapan)**:
   - Pencarian atau kalkulasi tarif mendukung parameter `rate_plan_code` (`room_only` atau `bed_and_breakfast`).
   - Paket `bed_and_breakfast` otomatis menambahkan biaya sarapan resmi Rp 100.000 / dewasa / malam ke komponen sarapan.
2. **AC-C02 (Kode Promo `OCTOBREAK`)**:
   - Kode promo valid `OCTOBREAK` memotong 15% dari harga kamar sebelum pajak.
   - Kode promo yang kedaluwarsa atau salah ditolak dengan pesan error yang jelas dan harga kembali ke normal.
3. **AC-C03 (Kontrak Moneter Eksplisit `Money`)**:
   - Mata uang selalu `"IDR"`, eksponen selalu `0`.
   - Rumus aritmatika integer wajib berlaku tepat tanpa pembulatan mengambang:
     $$\text{Total} = \text{Subtotal Kamar} + \text{Biaya Sarapan} - \text{Diskon Promo} + \text{Pajak PB1 (10\%)}$$
4. **AC-C04 (Quote Lock Engine TTL 15 Menit)**:
   - Setiap permintaan quote menghasilkan `quote_id` bertipe UUIDv7 dengan masa berlaku 15 menit (`expires_at = now + 15m`).
   - Endpoint pembuatan reservasi `POST /api/v1/bookings` wajib menyertakan `quote_id`.
   - Jika `quote_id` kedaluwarsa, API menolak reservasi dengan kode `QUOTE_EXPIRED` (HTTP 410) dan menginstruksikan tamu untuk mereview harga terbaru.
5. **AC-C05 (Penegakan Kebijakan Pembatalan & Persetujuan Consent)**:
   - Tamu wajib menyertakan `terms_accepted: true` dan `privacy_accepted: true`. Bila `false`, kembalikan `HTTP 400 Bad Request` (`CONSENT_REQUIRED`).
   - Jika pesanan memiliki kebijakan `non_refundable`, tamu ditolak saat membatalkan (`HTTP 400/409 NON_REFUNDABLE_BOOKING`).
   - Jika pesanan berstatus `flexible_48h`, pembatalan gratis hanya diizinkan sebelum $H-2$ jam 14:00 WIB.

---

## 5. Non-Functional Requirements (NFR)

1. **Integritas Moneter Integer**: Tidak ada representasi floating point (`float64` atau `float32`) dalam penyimpanan atau kalkulasi uang untuk menghindari cacat sen/rupiah ganda.
2. **Performa Mesin Quote**: Pembuatan quote in-memory/cache membutuhkan waktu $\le 5$ milidetik.
3. **Pemberian Waktu Seragam**: Zona waktu operasional terkunci pada `Asia/Jakarta` (WIB, UTC+7).
