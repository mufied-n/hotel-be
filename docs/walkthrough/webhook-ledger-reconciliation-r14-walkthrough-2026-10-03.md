# Walkthrough — Webhook Ledger & Amount Reconciliation (BE-R14)

[PRD](../prd/webhook-ledger-reconciliation-r14-2026-10-03.md) · [SRS](../srs/webhook-ledger-reconciliation-r14-2026-10-03.md) · [Tech](../tech/webhook-ledger-reconciliation-architecture-2026-10-03.md) · [E2E Report](../../testing/e2e/report/2026-10-03-201100-webhook-reconciliation-r14-e2e-report.md)

Dokumen ini melacak eksekusi implementasi perbaikan **BE-R14 (P1)**: verifikasi nominal, mata uang, dan invoice ID pada callback webhook Xendit, serta proteksi stale expiry.

## Checklist Milestone

- [x] **Tahap 1: Riset Mendalam**
  - Analisis BE-R14 di `docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md`.
  - Inspeksi `internal/adapter/payment/xendit.go`, `internal/booking/service.go`, dan `internal/api/router.go`.
- [x] **Tahap 2: Dokumen Spesifikasi & Desain**
  - PRD: [webhook-ledger-reconciliation-r14-2026-10-03.md](../prd/webhook-ledger-reconciliation-r14-2026-10-03.md)
  - SRS: [webhook-ledger-reconciliation-r14-2026-10-03.md](../srs/webhook-ledger-reconciliation-r14-2026-10-03.md)
  - Tech Architecture: [webhook-ledger-reconciliation-architecture-2026-10-03.md](../tech/webhook-ledger-reconciliation-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
- [x] **Tahap 4: TDD & Refinement (Target Coverage $\ge$ 80%)**
  - Update `internal/adapter/payment/xendit.go` (field Currency).
  - Update `internal/booking/service.go` (`GetPaymentAttempts`).
  - Update `internal/api/router.go` (`xenditWebhook`).
  - Unit tests di `internal/api/webhook_test.go` dan `internal/adapter/payment/xendit_test.go` (12 passed di api, 19 passed di payment).
  - Coverage: `internal/api` 85.0%, `internal/adapter/payment` 87.5%.
  - Verifikasi: `rtk go vet ./...` bersih, `rtk go test -race ./...` 761 passed.
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - Skrip: `testing/e2e/script/webhook_reconciliation_r14_e2e.sh` (18/18 PASS).
  - Laporan E2E: `testing/e2e/report/2026-10-03-201100-webhook-reconciliation-r14-e2e-report.md`.
- [ ] **Tahap 6: Verifikasi Akhir & Git Commit**

---

## Log Eksekusi

- *2026-10-03 20:06*: PRD, SRS, Tech Architecture, Walkthrough disusun.
- *2026-10-03 20:08*: TDD implementasi field currency, method GetPaymentAttempts, validasi webhook, dan unit tests. Coverage api 85.0%, payment 87.5%.
- *2026-10-03 20:11*: E2E `webhook_reconciliation_r14_e2e.sh` 18/18 PASS. Laporan E2E selesai disusun.
