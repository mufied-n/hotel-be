# Product Requirements Document (PRD) — Batch BE-D: Checkout, Idempotensi & Pemulihan Pembayaran
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** Draft disetujui untuk implementasi TDD
- **Target Parity Gap:** `BE-G07`, `BE-G09`, `BE-G11`, `BE-G12`
- **Target Properti:** Hotel Bintang 4 Pulang ke Uttara (95 Kamar Fisik)

---

## 1. Konteks Bisnis & Masalah

Pada audit paritas Codex terhadap alur checkout, ditemukan empat risiko finansial dan pengalaman pengguna:
1. **Kelengkapan Profil Tamu (`BE-G07`):** Payload checkout sebelumnya hanya menampung nama dan email. Resepsionis dan sistem operasional hotel bintang 4 membutuhkan nomor telepon aktif (WhatsApp/SMS konfirmasi saat kedatangan), perkiraan jam check-in (*estimated arrival time*), dan permintaan khusus (*special requests*, misal: lantai tinggi, bantal non-alergi) yang dibatasi maksimal 500 karakter dengan penafian (*disclaimer*) bahwa permintaan khusus tidak digaransi.
2. **Double Booking Akibat Gangguan Koneksi (`BE-G09`):** Ketika tamu menekan tombol pembayaran pada koneksi seluler tidak stabil, kegagalan jaringan setelah commit database memicu tombol ditekan berulang kali atau browser melakukan *network retry*. Tanpa mekanisme *Idempotency Key*, sistem membuat multi-reservasi yang menyedot kuota kamar dan menduplikasi tagihan pembayaran.
3. **Audit Jejak Pembayaran & Rekonsiliasi (`BE-G11`):** Transaksi gateway belum dicatat ke dalam buku besar (*payment attempts ledger*). Kegagalan verifikasi status pembayaran atau pengulangan webhook dari penyedia pembayaran dapat memicu transisi status ilegal atau menyulitkan rekonsiliasi keuangan hotel.
4. **Pencegahan Overbooking Pasca Hold Expired (`BE-G12`):** Waktu hold kamar (standar 30 menit) belum diverifikasi secara mutlak saat konfirmasi pembayaran tiba. Jika tamu membayar setelah batas waktu habis dan kamar telah dilepas ke inventaris publik serta dibeli orang lain, penerimaan pembayaran yang terlambat dapat memicu *overbooking* fatal pada unit kamar ke-95.

---

## 2. Persona Pengguna

1. **Tamu Publik (`guest`):**
   - Menikmati alur checkout yang aman dari penagihan ganda meskipun terjadi retry jaringan.
   - Dapat memasukkan kontak telepon E.164 dan permintaan khusus (maks 500 karakter).
   - Mengetahui batas waktu pasti hold pembayaran (`expires_at` dan `server_time`).

2. **Front Desk / Resepsionis (`receptionist`):**
   - Menerima informasi kontak tamu dan perkiraan jam kedatangan untuk mempersiapkan kamar.
   - Melihat catatan *special request* pada saat tamu tiba di lobi.

3. **Manajer Keuangan (`finance`):**
   - Memiliki audit trail transaksi lengkap pada tabel `payment_attempts` (ID referensi gateway, jumlah nominal integer, status, dan riwayat webhook).
   - Mengetahui status *late payment* (pembayaran setelah hold expired) untuk penanganan manual atau pengembalian dana (*refund*).

---

## 3. Matriks Kebutuhan & Penerimaan (Acceptance Criteria)

| ID | Fitur | Kriteria Penerimaan |
| :--- | :--- | :--- |
| **AC-D01** | Data Tamu Lengkap | Menerima `guest_phone` (format E.164), `estimated_arrival_time` (format HH:MM), dan `special_requests` (string $\le 500$ karakter). Field divalidasi dan tersimpan di DB. |
| **AC-D02** | Idempotency Key | Menerima header `Idempotency-Key`. Jika key sama & payload hash identik: kembalikan cached response 201 Created tanpa membuat booking baru. Jika key sama & payload beda: tolak dengan 409 Conflict `IDEMPOTENCY_CONFLICT`. |
| **AC-D03** | Payment Attempt Ledger | Setiap percobaan pembayaran gateway dicatat di tabel `payment_attempts` dengan status `initiated`, `success`, atau `failed`. Webhook pembayaran di-deduplikasi berdasarkan event ID. |
| **AC-D04** | Otoritas Mutlak Hold Expiry | Response create booking menyertakan `expires_at` dan `server_time`. Konfirmasi pembayaran memeriksa `now <= expires_at`. Jika terlambat, pembayaran ditolak dengan `409 HOLD_EXPIRED` dan dialihkan ke rekonsiliasi. |

---

## 4. Analisis Anti-Overengineering (Ponytail)
- Idempotency store diimplementasikan menggunakan tabel PostgreSQL `idempotency_keys` sederhana yang diintegrasikan dalam transaksi DB lokal, tanpa dependensi library eksternal.
- Validasi format telepon E.164 menggunakan regex Go standar tanpa pustaka libphonenumber pihak ketiga.
