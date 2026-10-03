# PRD — F14: Rekonsiliasi Finansial & Otomasi Gateway Refund (Xendit Refund API)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Terkait:
- **SRS Pasangan:** [SRS-F14-Finance](../srs/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **Tech Architecture:** [TECH-F14-Finance](../tech/finance-reconciliation-and-refunds-f14-architecture-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F14](../walkthrough/finance-reconciliation-and-refunds-f14-walkthrough-2026-10-03.md)

---

## 1. Latar Belakang & Urgensi Bisnis (Crucial Impact)

Modul keuangan dan refund adalah area yang paling krusial dalam sistem perhotelan karena menyangkut **integritas perputaran kas nyata hotel dan kepatuhan hukum konsumen**:
1. **Risiko Kehilangan Dana (*Over-Refund / Double-Refund*):**
   * Jika sistem mengizinkan refund tanpa serialisasi transaksi (*concurrency lock*), permintaan paralel atau retry jaringan dapat memicu pengembalian dana ganda (*double-refund*) yang menguras saldo kas hotel di payment gateway.
   * Total refund (penuh atau sebagian) tidak boleh melebihi jumlah uang yang telah diterima di invoice (`sum(refunds) <= captured_amount`).
2. **Penanganan Pembayaran Terlambat (*Late Payment Arrival*):**
   * Tamu sering kali membayar invoice setelah batas waktu *hold* (15–30 menit) kedaluwarsa. Uang terpotong di rekening tamu, namun kamar sudah otomatis dilepas (*auto-release*) dan mungkin telah dipesan oleh tamu lain.
   * Hotel membutuhkan sistem pencatatan kasus rekonsiliasi (*Payment Case*) yang memungkinkan staf keuangan dan GM untuk:
     a. Mengembalikan dana 100% ke tamu via Xendit Refund API secara instan dan terdokumentasi.
     b. Menerbitkan reservasi ulang jika kamar masih tersedia.
3. **Pemisahan Kewenangan (*Segregation of Duties & RBAC*):**
   * Resepsionis meja depan dilarang keras menerbitkan pengembalian dana. Hak refund hanya dimiliki oleh staf dengan role **`finance`** (sesuai batas kebijakan pembatalan) dan **`gm_admin`** (untuk *approval exception*).
   * Seluruh mutasi kas wajib memiliki jejak audit (*audit trail*): siapa yang mengeksekusi, nominal yang dikembalikan, alasan refund, dan ID referensi Xendit.

---

## 2. Profil Persona & Hak Akses (RBAC Matrix)

| Persona | Hak Akses | Batas Tanggung Jawab |
|---|---|---|
| **Staf Keuangan (`finance`)** | RBAC Staf | Memeriksa daftar mutasi rekonsiliasi, melihat payment case, mengeksekusi refund resmi sesuai batas kebijakan pembatalan kamar. |
| **General Manager (`gm_admin`)** | Full Control | Otoritas penuh menyetujui refund di luar kebijakan standar (*exception approval*), meninjau laporan audit rekonsiliasi kas. |
| **Resepsionis (`receptionist`)** | Read-Only | Hanya melihat status pembayaran dan status refund reservasi (tidak dapat memicu refund). |
| **Tamu Terverifikasi (`guest`)** | Sesi Privat | Hanya dapat membaca status pengembalian dana miliknya sendiri (anti-IDOR: `WHERE guest_email = session_email`). |

---

## 3. Aturan Bisnis Utama (Business Rules)

- **BR-F14-01: Verifikasi Batas Maksimal Refund (Over-Refund Prevention):**
  Jumlah kumulatif refund yang disetujui untuk satu pemesanan tidak boleh melebihi total pembayaran lunas (`total_price_minor`). Refund parsial diperbolehkan selama akumulasinya tidak melampaui tagihan awal.
- **BR-F14-02: Serialisasi Transaksi & Concurrency Lock:**
  Pengecekan sisa saldo yang dapat di-refund dan pemotongan saldo harus dieksekusi di dalam transaksi database dengan penguncian baris (`FOR UPDATE`) sebelum memanggil Xendit Refund API.
- **BR-F14-03: Idempotency Key Gateway:**
  Setiap pemanggilan refund ke Xendit wajib menyertakan `reference_id` deterministik unik (format: `rfnd-<booking_id>-<sequence>`). Retry jaringan akibat timeout tidak akan mendebit saldo gateway dua kali.
- **BR-F14-04: Late Payment Case Creation:**
  Ketika webhook Xendit berstatus `PAID` tiba untuk reservasi yang statusnya sudah `expired` (karena melewati batas *hold*), sistem tidak boleh otomatis membatalkan atau menelan dana tanpa jejak. Sistem wajib membuat `payment_case` bertipe `late_payment` berstatus `open` untuk ditindaklanjuti tim Finance.
- **BR-F14-05: Alasan Refund Wajib (Mandatory Reason):**
  Setiap permintaan refund wajib menyertakan alasan (`reason`) non-kosong, minimal 5 karakter, untuk kebutuhan audit akuntansi.

---

## 4. Kriteria Keberterimaan (Acceptance Criteria)

- [x] **AC-F14-01:** Skema database `payment_refunds` dan `payment_cases` dibuat melalui Goose migration dengan indeks performa dan audit actor.
- [x] **AC-F14-02:** Endpoint `POST /api/v1/finance/refunds` hanya dapat diakses oleh role `finance` dan `gm_admin`; role `receptionist` dan `guest` ditolak dengan HTTP 403 Forbidden.
- [x] **AC-F14-03:** Upaya refund pada pemesanan yang belum lunas atau melebihi nominal pembayaran ditolak dengan HTTP 400 Bad Request (`INVALID_REFUND_AMOUNT`).
- [x] **AC-F14-04:** Klien Xendit Gateway mendukung method `CreateRefund` yang memanggil `POST https://api.xendit.co/refunds` dengan format payload resmi Xendit, Basic Auth, dan penanganan timeout aman.
- [x] **AC-F14-05:** Permintaan webhook yang terlambat (*late payment*) pada booking yang sudah `expired` otomatis mencatat record di tabel `payment_cases`.
- [x] **AC-F14-06:** Endpoint `GET /api/v1/finance/cases` dan `POST /api/v1/finance/cases/{id}/resolve` memungkinkan staf Finance meninjau dan menyelesaikan sengketa pembayaran (refund / resolusi manual).
- [x] **AC-F14-07:** Endpoint `GET /api/v1/guest/bookings/{id}/refund-status` memungkinkan tamu melihat riwayat pengembalian dananya dengan proteksi kepemilikan anti-IDOR.
