# Walkthrough Tracker — Akses Tamu, Manajemen Sesi & Booking Saya (F02 & F03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **Completed & Verified (Ready for Git Commit)**
- **Feature Target:** Passwordless Guest Access (Email OTP via Resend Notifier), Cryptographic Session Management, IDOR-Safe My Bookings Read Model, and Allowed Actions.

---

## Standar Rekayasa & Regulasi yang Ditegakkan

1. **Perlindungan Data Pribadi (UU PDP No. 27/2022 & ISO/IEC 27001):**
   * Eliminasi kerentanan IDOR (Insecure Direct Object Reference) pada detail reservasi (BE-G13).
   * Query database menyaring `WHERE id = $1 AND guest_email = $2`, mengembalikan HTTP 404 generik jika diakses oleh pihak yang bukan pemilik.
2. **Keamanan Autentikasi & Sesi (OWASP ASVS V2 & V3):**
   * OTP 6-digit acak kriptografis (`crypto/rand`), TTL 10 menit, batas gagal 3 kali, one-time use.
   * Kode dan session token disimpan dalam bentuk SHA-256 hash (tidak ada plaintext at rest).
   * Verifikasi menggunakan komparasi waktu konstan (`subtle.ConstantTimeCompare`).
   * Anti-enumeration response pada endpoint permintaan kode tantangan.
3. **Anti-Overengineering (Prinsip Ponytail):**
   * Standard library Go murni tanpa library autentikasi eksternal yang membengkak.
   * Reuse Resend Notifier adapter untuk mendistribusikan email OTP.
   * Penyimpanan sesi langsung pada PostgreSQL 18 dengan indeks teroptimasi.

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset Standar & Regulasi (OWASP ASVS, UU PDP, ISO 27001, Hospitality)**
- [x] **Fase 2: Dokumen Analisis & Spesifikasi**
  - [x] PRD: [`docs/prd/guest-access-my-bookings-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/guest-access-my-bookings-2026-10-03.md)
  - [x] SRS: [`docs/srs/guest-access-my-bookings-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/guest-access-my-bookings-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/guest-access-my-bookings-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-access-my-bookings-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/guest-access-my-bookings-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/guest-access-my-bookings-walkthrough-2026-10-03.md)

- [x] **Fase 3: Migrasi Database & Domain Service (TDD)**
  - [x] Buat file migrasi `migrations/00008_guest_auth_and_sessions.sql`
  - [x] Jalankan migrasi di database lokal / test (goose version 8)
  - [x] Implementasikan domain `internal/guest` (Service, Store, Model)
  - [x] Tulis table-driven unit tests di `internal/guest/service_test.go` & `postgres_test.go` (Coverage 81.6%)
  - [x] Tambahkan email template OTP di `internal/adapter/notifier/resend.go` & `log.go` (Coverage 91.5%)

- [x] **Fase 4: HTTP Transport, Middleware & Router Integration (TDD)**
  - [x] Buat middleware autentikasi tamu di `internal/api/guest_auth.go`
  - [x] Daftarkan endpoints `/api/v1/auth/guest/*` dan `/api/v1/guest/bookings/*` di `internal/api/router.go`
  - [x] Tulis table-driven HTTP handler tests di `internal/api/guest_api_test.go` (Coverage 85.4%)
  - [x] Verifikasi `go test -v -cover ./...` (Seluruh test suite PASS)
  - [x] Verifikasi `go vet ./...` (0 errors)

- [x] **Fase 5: Pengujian End-to-End (E2E) & Laporan**
  - [x] Buat skrip automasi E2E `testing/e2e/script/guest_auth_my_bookings_e2e.sh`
  - [x] Integrasikan skenario E2E baru di `testing/e2e/script/e2e_runner_test.go` (E2E-29 s/d E2E-36)
  - [x] Eksekusi seluruh skenario E2E dan catat bukti eksekusi (36/36 PASS)
  - [x] Buat laporan formal di `testing/e2e/report/2026-10-03-guest-auth-my-bookings-e2e-report.md`

- [x] **Fase 6: Verifikasi Selektif & Git Commit Integrity**
  - [x] Cek status git staging (hanya file relevan dengan F02/F03 yang di-stage)
  - [x] Minta konfirmasi user sebelum git commit

