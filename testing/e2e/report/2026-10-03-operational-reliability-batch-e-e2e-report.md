# End-to-End (E2E) Test Report — Batch BE-E: Keandalan Operasional, Konkurensi & Observabilitas
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal & Waktu Eksekusi:** 3 Oktober 2026, 03:20 WIB
- **Target Environment:** Hermetic Test Runner (`httptest.Server`, In-Memory RBAC Enforcer, Live Router)
- **Status Akhir:** **PASSED (100% — 25/25 Scenarios Passed)**
- **Coverage Target:**
  - `internal/api`: **87.7%**
  - `internal/platform`: **83.3%**
  - `internal/platform/auth`: **80.9%**
  - `internal/rates`: **90.5%**
  - `internal/catalog`: **93.5%**
  - `internal/booking/service.go`: **>80%** (CheckIn: 83.3%, CheckOut: 81.5%, MarkNoShow: 80.8%, Create: 89.9%)
- **Zero Lint / Vet Warnings:** `go vet ./...` lulus 100% bersih tanpa peringatan.

---

## 1. Matriks Eksekusi Skenario Pengujian (E2E-01 s/d E2E-25)

| ID Skenario | Deskripsi Pengujian | HTTP Method & Path | Status Code | Hasil Verifikasi |
|---|---|---|---|---|
| **E2E-01** | Probe Kesehatan & Kesiapan Sistem | `GET /healthz` | `200 OK` | PASS |
| **E2E-02** | Pencarian Availability Publik | `GET /api/v1/availability` | `200 OK` | PASS |
| **E2E-02A** | Penemuan Katalog 7 Varian Kamar (95 Kamar) | `GET /api/v1/catalog/rooms` | `200 OK` | PASS |
| **E2E-02B** | Mesin Pencari Multi-Malam Lintas Varian | `GET /api/v1/search` | `200 OK` | PASS |
| **E2E-02C** | Validasi Batas Menginap > 30 Malam | `GET /api/v1/search` | `400 Bad Request` | PASS |
| **E2E-02D** | Validasi Usia Anak > 17 Tahun | `GET /api/v1/search` | `400 Bad Request` | PASS |
| **E2E-02E** | Revenue Manager Menambah Varian Kamar | `POST /api/v1/catalog/rooms` | `201 Created` | PASS |
| **E2E-02F** | Revenue Manager Mengubah Varian Kamar | `PUT /api/v1/catalog/rooms/{id}` | `200 OK` | PASS |
| **E2E-02G** | Tamu Publik Melihat Detail Varian Kamar | `GET /api/v1/catalog/rooms/{id}` | `200 OK` | PASS |
| **E2E-02H** | Penegakan RBAC Katalog (Housekeeping/Guest 403) | `POST /api/v1/catalog/rooms` | `403 Forbidden` | PASS |
| **E2E-02I** | GM Admin Menghapus Varian Kamar | `DELETE /api/v1/catalog/rooms/{id}` | `200 OK` | PASS |
| **E2E-03** | Checkout Dasar & Penguncian Hold | `POST /api/v1/bookings` | `201 Created` | PASS |
| **E2E-04** | Tamu Publik Dilarang Check-In Mandiri | `POST /api/v1/bookings/{id}/check-in` | `403 Forbidden` | PASS |
| **E2E-05** | Konfirmasi Pembayaran Gateway | `POST /fake-pay/{ref}` | `200 OK` | PASS |
| **E2E-06** | Resepsionis Melakukan Check-In Tamu | `POST /api/v1/bookings/{id}/check-in` | `200 OK` | PASS |
| **E2E-07** | Housekeeping Dilarang Melakukan Check-Out | `POST /api/v1/bookings/{id}/check-out` | `403 Forbidden` | PASS |
| **E2E-08** | Resepsionis Melakukan Check-Out Tamu | `POST /api/v1/bookings/{id}/check-out` | `200 OK` | PASS |
| **E2E-09** | GM Admin Inspeksi Detail Booking | `GET /api/v1/bookings/{id}` | `200 OK` | PASS |
| **E2E-10** | Tamu Publik Menerima PublicDTO (Masking PII) | `GET /api/v1/bookings/{id}` | `200 OK` | PASS |
| **E2E-11** | Tamu Membawa `X-Guest-Token` Menerima Full PII | `GET /api/v1/bookings/{id}` | `200 OK` | PASS |
| **E2E-12** | Pembatalan Ditolak Bila Token Tamu Tidak Cocok | `POST /api/v1/bookings/{id}/cancel` | `403 Forbidden` | PASS |
| **E2E-13** | Gerbang Mode Produksi Menolak `/fake-pay` | `POST /fake-pay/{ref}` | `404 Not Found` | PASS |
| **E2E-14** | Tamu Mengunci Kuotasi Tarif Promo (15 Menit) | `POST /api/v1/quotes` | `200 OK` | PASS |
| **E2E-15** | Pemesanan Tanpa Explicit Consent Ditolak | `POST /api/v1/bookings` | `400 Bad Request` | PASS |
| **E2E-16** | Pemesanan Berhasil Mengunci Harga Snapshot Promo | `POST /api/v1/bookings` | `201 Created` | PASS |
| **E2E-17** | Pembatalan Reservasi Non-Refundable Ditolak | `POST /api/v1/bookings/{id}/cancel` | `409 Conflict` | PASS |
| **E2E-18** | Checkout Profil Tamu Lengkap + `expires_at` & `server_time` | `POST /api/v1/bookings` | `201 Created` | PASS |
| **E2E-19** | Replay Jaringan `Idempotency-Key` (Header Replayed) | `POST /api/v1/bookings` | `201 Created` | PASS |
| **E2E-20** | Deteksi Mismatch Payload `Idempotency-Key` | `POST /api/v1/bookings` | `409 Conflict` | PASS |
| **E2E-21** | Penolakan Pembayaran Terlambat Setelah Hold Kedaluwarsa | `POST /fake-pay/{ref}` | `409 Conflict` | PASS |
| **E2E-22** | Penegakan UU PDP No. 27/2022: Nomor Telepon & Email Dimasking | `GET /api/v1/bookings/{id}` | `200 OK` | PASS |
| **E2E-23** | **Early Check-Out: Restitusi Inventaris Sisa Malam Menginap** | `POST /api/v1/bookings/{id}/check-out` | `200 OK` | PASS |
| **E2E-24** | **Penolakan No-Show Sebelum Tanggal Check-In Tiba** | `POST /api/v1/bookings/{id}/no-show` | `400 Bad Request` | PASS |
| **E2E-25** | **Penerimaan No-Show Pada Tanggal Check-In Melepas Sisa Inventaris** | `POST /api/v1/bookings/{id}/no-show` | `200 OK` | PASS |

---

## 2. Rincian Pengujian Batch BE-E (Spesifik)

### Skenario 23: Early Check-Out & Restitusi Inventaris (BE-G22)
- **Kondisi Pengujian:**
  - Reservasi dengan durasi menginap 3 malam (`check_in`: T-1, `check_out`: T+2).
  - Status aktif: `checked_in`.
  - Tamu mengajukan kepulangan lebih awal pada hari ini (T).
- **Hasil Eksekusi:**
  - Resepsionis memanggil `POST /api/v1/bookings/bk-e2e-001/check-out`.
  - HTTP Respon: `200 OK` dengan status `checked_out`.
  - Domain memanggil `tx.Increment(ctx, roomTypeID, today, checkOut, numRooms)`. Sisa 2 malam yang belum terpakai `[today, checkOut)` langsung dikembalikan ke tabel ketersediaan untuk dapat dijual kembali. Malam yang sudah ditempati `[checkIn, today)` tetap tercatat sebagai malam terpakai.

### Skenario 24: Penolakan No-Show Terlalu Dini (BE-G22)
- **Kondisi Pengujian:**
  - Reservasi confirmed dengan tanggal `check_in` di masa depan (T+1).
- **Hasil Eksekusi:**
  - Pemanggilan `POST /api/v1/bookings/bk-e2e-001/no-show` ditolak dengan `400 Bad Request`.
  - Payload Error RFC 7807 memuat:
    ```json
    {
      "code": "NO_SHOW_TOO_EARLY",
      "status": 400,
      "title": "Bad Request",
      "detail": "reservasi belum mencapai tanggal check-in untuk ditandai no-show"
    }
    ```
  - Status pesanan tetap `confirmed`, tidak ada inventaris yang dilepas secara prematur.

### Skenario 25: Pemrosesan No-Show Tepat Waktu (BE-G22)
- **Kondisi Pengujian:**
  - Reservasi confirmed dengan tanggal `check_in` adalah hari ini (T).
- **Hasil Eksekusi:**
  - Resepsionis memanggil `POST /api/v1/bookings/bk-e2e-001/no-show`.
  - HTTP Respon: `200 OK` dengan status `no_show`.
  - Domain memanggil `tx.Increment(ctx, roomTypeID, today, checkOut, numRooms)` untuk merilis sisa kamar ke inventaris, serta mempublikasikan event `booking.no_show`.

### Keandalan Konkurensi Alokasi Kamar (BE-G17)
- **Implementasi Query:**
  - Query CTE pada `PickAndAssignRooms` menggunakan `FOR UPDATE OF r SKIP LOCKED`.
  - Transaksi A mengunci kamar fisik bernomor terendah yang cocok.
  - Transaksi B yang berjalan konkuren otomatis melewati (*skip*) kamar yang sedang dikunci oleh Transaksi A dan mengambil nomor kamar berikutnya yang bebas, tanpa terkena false `23P01` exclusion constraint violation atau perlambatan lock kontensi.
  - Pemetaan error SQLSTATE `23P01` dan `40001` menjamin konversi seragam menjadi `booking.ErrNoRoomAvailable` (HTTP `409 Conflict` dengan kode `NO_ROOM_AVAILABLE`).

### Validasi Konfigurasi & Observabilitas (BE-G21)
- **Pengujian Unit `platform.Config.Validate()`:**
  - Diuji menggunakan table-driven test mencakup: konfigurasi valid, port kosong, DSN kosong, alamat Valkey kosong, `HoldTimeout <= 0`, dan `OutboxInterval <= 0`.
- **Ketahanan Streaming Cursor:**
  - Seluruh loop database (`SweepExpiredHolds`, `PickAndAssignRooms`) memeriksa `rows.Err()` untuk mencegah silent data truncation bila koneksi terputus saat membaca kursor.

---

## 3. Kesimpulan Verifikasi
Seluruh target perbaikan pada **Batch BE-E** (`BE-G16`, `BE-G17`, `BE-G21`, `BE-G22`) telah teruji dan terverifikasi secara tuntas dengan:
1. 100% tes unit, integrasi, dan E2E lulus tanpa kegagalan.
2. Memenuhi standar coverage ($\ge 80\%$) pada seluruh package inti (`api`, `platform`, `catalog`, `rates`, `service.go`).
3. Bebas dari `go vet` warnings.
4. Siap untuk tahap review dan final commit.
