# Software Requirements Specification (SRS) — Batch BE-E: Keandalan Operasional, Konkurensi & Observabilitas
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status Dokumen:** **APPROVED / READY FOR IMPLEMENTATION**
- **Referensi Desain:** [`docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-E01: Parallel Conflict-Free Physical Room Allocation (`BE-G17`)
- **Deskripsi:** Saat resepsionis memicu endpoint `POST /api/v1/bookings/{id}/check-in`, query pemilihan kamar fisik bebas pada tabel `rooms` WAJIB mengunci baris kandidat dengan `FOR UPDATE OF r SKIP LOCKED`.
- **Perilaku:**
  1. Bila dua atau lebih transaksi check-in berjalan berbarengan untuk tipe kamar yang sama, worker transaksi A mengunci kamar fisik terendah (misal: 101).
  2. Transaksi B melewati (*skip*) kamar 101 dan secara otomatis mengunci kamar fisik bebas berikutnya (misal: 102).
  3. Keduanya berhasil melakukan assign kamar tanpa terjadi *exclusion violation* (SQLSTATE 23P01).
  4. Bila jumlah kamar fisik bebas yang berhasil dikunci kurang dari `count`, transaksi dibatalkan (*rollback*) dan mengembalikan `ErrNoRoomAvailable` (`409 Conflict`).

### FR-E02: Early Check-Out Inventory Restitution (`BE-G22`)
- **Deskripsi:** Saat tamu melakukan check-out lebih awal daripada tanggal yang tertera pada reservasi (`today < scheduled_checkout`), sistem wajib mengembalikan ketersediaan kamar untuk sisa rentang tanggal `[today, scheduled_checkout)` ke tabel `inventory_calendar`.
- **Perilaku:**
  1. Jika check-out dilakukan tepat waktu atau setelah tanggal berakhir, ketersediaan kamar tidak diubah (sudah half-open terpakai).
  2. Jika check-out dilakukan sebelum tanggal berakhir (misal stay 5 malam, check-out di hari ke-2), panggil `tx.Increment(ctx, roomTypeID, today, checkOutDate, numRooms)`.
  3. Emit event outbox `booking.checked_out` dengan menyertakan payload `early_checkout: true` dan jumlah malam yang dikembalikan.

### FR-E03: No-Show Cutoff Time Enforcement (`BE-G22`)
- **Deskripsi:** Status *no-show* hanya dapat diberlakukan terhitung sejak tanggal check-in (jam 14:00 WIB).
- **Perilaku:**
  1. Jika resepsionis memanggil `POST /api/v1/bookings/{id}/no-show` sebelum tanggal check-in tiba (`today < check_in`), tolak dengan kode error `400 Bad Request` (`NO_SHOW_TOO_EARLY`).
  2. Jika dipanggil pada atau setelah tanggal check-in, set status ke `no_show`, kembalikan sisa inventaris kamar mulai dari tanggal hari ini hingga check-out, dan kirim event outbox `booking.no_show`.

### FR-E04: Robust Outbox Processing & Dead-Letter Isolation (`BE-G16`)
- **Deskripsi:** Worker outbox relay harus tangguh terhadap koneksi terputus dan kegagalan terminal.
- **Perilaku:**
  1. Validasi `interval` saat relay dijalankan: jika `interval <= 0`, gunakan default 5 detik (mencegah *ticker panic*).
  2. Periksa error pada `tx.QueryRow` dan tangani `pgx.ErrNoRows` sebagai queue kosong tanpa membuang error infrastruktur lainnya.
  3. Saat retry mencapai `MaxAttempts` (default 8 kali), update status outbox menjadi `failed` (*dead letter*) dan log sebagai error terisolasi tanpa memblokir pemrosesan antrian event berikutnya.

### FR-E05: Sweep & Configuration Observability (`BE-G21`)
- **Deskripsi:** Background worker sweep expired hold wajib memeriksa `rows.Err()` pasca iterasi cursor.
- **Perilaku:**
  1. Jika cursor query mengalami diskoneksi di tengah jalan, log error terminal dan hentikan rilis hold untuk menghindari pemrosesan data parsial.
  2. Validasi konfigurasi platform `Config.Validate()` untuk memastikan nilai `HoldTimeout`, `OutboxInterval`, dan URL database valid.
  3. Graceful shutdown mengkoordinasikan penghentian HTTP server dan asynq worker runner secara teratur dengan timeout 10 detik.

---

## 2. Kontrak Error Code HTTP Baru

| HTTP Status | Error Code | Keterangan Bisnis |
|---|---|---|
| `400 Bad Request` | `NO_SHOW_TOO_EARLY` | Reservasi belum mencapai tanggal check-in; gunakan pembatalan (*cancellation*) jika tamu batal datang sebelum hari H. |
| `409 Conflict` | `NO_ROOM_AVAILABLE` | Tidak ada kamar fisik bebas yang memenuhi kriteria alokasi atau seluruh kamar sedang dikunci. |
| `409 Conflict` | `ALREADY_CHECKED_OUT` | Reservasi sudah berstatus checked-out. |
