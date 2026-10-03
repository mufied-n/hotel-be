# E2E Test Report: Atomic OTP Challenges & Attempt Protection (BE-R03)

**Fitur:** Atomic OTP Challenges & Attempt Protection  
**ID Gap:** BE-R03 (P1)  
**Terkait:** BE-R01, BE-R02, BE-R04, F02/F03  
**Tanggal Pengujian:** 3 Oktober 2026, 20:43:44 WIB  
**Hasil:** **18/18 PASS (100% SUKSES)**  
**Environment:** Linux / PostgreSQL 18 Alpine / Valkey 8 / Monolith HTTP Port 28080  

---

## 1. Ringkasan Eksekutif

Pengujian End-to-End ini memverifikasi mitigasi race condition pada modul autentikasi tamu (*Guest Auth OTP Challenge*) untuk hotel bintang 4 *Pulang ke Uttara* (Yogyakarta) sesuai standar OWASP ASVS V2/V3 dan UU PDP No. 27/2022:
1. **Advisory Lock Cooldown (Rate Limiting)**: Menggunakan PostgreSQL transaction advisory lock `pg_advisory_xact_lock(hashtext('guest_challenge:' || email))` untuk menyerialisasi permintaan challenge pada email yang sama secara atomik tanpa mengunci seluruh tabel. Ketika 5 request request-challenge dikirimkan secara bersamaan, tepat 1 request berhasil (200 OK) dan 4 lainnya ditolak dengan HTTP 429 Too Many Requests (`RATE_LIMIT_EXCEEDED`).
2. **Strict Brute-Force Attempt Counter & Lockout**: Verifikasi OTP yang salah dihitung langsung di database (`UPDATE guest_auth_challenges SET attempts = attempts + 1`). Setelah mencapai batas maksimal (3 percobaan), challenge dikunci secara permanen dan seluruh permintaan verifikasi selanjutnya ditolak dengan HTTP 403 Forbidden (`MAX_ATTEMPTS_EXCEEDED`), bahkan jika kode yang dimasukkan setelahnya adalah kode yang valid.
3. **Single-Use OTP Verification Under Concurrency**: Menggunakan transaksi atomik dengan row lock `SELECT ... FOR UPDATE` dan atomic conditional update `SET verified_at = now WHERE id = $1 AND verified_at IS NULL`. Ketika 20 request verifikasi dikirimkan serentak dengan kode OTP yang benar, tepat 1 request yang berhasil menjadi pemenang tunggal (HTTP 200 OK), sementara 19 request lainnya ditolak dengan HTTP 401 Unauthorized (`INVALID_OR_EXPIRED_CODE`). Di tingkat persistensi, database hanya membuat tepat 1 baris sesi di `guest_sessions` (tidak ada duplikasi sesi).
4. **Anti-Replay Attack**: Percobaan replay pada kode OTP yang sudah diverifikasi ditolak dengan HTTP 401 Unauthorized.
5. **Session Token Validation**: Token sesi terbitan pemenang berhasil digunakan untuk mengautentikasi request `GET /api/v1/auth/guest/me` dengan HTTP 200 OK dan memuat profil email tamu yang sah.

---

## 2. Rincian Eksekusi Test Cases

| No | Skenario Uji | Payload / Parameter | Expected Result | Actual Result | Status |
|---|---|---|---|---|---|
| 1 | Healthcheck endpoint | `GET /healthz` | HTTP 200 `{"status":"ok"}` | HTTP 200 `{"status":"ok"}` | **PASS** |
| 2 | Concurrent challenge cooldown (pemenang) | 5 concurrent POST `/challenge` | Tepat 1 berhasil (200 OK) | 1 berhasil (200 OK) | **PASS** |
| 3 | Concurrent challenge cooldown (penolakan) | 5 concurrent POST `/challenge` | 4 ditolak (429 Too Many Requests) | 4 ditolak (429 Too Many Requests) | **PASS** |
| 4 | Error code rate limit | Body response 429 | `RATE_LIMIT_EXCEEDED` | `RATE_LIMIT_EXCEEDED` | **PASS** |
| 5 | Percobaan salah ke-1 | Kode: `000001` | HTTP 401 Unauthorized | HTTP 401 Unauthorized | **PASS** |
| 6 | Error code percobaan salah ke-1 | Body response 401 | `INVALID_OR_EXPIRED_CODE` | `INVALID_OR_EXPIRED_CODE` | **PASS** |
| 7 | Percobaan salah ke-2 | Kode: `000002` | HTTP 401 Unauthorized | HTTP 401 Unauthorized | **PASS** |
| 8 | Percobaan salah ke-3 (kunci challenge) | Kode: `000003` | HTTP 403 Forbidden | HTTP 403 Forbidden | **PASS** |
| 9 | Error code percobaan ke-3 | Body response 403 | `MAX_ATTEMPTS_EXCEEDED` | `MAX_ATTEMPTS_EXCEEDED` | **PASS** |
| 10 | Percobaan ke-4 dengan kode benar pasca-kunci | Kode: `123456` (sah) | HTTP 403 Forbidden | HTTP 403 Forbidden | **PASS** |
| 11 | Error code pasca-kunci | Body response 403 | `MAX_ATTEMPTS_EXCEEDED` | `MAX_ATTEMPTS_EXCEEDED` | **PASS** |
| 12 | Single-use OTP race (20 concurrent verify) | 20 concurrent POST `/verify` | Tepat 1 menang (200 OK) | 1 menang (200 OK) | **PASS** |
| 13 | Single-use OTP race (19 penolakan serentak) | 20 concurrent POST `/verify` | 19 ditolak (401 Unauthorized) | 19 ditolak (401 Unauthorized) | **PASS** |
| 14 | Invariant database guest_sessions | `SELECT count(*) FROM guest_sessions` | Tepat 1 sesi dibuat di DB | Tepat 1 sesi dibuat di DB | **PASS** |
| 15 | Anti-Replay attack pada OTP terpakai | POST `/verify` dengan kode pemenang | HTTP 401 Unauthorized | HTTP 401 Unauthorized | **PASS** |
| 16 | Akses profil sesi terverifikasi | `GET /api/v1/auth/guest/me` | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 17 | Verifikasi integritas profil email | Body response `/me` | Memuat email tamu yang sah | Cocok | **PASS** |
| 18 | Validasi format kode OTP tidak valid | Kode: `12` (< 6 digit) | HTTP 401 Unauthorized | HTTP 401 Unauthorized | **PASS** |

---

## 3. Kesimpulan Verifikasi

Temuan **BE-R03 (P1)** telah terselesaikan dan terverifikasi secara tuntas:
- **Unit & Concurrency Tests**: Mencakup 42 test suite di package `internal/guest` dengan code coverage **89.6%** (melebihi target batas minimal 80%).
- **E2E Automation**: Skrip E2E (`testing/e2e/script/atomic_otp_challenges_r03_e2e.sh`) membuktikan ketahanan sistem di bawah beban konkurensi nyata, eliminasi race window pada verifikasi OTP, kepatuhan batas percobaan brute-force, dan isolasi mutlak token sesi.
