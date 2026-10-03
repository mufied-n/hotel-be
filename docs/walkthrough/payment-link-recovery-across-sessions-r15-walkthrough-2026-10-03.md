# Walkthrough: Pemulihan Tautan Pembayaran Lintas Sesi (BE-R15)

**Nomor Dokumen:** WT-PULANG-BE-R15  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1 s/d Stage 6 Siap Review)  
**Author:** AI Engineering Agent  
**Terkait:** [`BE-R15`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md), BE-G11, BE-G12, F03, F05  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Analisis gap `BE-R15` pada audit snapshot: hilangnya payment URL saat tamu refresh atau berganti perangkat.
  - [x] Analisis struktur `payment_attempts.payload` dan integrasi dengan `AllowedActions.CanPay`.
  - [x] Perancangan recovery URL tanpa duplicate invoice creation.
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: [`docs/prd/payment-link-recovery-across-sessions-r15-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/payment-link-recovery-across-sessions-r15-2026-10-03.md)
  - [x] SRS: [`docs/srs/payment-link-recovery-across-sessions-r15-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/payment-link-recovery-across-sessions-r15-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/payment-link-recovery-across-sessions-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/payment-link-recovery-across-sessions-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Definisikan `PaymentRecovery` struct & `ErrPaymentRecoveryNotPending` di [`internal/booking/booking.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go).
  - [x] Implementasikan `GetPaymentRecovery` di [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go).
  - [x] Perbaiki scanning kolom `payload` JSONB pada `GetAttemptsByBookingID` di [`internal/booking/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go).
  - [x] Tambahkan field `PaymentURL` pada `BookingDetail` di [`internal/guest/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/model.go).
  - [x] Pasang resolusi `PaymentURL` dari `payment_attempts` pada `GetBookingDetailByEmail` di [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go) dan proteksi `AllowedActions.CanPay` di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go).
  - [x] Tambahkan endpoint `GET /api/v1/bookings/:id/payment` dan shortcut `GET /api/v1/guest/bookings/:id/payment` di [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go) dan [`internal/api/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_auth.go).
  - [x] Konfigurasi Casbin rule RBAC di [`internal/platform/auth/casbin_pgx.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth/casbin_pgx.go) dan migrasi DB di [`migrations/00018_payment_recovery_casbin.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00018_payment_recovery_casbin.sql).
  - [x] Table-driven unit tests di `internal/booking/service_test.go`, `internal/api/router_test.go`, dan `internal/api/guest_api_test.go`.
  - [x] Verifikasi `go test -v -cover ./...` (Coverage `internal/api` 83.6%) dan `go vet ./...` (Zero error).
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: [`testing/e2e/script/payment_link_recovery_r15_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/payment_link_recovery_r15_e2e.sh).
  - [x] Uji skenario multi-perangkat / cross-session recovery.
  - [x] Uji pencegahan duplikasi tagihan (attempt count tetap 1 saat pemulihan dipanggil berulang kali).
  - [x] Uji penolakan pemulihan saat hold kedaluwarsa (HTTP 410) dan saat booking confirmed (HTTP 409).
  - [x] Seluruh 24 asersi lolos (100% PASS).
  - [x] Dokumentasikan di [`testing/e2e/report/2026-10-03-234500-payment-link-recovery-r15-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-234500-payment-link-recovery-r15-e2e-report.md).
- [ ] **Tahap 6: Review & Final Report**
  - [x] Update manifest audit gap [`docs/gap/07-backend-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/07-backend-reaudit-2026-10-03.md) & [`docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md).
  - [ ] Minta konfirmasi user dan lakukan git commit.

---

## 2. Log Eksekusi & Catatan Modifikasi File

1. **`internal/booking/booking.go`**:
   - Menambahkan struct `PaymentRecovery` untuk response DTO endpoint pemulihan.
   - Menambahkan domain sentinel error `ErrPaymentRecoveryNotPending`.
2. **`internal/booking/service.go`**:
   - Mengimplementasikan `GetPaymentRecovery(ctx, bookingID)` yang memeriksa status pending, hold expiry (mengembalikan `ErrHoldExpired`), dan riwayat `PaymentAttempt`.
   - Mengambil URL invoice yang sudah ada dari `att.Payload["payment_url"]` guna mencegah pembentukan tagihan invoice ganda (*zero duplicate invoice generation*).
3. **`internal/booking/postgres.go`**:
   - Memperbaiki query `GetAttemptsByBookingID` untuk menyertakan kolom `payload` dan mendeserialisasi JSON bytes ke dalam `PaymentAttempt.Payload`.
4. **`internal/guest/model.go` & `internal/guest/postgres.go` & `internal/guest/service.go`**:
   - Menambahkan atribut `PaymentURL` pada `BookingDetail`.
   - Mengisi `PaymentURL` dari `payment_attempts` aktif berstatus `initiated` / `unknown_timeout` ketika reservasi berstatus `pending` dan masa hold belum berakhir.
   - Mengosongkan `PaymentURL` jika `AllowedActions.CanPay` bernilai false.
5. **`internal/api/router.go` & `internal/api/guest_auth.go`**:
   - Menambahkan handler `getBookingPayment` pada endpoint `GET /api/v1/bookings/:id/payment`.
   - Menambahkan handler `handleGuestBookingPayment` pada endpoint `GET /api/v1/guest/bookings/:id/payment`.
   - Memasang pertahanan IDOR bertingkat: verifikasi kepemilikan via `guest_token`, sesi tamu terautentikasi (`guest_email`), atau hak akses staf internal.
6. **`internal/platform/auth/casbin_pgx.go` & `migrations/00018_payment_recovery_casbin.sql`**:
   - Menambahkan policy rule Casbin untuk memperbolehkan role `guest` mengakses `GET /api/v1/bookings/:id/payment`.
7. **Pengujian**:
   - Unit tests: Table-driven test `TestService_GetPaymentRecovery_TableDriven` di `booking`, `TestGetBookingPayment_Handler` di `api`, dan `TestGuestAuth_BookingPayment_Recovery` di `api/guest_auth`.
   - E2E tests: [`testing/e2e/script/payment_link_recovery_r15_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/payment_link_recovery_r15_e2e.sh) dengan 24 asersi (100% PASS).
