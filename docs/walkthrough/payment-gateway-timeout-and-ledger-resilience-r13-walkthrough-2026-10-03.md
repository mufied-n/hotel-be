# Walkthrough: Ketahanan Timeout Gateway Pembayaran & Integritas Buku Besar (BE-R13)

**Nomor Dokumen:** WT-PULANG-BE-R13  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1 s/d Stage 6 Selesai)  
**Author:** AI Engineering Agent  
**Terkait:** [`BE-R13`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/07-backend-reaudit-2026-10-03.md), BE-G11, BE-G12, F05, F14  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Analisis cacat source di `booking/service.go:344-361` (ambiguitas error gateway, auto-cancel pada timeout, swallowed ledger error).
  - [x] Analisis skema tabel `payment_attempts` dan batasan `UpdateAttemptStatus` berbasis `booking_id`.
  - [x] Perancangan klasifikasi error timeout vs definitive failure.
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: [`docs/prd/payment-gateway-timeout-and-ledger-resilience-r13-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/payment-gateway-timeout-and-ledger-resilience-r13-2026-10-03.md)
  - [x] SRS: [`docs/srs/payment-gateway-timeout-and-ledger-resilience-r13-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/payment-gateway-timeout-and-ledger-resilience-r13-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/payment-gateway-timeout-and-ledger-resilience-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/payment-gateway-timeout-and-ledger-resilience-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Perluas [`internal/booking/payment_attempt.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/payment_attempt.go):
    - Tambahkan metode `UpdateAttemptByID` pada interface `PaymentAttemptStore`.
  - [x] Implementasikan `UpdateAttemptByID` di [`internal/booking/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go) dengan query parameter `id = $5`.
  - [x] Definisikan error dan helper di [`internal/booking/booking.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go):
    - `ErrPaymentGatewayTimeout`, `ErrPaymentDefinitiveFailure`
    - Helper `IsGatewayTimeout(err error) bool`
  - [x] Modifikasi alur `Create` di [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go):
    - Catat attempt intent pra-call (`status: "initiated"`, UUID v7).
    - Tangani timeout: status `"unknown_timeout"`, jangan batalkan booking, pertahankan kamar di-hold.
    - Tangani definitive failure: status `"failed"`, batalkan booking (kompensasi `s.Cancel`), log error jika kompensasi gagal.
    - Update attempt spesifik dengan reference gateway saat sukses.
    - Jangan abaikan error `RecordAttempt` atau `UpdateAttemptStatus` (catat via `s.log.ErrorContext`).
    - Guard logger default `slog.Default()` jika `log == nil`.
  - [x] Modifikasi [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go):
    - Petakan `ErrPaymentGatewayTimeout` ke HTTP 504 `GATEWAY_TIMEOUT`.
    - Petakan `ErrPaymentDefinitiveFailure` ke HTTP 502 `PAYMENT_FAILED`.
  - [x] Table-driven unit tests di [`internal/booking/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service_test.go) dan [`internal/api/router_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router_test.go).
  - [x] Verifikasi `go test -v -cover ./...` dan `go vet ./...` (100% PASS, 0 linter issues).
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: [`testing/e2e/script/payment_gateway_timeout_r13_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/payment_gateway_timeout_r13_e2e.sh).
  - [x] Uji gateway timeout: verifikasi status booking tetap `pending`, ketersediaan kamar tidak bertambah/dirilis, dan attempt status `unknown_timeout`.
  - [x] Uji webhook konfirmasi pada booking yang sempat timeout: verifikasi transisi menjadi `confirmed`.
  - [x] Uji definitive failure: verifikasi status booking `cancelled`, ketersediaan kamar dikembalikan ke publik, dan attempt status `failed`.
  - [x] Catat hasil di [`testing/e2e/report/2026-10-03-233500-payment-gateway-timeout-r13-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-233500-payment-gateway-timeout-r13-e2e-report.md) (21 asersi, 100% PASS).
- [x] **Tahap 6: Review & Final Report**
  - [x] Update audit gap manifest `docs/gap/07-backend-reaudit-2026-10-03.md` dan `docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md`.
  - [x] Konfirmasi ke user sebelum git commit.

---

## 2. Log Eksekusi & Catatan Modifikasi File

| File | Status | Keterangan Perubahan |
| :--- | :--- | :--- |
| [`internal/booking/payment_attempt.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/payment_attempt.go) | Modified | Menambahkan signature `UpdateAttemptByID(ctx, id, status, ref, payload) error` pada `PaymentAttemptStore`. |
| [`internal/booking/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go) | Modified | `RecordAttempt` menyimpan `attempt.ID` secara eksplisit; mengimplementasikan `UpdateAttemptByID` dengan filter `WHERE id = $5`. |
| [`internal/booking/booking.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go) | Modified | Menambahkan sentinel `ErrPaymentGatewayTimeout`, `ErrPaymentDefinitiveFailure`, serta helper `IsGatewayTimeout(err error) bool`. |
| [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go) | Modified | Pre-call payment attempt intent insertion; isolasi gateway timeout tanpa pembatalan hold; kompensasi pembatalan saat definitive failure; pencatatan error ledger via slog; guard nil logger di `NewService`. |
| [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go) | Modified | Pemetaan `ErrPaymentGatewayTimeout` $\rightarrow$ 504 `GATEWAY_TIMEOUT` dan `ErrPaymentDefinitiveFailure` $\rightarrow$ 502 `PAYMENT_FAILED`. |
| [`internal/booking/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service_test.go) | Modified | Menambahkan table-driven test `TestService_PaymentTimeoutAndDefinitiveFailure_TableDriven` dan `TestIsGatewayTimeout_TableDriven`. |
| [`internal/api/router_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router_test.go) | Modified | Menambahkan table-driven test `TestCreateBooking_PaymentGatewayErrors_TableDriven`. |
| [`testing/e2e/script/payment_gateway_timeout_r13_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/payment_gateway_timeout_r13_e2e.sh) | Created | Skrip pengujian E2E otomatis untuk 4 skenario timeout, webhook recovery, failure, dan ledger check. |
| [`testing/e2e/report/2026-10-03-233500-payment-gateway-timeout-r13-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-233500-payment-gateway-timeout-r13-e2e-report.md) | Created | Laporan komprehensif eksekusi E2E dengan 21 asersi lolos (100% pass). |
