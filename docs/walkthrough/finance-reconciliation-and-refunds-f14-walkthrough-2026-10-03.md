# Walkthrough — F14: Rekonsiliasi Finansial & Otomasi Gateway Refund (Xendit Refund API)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **PRD:** [PRD-F14-Finance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **SRS:** [SRS-F14-Finance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **Tech Architecture:** [TECH-F14-Finance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/finance-reconciliation-and-refunds-f14-architecture-2026-10-03.md)
- **E2E Report:** [E2E-Report-F14](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-finance-refunds-f14-e2e-report.md)

---

## Progress Tracking Matrix

| Tahap | Aktivitas | Status | Target Evidence |
|---|---|:---:|---|
| **Tahap 1** | Riset Pasar, Xendit Refund API, User Persona, dan Engineering Anti-Over-Refund | **DONE** | Dokumen PRD, SRS, Tech Architecture |
| **Tahap 2** | Analisis & Pembuatan Dokumen PRD, SRS, dan Desain Arsitektur Teknis | **DONE** | [`docs/prd/`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/finance-reconciliation-and-refunds-f14-2026-10-03.md), [`docs/srs/`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/finance-reconciliation-and-refunds-f14-2026-10-03.md), [`docs/tech/`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/finance-reconciliation-and-refunds-f14-architecture-2026-10-03.md) |
| **Tahap 3** | Walkthrough Tracking & Milestone Log | **DONE** | [`docs/walkthrough/`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/finance-reconciliation-and-refunds-f14-walkthrough-2026-10-03.md) |
| **Tahap 4** | Implementasi TDD (Table-Driven Tests $\ge 80\%$ Coverage) | **DONE** | `internal/finance/` (83.2% coverage), `go vet ./...` clean |
| **Tahap 5** | Automasi Pengujian E2E & Pembuatan Laporan | **DONE** | 45/45 Scenarios PASS, [`testing/e2e/report/`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-finance-refunds-f14-e2e-report.md) |
| **Tahap 6** | Verifikasi Akhir & Laporan ke User | **DONE** | Laporan transparan & meminta konfirmasi user sebelum git commit |

---

## Modifikasi & Penambahan File

1. **[`migrations/00009_finance_reconciliation_and_refunds.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00009_finance_reconciliation_and_refunds.sql)**:
   - DDL tabel `payment_refunds`, `payment_cases`, index pendukung, dan aturan Casbin rule untuk role `finance` dan `gm_admin`.
2. **[`internal/adapter/payment/xendit.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/adapter/payment/xendit.go)**:
   - Menambahkan DTO `RefundRequest`, `RefundResult`, dan method `CreateRefund(ctx context.Context, req RefundRequest) (RefundResult, error)` dengan idempotency key.
3. **`internal/finance/` (Package Domain Baru)**:
   - [`model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/model.go): Definisi struct `PaymentRefund`, `PaymentCase`, DTO `CreateRefundInput`, `ResolveCaseInput`, `RefundStatusView`, `ReconciliationSummary`.
   - [`store.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/store.go): Interface `Store` untuk akses basis data dan definisi error domain (`ErrBookingNotFound`, `ErrOverRefund`, `ErrCaseAlreadyResolved`, dsb.).
   - [`postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/postgres.go): Implementasi PostgreSQL pgxpool store dengan lock baris `FOR UPDATE` dan kalkulasi saldo refundable.
   - [`postgres_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/postgres_test.go): Pengujian siklus hidup PostgreSQL store terhadap database test.
   - [`service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/service.go): Implementasi use case bisnis `ProcessRefund`, `CreateLatePaymentCase`, `ListCases`, `ResolveCase`, `GetBookingRefundStatus`, `GetReconciliationSummary`.
   - [`service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance/service_test.go): Table-driven unit tests (mencapai 83.2% statement coverage).
4. **Transport Layer HTTP (`internal/api/` & `cmd/server/`)**:
   - [`internal/api/finance_handler.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/finance_handler.go): HTTP handler untuk seluruh endpoint finance dan guest refund status.
   - [`internal/api/finance_api_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/finance_api_test.go): Table-driven handler tests menguji otorisasi Casbin role `finance` vs `receptionist`, validasi payload, dan anti-IDOR.
   - [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go): Registrasi route finance, dependensi `FinanceSvc`, dan otomatisasi pencatatan kasus pada webhook late payment.
   - [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go): Inisialisasi `finance.NewPostgresStore` dan `finance.NewService` pada composition root server.
5. **End-to-End Testing (`testing/e2e/`)**:
   - [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go): Menambahkan implementasi `e2eFinanceStore` dan skenario E2E-41 s/d E2E-45.
   - [`testing/e2e/report/2026-10-03-finance-refunds-f14-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-finance-refunds-f14-e2e-report.md): Laporan resmi eksekusi E2E (45/45 PASS 100%).

---

## Log Eksekusi & Bukti Pengujian

```bash
# 1. goose up migration version 9
go run ./cmd/migrate up
# Output: OK 00009_finance_reconciliation_and_refunds.sql

# 2. Unit & Integration test package internal/finance
go test -v -cover ./internal/finance
# Output:
# === RUN   TestPostgresStore_RefundLifecycle
# --- PASS: TestPostgresStore_RefundLifecycle (0.01s)
# === RUN   TestPostgresStore_PaymentCasesLifecycle
# --- PASS: TestPostgresStore_PaymentCasesLifecycle (0.01s)
# === RUN   TestPostgresStore_SummaryAndOwnership
# --- PASS: TestPostgresStore_SummaryAndOwnership (0.01s)
# === RUN   TestFinanceService_ProcessRefund_TableTest
# --- PASS: TestFinanceService_ProcessRefund_TableTest (0.00s)
# === RUN   TestFinanceService_LatePaymentCase_TableTest
# --- PASS: TestFinanceService_LatePaymentCase_TableTest (0.00s)
# === RUN   TestFinanceService_GuestRefundStatus_TableTest
# --- PASS: TestFinanceService_GuestRefundStatus_TableTest (0.00s)
# === RUN   TestFinanceService_ReconciliationSummary_TableTest
# --- PASS: TestFinanceService_ReconciliationSummary_TableTest (0.00s)
# PASS
# coverage: 83.2% of statements

# 3. HTTP Transport Tests
go test -v ./internal/api -run TestFinanceAPI
# Output: PASS (all table tests passed)

# 4. End-to-End Suite
go test -v ./testing/e2e/script
# Output:
# === RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-41:_Finance_refund_processing_&_RBAC_authorization
# === RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-42:_Anti-over-refund_guard_strictly_rejects_excessive_amount_(409_Conflict)
# === RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-43:_Late_payment_case_listing_and_resolution_workflow_(200_OK)
# === RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-44:_Finance_reconciliation_summary_aggregation_(200_OK)
# === RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-45:_Guest_refund_status_inquiry_&_anti-IDOR_verification
# --- PASS: TestEndToEndHotelBookingRBACLifecycle (0.03s)
# PASS (45/45 E2E Scenarios PASS)

# 5. Full Repository Test Suite & Vet
go test ./...
# Output: ALL PASS
go vet ./...
# Output: Clean (0 errors / 0 warnings)
```
