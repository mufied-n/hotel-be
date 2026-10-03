# Product Requirements Document (PRD): Ketahanan Timeout Gateway Pembayaran & Integritas Buku Besar (BE-R13)

**Nomor Dokumen:** PRD-PULANG-BE-R13  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Stakeholder Terkait:** Finance & Accounting, Hotel Front Desk, DevOps / SRE, Hotel Guests  
**Terkait Audit & Gap:** `BE-R13`, `BE-G11`, `BE-G12`, `F05`, `F14`

---

## 1. Latar Belakang & Konteks Bisnis

Pada sistem perhotelan bintang 4 (*Pulang ke Uttara, Yogyakarta*), integrasi dengan payment gateway eksternal (seperti Xendit Invoice v2) adalah titik temu penting antara sistem pemesanan hotel dan aliran finansial perbankan.

Berdasarkan audit teknis `BE-R13`:
1. **Penanganan Ambiguitas Gateway Timeout (*Outcome Unknown*):**
   Sebelum perbaikan, seluruh kegagalan saat pemanggilan `payment.CreateCharge` diperlakukan secara seragam sebagai kegagalan definitif (`failed`), lalu sistem langsung membatalkan reservasi (`s.Cancel`) dan mengembalikan kamar ke stok ketersediaan publik.
   Jika penyedia gateway sebenarnya telah sukses menerbitkan invoice namun respons HTTP mengalami *timeout* atau *network disruption* ke server hotel, tamu mungkin tetap menerima notifikasi invoice/virtual account dari bank atau Xendit. Ketika tamu membayar, kamar hotel telah dibatalkan dan berpotensi dijual ke orang lain (*overbooking / double-booking disaster*).
2. **Pencatatan Buku Besar Tidak Atomik & Hilangnya Jejak Audit:**
   Percobaan pembayaran (*payment attempt*) baru dicatat setelah panggilan gateway selesai. Jika panggilan timeout atau DB error, error `RecordAttempt` dan `UpdateAttemptStatus` diabaikan secara diam-diam (`_ = ...`).
3. **Pembaruan Massal Berdasarkan `booking_id`:**
   Metode pembaruan status percobaan pembayaran memperbarui seluruh baris dengan `booking_id`, mengaburkan riwayat status per attempt ketika satu pemesanan memiliki lebih dari satu percobaan pembayaran.

Oleh karena itu, diperlukan pemisahan ketat antara **Definitive Failure** dan **Outcome Unknown (Timeout)**, pencatatan *intent* sebelum pemanggilan gateway, pembaruan attempt spesifik berbasis `attempt_id`, serta perlindungan stok kamar saat terjadi timeout.

---

## 2. Persona Pengguna & Kebutuhan

| Persona | Kebutuhan Utama | Nilai Tambah Fitur |
| :--- | :--- | :--- |
| **Tamu Hotel (`guest`)** | Tidak kehilangan kamar yang sudah di-hold saat terjadi gangguan koneksi sementara pada payment gateway. | Reservasi tetap berstatus *held/pending* selama masa hold (30 menit); tamu dapat memverifikasi atau mencoba ulang pembayaran tanpa kamar terlepas. |
| **Finance & Accounting** | Rekonsiliasi buku besar pembayaran yang akurat dan audit trail yang tidak pernah hilang. | Setiap upaya transaksi memiliki `attempt_id` unik dengan status jelas (`initiated`, `success`, `failed`, `unknown_timeout`, `received_after_expiry`). |
| **Front Desk / Receptionist** | Kepastian status reservasi dan tidak terjadi *overbooking* akibat stok dilepas secara salah. | Mencegah pembatalan sepihak atas transaksi yang invoice-nya sudah terbentuk di gateway. |

---

## 3. Matriks Aturan Bisnis & Spesifikasi Perilaku

1. **Pembedaan Klasifikasi Error Gateway:**
   - **Outcome Unknown / Timeout:** Mencakup `context.DeadlineExceeded`, HTTP timeout, DNS/network connection reset, dan HTTP status 502/503/504 dari gateway.
     - *Perilaku:* Reservasi **TIDAK DIBATALKAN**. Stok kamar **TIDAK DILEPAS**. Percobaan dicatat dengan status `unknown_timeout`. API merespons dengan HTTP 504 `GATEWAY_TIMEOUT`.
   - **Definitive Failure:** Mencakup HTTP 400 Bad Request, kartu invalid, parameter ditolak oleh gateway secara pasti.
     - *Perilaku:* Percobaan dicatat dengan status `failed`. Kompensasi pembatalan `s.Cancel` dijalankan. Jika kompensasi gagal, dicatat peringatan kritis ke audit log. API merespons dengan HTTP 502 `PAYMENT_FAILED`.
2. **Pencatatan Intent Pra-Panggilan:**
   - Sebelum request dikirimkan ke gateway eksternal, record `PaymentAttempt` dibuat dengan `status: "initiated"` dan `ID` berbasis UUID v7.
3. **Pembaruan Berbasis Attempt ID Spesifik:**
   - Setiap mutasi status attempt wajib merujuk langsung ke `id` (primary key baris `payment_attempts`), bukan hanya `booking_id`.
4. **Propagasi & Logging Error Ledger:**
   - Seluruh kegagalan penyimpanan ke `payment_attempts` wajib dicatat ke log terstruktur (`log/slog`) dengan level ERROR dan tidak boleh disembunyikan.

---

## 4. Kriteria Keberhasilan (Acceptance Criteria)

- [x] **AC-01:** Ketika gateway mengalami timeout (atau simulated network timeout), reservasi tetap berstatus `pending` dan inventory kamar tetap tertahan (tidak dirilis kembali ke publik).
- [x] **AC-02:** Percobaan pembayaran pada kondisi timeout tercatat di buku besar `payment_attempts` dengan status `unknown_timeout`.
- [x] **AC-03:** Ketika gateway menolak pembayaran secara definitif (4xx), sistem menjalankan kompensasi pembatalan reservasi dan mencatat status `failed`.
- [x] **AC-04:** Kegagalan pembaruan attempt ledger dicatat secara eksplisit ke dalam sistem log.
- [x] **AC-05:** Pengujian unit dan automated E2E test memverifikasi seluruh alur timeout, definitive failure, dan attempt tracking dengan pass 100%.
