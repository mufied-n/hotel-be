# Walkthrough — Production Provider Fail-Fast & Readiness Safety (BE-R16)

[PRD](../prd/production-provider-failfast-r16-2026-10-03.md) · [SRS](../srs/production-provider-failfast-r16-2026-10-03.md) · [Tech](../tech/production-provider-failfast-architecture-2026-10-03.md) · [E2E Report](../../testing/e2e/report/2026-10-03-200300-production-provider-failfast-e2e-report.md)

Dokumen ini melacak eksekusi implementasi perbaikan **BE-R16 (P0)**: penegakan fail-fast startup di production, pemisahan kapabilitas readiness probe, redaksi OTP log, dan hardening handler fake payment dev.

## Checklist Milestone

- [x] **Tahap 1: Riset Mendalam**
  - Analisis audit BE-R16 di `docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md`
  - Analisis alur booting `cmd/server/main.go`, `internal/platform/config.go`, `internal/adapter/notifier/log.go`, dan `internal/api/router.go`.
- [x] **Tahap 2: Dokumen Spesifikasi & Desain**
  - PRD: [production-provider-failfast-r16-2026-10-03.md](../prd/production-provider-failfast-r16-2026-10-03.md)
  - SRS: [production-provider-failfast-r16-2026-10-03.md](../srs/production-provider-failfast-r16-2026-10-03.md)
  - Tech Architecture: [production-provider-failfast-architecture-2026-10-03.md](../tech/production-provider-failfast-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Pelacakan perubahan file dan log eksekusi.
- [x] **Tahap 4: TDD & Refinement (Target Coverage $\ge$ 80%)**
  - Update unit test `internal/platform/config_test.go` (table test validasi production).
  - Update `internal/adapter/notifier/log_test.go` (redaksi OTP).
  - Update `internal/api/router_test.go` (pengujian capability readiness payload).
  - Implementasi perubahan di `internal/platform/config.go`, `internal/adapter/notifier/log.go`, `internal/api/router.go`, dan `cmd/server/main.go`.
  - Verifikasi: `go vet ./...` bersih; cakupan statement: `internal/platform` 86.0%, `internal/adapter/notifier` 92.2%, `internal/api` 85.0%.
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - Skrip pengujian: `testing/e2e/script/production_provider_failfast_e2e.sh`.
  - Uji skenario:
    1. Startup gagal jika `APP_ENV=production` tanpa Xendit & Resend keys (19/19 lulus).
    2. Startup sukses dengan keys.
    3. `/ready` mengembalikan `environment`, `payment_gateway`, `notifier`.
    4. `/fake-pay` menolak non-UUID dengan 400 Bad Request JSON tanpa SQL leak.
    5. `/fake-pay` menghasilkan 404 pada mode production.
    6. Regresi 9 skrip suite lainnya lulus 100%.
  - Dokumentasi laporan E2E: `testing/e2e/report/2026-10-03-200300-production-provider-failfast-e2e-report.md`.
- [ ] **Tahap 6: Verifikasi Akhir & Laporan ke User**

---

## Log Eksekusi

- *2026-10-03 19:57*: Dokumen PRD, SRS, Tech Architecture, dan Walkthrough disusun.
- *2026-10-03 20:00*: TDD `internal/platform` (86.0%), `internal/adapter/notifier` (92.2%), `internal/api` (85.0%), zero lint/vet issues.
- *2026-10-03 20:02*: E2E `production_provider_failfast_e2e.sh` 19/19 PASS, 9 skrip regresi PASS 100%.
- *2026-10-03 20:03*: Laporan E2E selesai disusun.
