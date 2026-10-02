# Software Requirements Specification (SRS) — Batch BE-F: Verifikasi Konkurensi DB Nyata
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen ID:** `SRS-BATCH-F-REAL-DB-CONCURRENCY-2026-10-03`
- **Versi:** 1.0.0
- **Tanggal:** 3 Oktober 2026
- **Status:** **Approved / Ready for Implementation**
- **Referensi PRD:** [`docs/prd/real-db-concurrency-verification-batch-f-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/real-db-concurrency-verification-batch-f-2026-10-03.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-F01: Manajemen Lingkungan Uji Database Terisolasi (Isolated DB Fixture)
- **Deskripsi:** Sistem pengujian wajib mampu terhubung ke instance database PostgreSQL 18 nyata melalui DSN lingkungan (`TEST_DATABASE_URL` atau fallback Docker network IP `postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable`).
- **Prasyarat:** Database uji telah termigrasi utuh hingga migrasi Goose versi 7 (`00007_checkout_idempotency_ledger.sql`).
- **Pembersihan Data:** Setiap unit/integration test wajib membersihkan data transaksional sebelum dan setelah pengujian dijalankan (*clean fixture isolation*).

### FR-F02: Verifikasi Konkurensi Perebutan Kamar Terakhir (Race on Last Room)
- **Skenario:** 
  - Tipe kamar `01900000-0000-7000-8000-000000000001` (Deluxe Balcony King) diatur memiliki stok `available_rooms = 1` pada tanggal tertentu.
  - $N=20$ goroutine dijalankan secara bersamaan menggunakan `sync.WaitGroup` dan *synchronization barrier* untuk mengirim permintaan `InsertBookingWithHold`.
- **Ekspektasi Output:**
  - Tepat 1 transaksi berhasil mendapatkan hold dan mengembalikan `nil error`.
  - Tepat 19 transaksi lainnya menerima `inventory.ErrInsufficient` atau `booking.ErrInsufficient`.
  - Query verifikasi langsung ke database:
    `SELECT available_rooms FROM inventory WHERE ...` harus menghasilkan tepat `0`, tidak boleh negatif.
  - Jumlah baris pada tabel `bookings` untuk batch tersebut tepat 1 baris.

### FR-F03: Verifikasi Atomicity Rollback pada Multi-Malam Parsial
- **Skenario:**
  - Tamu mengajukan booking untuk 3 malam berurutan ($T_1, T_2, T_3$).
  - Tanggal $T_1$ stok = 2, $T_2$ stok = 2, $T_3$ stok = 0.
  - Layanan menjalankan transaksi `InTx`.
- **Ekspektasi Output:**
  - Fungsi `LockAndDecrement` mengembalikan error pada iterasi malam ke-3.
  - Transaksi database PostgreSQL me-rollback seluruh operasi.
  - Query verifikasi langsung: stok pada $T_1$ dan $T_2$ tetap bernilai 2 (tidak ada *inventory leak*).
  - Tidak ada baris booking yang tercipta di tabel `bookings`.

### FR-F04: Verifikasi Alokasi Kamar Fisik Paralel dengan SKIP LOCKED (BE-G17)
- **Skenario:**
  - Terdapat 2 booking terkonfirmasi untuk tipe kamar Deluxe Balcony King pada rentang tanggal yang sama.
  - Terdapat 2 kamar fisik bebas: kamar 101 dan 102.
  - 2 goroutine mengeksekusi `PickAndAssignRooms` secara simultan.
- **Ekspektasi Output:**
  - Kedua transaksi berhasil (*status checked-in*).
  - Transaksi A mengunci kamar 101; Transaksi B otomatis melewati kamar 101 via `SKIP LOCKED` dan mengambil kamar 102.
  - Tidak ada error `23P01` atau deadlock.
  - Kedua kamar terdaftar rapi pada tabel `room_assignments`.

### FR-F05: Penolakan Pelanggaran Batasan Integritas GiST Exclusion (Physical Double Booking)
- **Skenario:**
  - Dua transaksi mencoba memasukkan baris ke `room_assignments` secara manual untuk nomor kamar fisik yang sama pada rentang tanggal menginap yang tumpang tindih (`daterange(check_in, check_out) && daterange(check_in, check_out)`).
- **Ekspektasi Output:**
  - Transaksi kedua wajib ditolak oleh PostgreSQL engine dengan error code `23P01` (*exclusion_violation*).
  - Membuktikan bahwa integritas fisik kamar tidak hanya dilindungi oleh kode Go, melainkan dijamin di level terendah database engine.

### FR-F06: Verifikasi Balapan Sweep Hold Kedaluwarsa vs Konfirmasi Pembayaran (BE-G12)
- **Skenario:**
  - Booking A berstatus pending dengan `expires_at = now() - 10s` (sudah kedaluwarsa).
  - Goroutine A menjalankan `Confirm(ctx, bookingID)`.
  - Goroutine B menjalankan `SweepExpiredHolds(ctx, pool, ...)`.
- **Ekspektasi Output:**
  - Jika `Confirm` berjalan, sistem memeriksa `time.Now().UTC().After(*b.ExpiresAt)` dan menolak konfirmasi dengan `ErrHoldExpired`.
  - Upaya pembayaran terlambat dicatat pada `payment_attempts` dengan status `received_after_expiry`.
  - Stok kamar dipulihkan ke tabel inventaris.

---

## 2. Struktur Pengujian Integrasi & Kontrak Kode

Suite pengujian disimpan di lokasi:
- Path: `testing/integration/postgres_concurrency_test.go`
- Skrip Runner: `testing/integration/run_integration.sh`
- Report Hasil: `testing/e2e/report/2026-10-03-real-db-concurrency-batch-f-e2e-report.md`
