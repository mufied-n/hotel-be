# Audit End-to-End — HEAD `3cabba3` (3 Okt 2026, 13:25–13:28 WIB)

## Environment
Stack terisolasi, dibuat khusus untuk audit dan sudah dihapus: Postgres 18 (`:25432`) dan Valkey 8 (`:26379`) baru. Migrasi 00001–00015 dijalankan lewat `cmd/migrate`, lalu server dibangun dari source dan dijalankan di `:28080` (`APP_ENV` default = development, tanpa kunci Xendit/Resend).
Container `current-booking-*` milik pengguna **tidak disentuh**.

> Catatan: `localhost:8080` adalah `docker-era-api-gateway` (proyek lain) dan `:5432`/`:6379` adalah DB/Redis proyek lain. Skrip di `testing/e2e/script/*.sh` memakai default `BASE_URL=http://localhost:8080`, jadi akan menembak layanan yang salah bila dijalankan tanpa override.

## Hasil

| # | Skenario | Hasil | Status |
|---|---|---|---|
| 1 | Migrasi 1–15 di DB kosong | semua OK | PASS |
| 2 | `/healthz`, `/ready` | 200 / 200 | PASS |
| 3 | Katalog 7 varian, search multi-malam, quote 15 menit (subtotal 1.500.000 + pajak 150.000 = 1.650.000) | sesuai | PASS |
| 4 | Create dengan quote tanpa consent | 400 `CONSENT_REQUIRED` | PASS |
| 5 | Create dengan quote + consent → payment_url, guest_token, terms_accepted_at | 201 | PASS |
| 6 | `GET /bookings/:id` tanpa token → DTO minimal; dengan token → penuh | sesuai | PASS |
| 7 | Replay key sama dengan body berbeda | 409 `IDEMPOTENCY_CONFLICT` | PASS |
| 8 | Pay (fake) → check-in → check-out | confirmed → checked_in (kamar 301) → checked_out | PASS |
| 9 | Check-in sebagai guest | 403 | PASS |
| 10 | `go test -race -count=1 ./...` x3 dengan DB nyata, 5 skenario concurrency | 0 gagal, 0 skip | PASS |
| 11 | **Create tanpa `quote_id` dan tanpa consent** | **201** | **FAIL** (BE-R06) |
| 12 | **8 request paralel, Idempotency-Key sama** | **8×201, 4 booking berbeda, 4 baris di DB** | **FAIL** (BE-R08) |
| 13 | **`Authorization: Bearer gm_admin` / `X-User-Role: finance` tanpa akun** | **200** (papan 95 kamar, ringkasan finance) | **FAIL** (BE-R01) |
| 14 | **Check-in dan check-out dengan Bearer/X-User-Role palsu** | **200** | **FAIL** (BE-R01) |
| 15 | `POST /fake-pay/<ref non-UUID>` | **500 dengan teks SQL mentah** | **FAIL** (kebocoran error) |

## Bukti temuan

**BE-R06 — create tanpa quote.** Booking `status=pending` terbentuk dengan `tax_minor: 0`, `total_price_minor: 1500000` (quote resmi 1.650.000), teks pembatalan generik "48 jam sebelum check-in" tanpa batas 14:00 WIB, dan tanpa `terms_accepted`. Inventory ikut terpakai. Harga jadi 10% lebih murah dan tanpa consent.

**BE-R08 — idempotency konkuren.** `SELECT count(*) FROM bookings WHERE guest_name='Idem'` = 4. Tiap booking mengurangi stok dan membuat hold dan outbox sendiri. Pada payment live, ini menjadi beberapa invoice untuk satu klik checkout.

**BE-R01 — identitas staf.** `curl -H "Authorization: Bearer gm_admin" /api/v1/housekeeping/rooms` → 200 (`total_rooms: 95`). `curl -H "X-User-Role: finance" /api/v1/finance/reconciliations` → 200. `X-User-Role: receptionist` dipakai menyelesaikan check-out.

**Fake gateway aktif.** Tanpa `XENDIT_SECRET_KEY`, server log `payment.gateway.fake.active`. `APP_ENV` default `development`, sehingga `POST /fake-pay/<booking_id>` tanpa autentikasi langsung mengonfirmasi booking tanpa pembayaran (200 `confirmed`). Server berjalan aman di production hanya jika `APP_ENV=production` diset secara eksplisit. Jika lupa, ini membuka konfirmasi gratis (BE-R16).

## Koreksi terhadap dokumen `docs/gap`
- **Dok 12 — "`go test -race ./...` gagal karena collision DB" tidak tereproduksi.** Tiga kali jalan dengan DB nyata lulus semua, tanpa skip. Namun `testing/integration/db_helper.go` memakai default DSN yang di-hardcode (`172.24.0.3/booking_test`) dan test mengubah baris inventory bersama: setelah suite, `room_type 01900000-…-0001` hanya punya 31 hari inventory dari 365. Test saling mengubah seed. Risiko isolasi nyata, tetapi tidak muncul sebagai kegagalan dalam 3 percobaan.
- **Dok 12 — instance `:18080` memberi 404 untuk route baru.** Terkonfirmasi (`/api/v1/auth/guest/me` → 404). Container `current-booking-app-1` berjalan dari image lama.
- **BE-R02** sudah diselesaikan kode (commit `1211fc8`), tetapi dokumen masih OPEN.

## Tidak diuji
Webhook Xendit, Resend, refund gateway nyata (tidak ada kredensial), OTP end-to-end, deadline pembatalan WIB vs UTC (hanya pembacaan kode), hold expiry nyata 30 menit, perilaku di balik reverse proxy.

## Kesimpulan
Alur tamu inti (search → quote → consent → hold → bayar → check-in/out) berfungsi pada source terbaru, dan 5 skenario concurrency DB lulus. Tiga defect dikonfirmasi lewat eksekusi nyata (bukan hanya pembacaan kode): **BE-R01 (P0)**, **BE-R06**, **BE-R08**. Backend belum layak diekspos publik atau dipakai untuk pembayaran live sebelum ketiganya ditutup.
