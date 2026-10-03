# E2E Report — Checkout Integrity (BE-R06, BE-R08)

Waktu: 3 Okt 2026 13:35 WIB. Skrip: [checkout_integrity_e2e.sh](../script/checkout_integrity_e2e.sh). Dokumen: [PRD](../../../docs/prd/checkout-integrity-r06-r08-2026-10-03.md) · [SRS](../../../docs/srs/checkout-integrity-r06-r08-2026-10-03.md) · [Tech](../../../docs/tech/checkout-integrity-architecture-2026-10-03.md).

## Environment
Postgres 18 dan Valkey 8 sementara (migrasi 1–15), server dibangun dari working tree dan dijalankan di `:28080` tanpa kunci Xendit/Resend (fake gateway, `APP_ENV=development`). Seluruh container dan proses dihapus setelah uji. Container `current-booking-*` milik pengguna tidak disentuh.

## Hasil: PASS=15 FAIL=0

| Skenario | Sebelum (audit 13:26) | Sesudah |
|---|---|---|
| Create tanpa `quote_id` | 201, total 1.500.000 tanpa pajak/consent | **400 `QUOTE_REQUIRED`** |
| Quote tanpa consent | 400 | 400 `CONSENT_REQUIRED` |
| Quote tidak dikenal | — | 410 `QUOTE_EXPIRED` |
| 20 request paralel, key sama | 8 request → 4 booking | **1 booking**, sisanya 201 replay atau 409 |
| Replay setelah selesai | 201 | 201 + `Idempotency-Replayed: true` |
| Key sama, body beda | 409 | 409 `IDEMPOTENCY_CONFLICT` |
| Create gagal lalu retry key sama | — | key dilepas, retry 201 |
| fake-pay → check-in → check-out; guest check-in | 200/200/200; 403 | 200/200/200; 403 (tidak regresi) |

Verifikasi DB setelah uji: tepat 1 baris `bookings` dan 1 `payment_attempts` per skenario (check_in 2026-11-24 dan 2026-11-26).

## Uji otomatis
- `go vet ./...`: bersih.
- `go test -race -count=1 -cover ./...` dengan `TEST_DATABASE_URL` ke DB sementara: semua paket lulus, termasuk 6 skenario real-DB (5 lama + `TestRealDB_IdempotencyReserveAtomic`: 20 `Reserve` paralel menghasilkan tepat 1 pemilik).
- Coverage: `internal/api` 84.4%, `internal/booking` 54.9% (naik dari 52.1%, masih di bawah 80% sejak sebelum perubahan), `testing/integration` 76.7%.

## Perubahan perilaku yang perlu diketahui klien
- `POST /api/v1/bookings` kini wajib `quote_id` dan consent; tanpa quote ditolak 400.
- Request dengan key yang sedang diproses mendapat 409 `IDEMPOTENCY_IN_PROGRESS` dengan `Retry-After: 1`. Client cukup retry dengan key dan body identik.
- Store idempotency yang gagal memberi 503 `IDEMPOTENCY_UNAVAILABLE`, bukan lagi diam-diam membuat booking baru.
- Satu quote masih dapat dipakai membuat lebih dari satu booking selama belum kedaluwarsa. Ini di luar scope dan belum diperbaiki.

## Risiko residual
- Jika proses mati setelah booking commit tetapi sebelum `Complete`, reservasi kedaluwarsa dalam 2 menit dan retry dapat membuat booking kedua.
- Fake gateway dan identitas staf tidak berubah: **BE-R01 dan BE-R16 masih OPEN**. Skrip ini sendiri memakai `Bearer receptionist` untuk check-in/out, yaitu mekanisme yang terbukti dapat dipalsukan.

## Tindak lanjut (13:40) — hal yang sebelumnya dicatat sebagai risiko
- **Satu quote = satu booking.** Migrasi `00016_unique_booking_quote.sql` menambah unique index `uq_bookings_quote_id`; percobaan kedua dengan quote yang sama → 409 `QUOTE_ALREADY_USED`, stok tidak bocor (`TestRealDB_QuoteSingleUse`). Ini juga menutup risiko residual R08: bila reservasi idempotency kedaluwarsa sebelum respons tercatat, retry tidak dapat membuat booking kedua. Konsekuensi: setelah create gagal di sisi payment, klien harus meminta quote baru.
- **Integration test aman.** `db_helper.go` tidak lagi memakai DSN hardcode; wajib `TEST_DATABASE_URL`, dan database harus bernama `*_test` (override `ALLOW_DESTRUCTIVE_TESTS=1`). Tanpa env test di-skip.
- **Coverage `internal/booking`:** unit saja 55.9%; gabungan unit + real-DB (`-coverpkg=./internal/booking`) **82.8%**. Lapisan SQL hanya terbukti lewat real-DB test.
- Migrasi harus dijalankan sebelum deploy; jika data lama berisi `quote_id` ganda, migrasi gagal (aman, tidak merusak data).
