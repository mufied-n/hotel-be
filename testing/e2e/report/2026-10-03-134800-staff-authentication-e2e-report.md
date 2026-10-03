# E2E Report — Staff Authentication (BE-R01)

Tanggal: 2026-10-03 · Lingkungan: stack terisolasi (Postgres 18 `booking_test` :25432, Valkey :26379, server :28080), migrasi 1–17.

## Hasil

| Skrip / suite | Hasil |
|---|---|
| `staff_auth_e2e.sh` | PASS=20 FAIL=0 |
| `checkout_integrity_e2e.sh` (regresi) | PASS=15 FAIL=0 |
| `guest_special_requests_e2e.sh` (regresi) | semua lulus |
| `front_desk_daily_roster_e2e.sh` | berjalan, aktor tercatat `staff:fo_receptionist` |
| `go vet ./...` | bersih |
| `go test -race -cover ./...` | semua lulus; `internal/staffauth` 88.0%, `internal/api` 85.0% |

## Sebelum vs sesudah (kredensial palsu)

| Request | Sebelum | Sesudah |
|---|---|---|
| `Authorization: Bearer gm_admin` | 200 (akses GM) | diperlakukan tamu → 403 |
| `X-User-Role: gm_admin` | 200 | diabaikan → 403 |
| `X-Internal-Secret` / `X-Testing-Role` | diterima | diabaikan |
| `Bearer stf_<tidak valid>` | — | 401 `AUTHENTICATION_REQUIRED` |
| Login benar → token `stf_…` | — | 200, sesi 8 jam; logout mencabut sesi seketika |
| 5× password salah | — | akun terkunci 15 menit |

## Skrip legacy (diperbarui, semua lulus)

Rerun awal menunjukkan kegagalan di skrip lama. Penyebabnya bukan auth, melainkan asumsi lama skrip:

| Skrip | Penyebab | Perbaikan | Hasil |
|---|---|---|---|
| `hotel_booking_rbac_e2e.sh` | booking tanpa `quote_id` → 400 `QUOTE_REQUIRED` (BE-R06), lalu langkah turunan 404; key idempotency tetap antar-run; limiter 20 rps | helper `mkquote` + consent per booking, `RUN_ID` pada key, `sleep 3` | 40/40, diulang 3× |
| `housekeeping_room_readiness_e2e.sh` | mengasumsikan kamar 202/204 sudah kotor (default DB `inspected`) | fixture `set_room_status` (`lib_fixtures.sh`) | lulus |
| `stay_modification_room_move_e2e.sh` | `BOOKING_ID` default `bk-e2e-001` tidak ada | fixture `create_checked_in_booking` + reset room 202/204 | lulus, diulang 3× |
| `front_desk_daily_roster`, `feature_flags`, `transport_migration`, `guest_special_requests`, `checkout_integrity`, `staff_auth` | — | — | lulus |

Fixture memakai `psql` + `DATABASE_URL` (wajib DB `*_test`).

## Risiko residual

- Tanpa MFA, refresh token, atau audit event login.
- BE-R16: gateway pembayaran palsu masih jadi fallback; `/fake-pay` tanpa auth. Wajib `APP_ENV=production` + Xendit.
- Password staf seed kosong sampai operator menjalankan `staffadmin set-password <username>`.
- Wajib TLS di depan server (token bearer).
