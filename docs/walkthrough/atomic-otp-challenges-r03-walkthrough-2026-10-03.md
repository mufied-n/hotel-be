# Walkthrough Tracking — Atomic OTP Challenge & Attempt Limiting (BE-R03)

**Fitur:** Atomic OTP Challenge, Attempt Limiting & Single-Use Verification  
**ID Gap:** BE-R03 (P1)  
**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed (Ready for Commit)  

---

## 1. Checklist Rencana Kerja

- [x] **Tahap 1: Riset Mendalam**
  - Analisis kode di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go) dan [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go).
  - Pemetaan skenario interleaving race condition: parallel guess brute force, parallel OTP replay, dan cooldown bypass.
- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - PRD: [`docs/prd/atomic-otp-challenges-r03-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/atomic-otp-challenges-r03-2026-10-03.md)
  - SRS: [`docs/srs/atomic-otp-challenges-r03-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/atomic-otp-challenges-r03-2026-10-03.md)
  - Tech Architecture: [`docs/tech/atomic-otp-challenges-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/atomic-otp-challenges-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Dokumen: [`docs/walkthrough/atomic-otp-challenges-r03-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/atomic-otp-challenges-r03-walkthrough-2026-10-03.md)
- [x] **Tahap 4: Implementasi TDD (Target $\ge$ 80% Coverage)**
  - Perluas `Store` interface di [`internal/guest/store.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/store.go) dengan [`CreateChallengeWithCooldown`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/store.go#L28) dan [`VerifyAndConsumeChallenge`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/store.go#L29).
  - Implementasikan [`CreateChallengeWithCooldown`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go#L83) dan [`VerifyAndConsumeChallenge`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go#L132) di [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go) dengan row lock `FOR UPDATE`, direct atomic increment, dan `pg_advisory_xact_lock`.
  - Perbarui [`RequestChallenge`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go#L58) dan [`VerifyChallenge`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go#L101) di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go).
  - Perbarui mock store dan unit test di [`internal/guest/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service_test.go).
  - Tulis real-DB concurrent test di [`internal/guest/postgres_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres_test.go).
  - Hasil coverage: **89.6%** (42 tests pass). Lulus `go vet ./...` (0 issues).
- [x] **Tahap 5: End-to-End Testing & Verification**
  - Skrip E2E: [`testing/e2e/script/atomic_otp_challenges_r03_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/atomic_otp_challenges_r03_e2e.sh).
  - Hasil pengujian: **18/18 PASS (100% SUKSES)**.
  - Laporan E2E: [`testing/e2e/report/2026-10-03-204500-atomic-otp-challenges-r03-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-204500-atomic-otp-challenges-r03-e2e-report.md).
- [x] **Tahap 6: Verification & Git Commit**
  - Update dokumen audit gap backend.
  - Siap di-commit ke master.
