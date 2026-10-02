# Walkthrough Tracker — Batch BE-E: Keandalan Operasional, Konkurensi & Observabilitas
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **Completed (100% — Ready for Review & Final Commit)**
- **Feature Target:** Alokasi Kamar Bebas Konflik (`BE-G17`), Restitusi Early Check-Out & No-Show Cutoff (`BE-G22`), Observabilitas Sweep & Graceful Shutdown (`BE-G21`), serta Durabilitas Outbox Relay (`BE-G16`).

---

## Standar Rekayasa & Kepatuhan yang Ditegakkan

1. **Konkurensi & Integritas Transaksional (ACID & ISO/IEC 27001):**
   - CTE `FOR UPDATE OF r SKIP LOCKED` pada alokasi kamar fisik saat check-in.
   - Pengecekan `rows.Err()` di seluruh cursor streaming DB.
2. **Standar Industri Perhotelan (HTNG & PHRI Bintang 4):**
   - Restitusi inventaris kamar tersisa pada saat early check-out agar dapat dijual kembali (*resale*).
   - Penolakan status no-show sebelum tanggal check-in tiba (`ErrNoShowTooEarly`).
3. **Keandalan Infrastruktur (OWASP API8 & Ponytail):**
   - Validasi konfigurasi fail-fast pada runtime startup.
   - Koordinasi graceful shutdown untuk HTTP server, asynq worker, dan outbox relay.
   - Zero dependensi eksternal baru (murni idiom standard Go + PostgreSQL).

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset Standar Internasional, Nasional & Legal**
  - [x] Riset standar konkurensi PostgreSQL `SKIP LOCKED` untuk alokasi resource diskrit
  - [x] Riset standar operasional hotel bintang 4 untuk early departure & no-show cutoff
  - [x] Riset best practice graceful shutdown worker dan Go error checking

- [x] **Fase 2: Dokumen Analisis & Spesifikasi**
  - [x] PRD: [`docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md)
  - [x] SRS: [`docs/srs/operational-reliability-concurrency-batch-e-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/operational-reliability-concurrency-batch-e-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/operational-reliability-concurrency-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/operational-reliability-concurrency-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/operational-reliability-concurrency-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/operational-reliability-concurrency-walkthrough-2026-10-03.md)

- [x] **Fase 3: Implementasi TDD (Domain, Storage & Workers)**
  - [x] Update `PickAndAssignRooms` dengan `FOR UPDATE OF r SKIP LOCKED` dan error handling menyeluruh (`BE-G17`)
  - [x] Update use case `CheckOut` untuk restitusi inventaris sisa malam pada early check-out (`BE-G22`)
  - [x] Update use case `MarkNoShow` dengan batas waktu cutoff tanggal check-in (`BE-G22`)
  - [x] Update `SweepExpiredHolds` untuk validasi `rows.Err()` pasca iterasi (`BE-G21`)
  - [x] Update `OutboxRelay` dengan proteksi non-positive interval, dead-letter logging, dan sanitasi (`BE-G16`)
  - [x] Update `platform.Config` dengan validasi fail-fast dan graceful shutdown koordinatif (`BE-G21`)
  - [x] Table-driven tests dengan coverage $\ge 80\%$ pada paket inti

- [x] **Fase 4: End-to-End (E2E) Automation & Verification**
  - [x] Skenario E2E Parallel Check-In tanpa false allocation failure (`BE-G17`)
  - [x] Skenario E2E-23 Early Check-Out inventory restitution (sisa malam kembali ke inventaris) (`BE-G22`)
  - [x] Skenario E2E-24 No-Show rejection sebelum tanggal check-in (400 NO_SHOW_TOO_EARLY) (`BE-G22`)
  - [x] Skenario E2E-25 No-Show acceptance pada tanggal check-in merilis sisa malam (`BE-G22`)
  - [x] Laporan hasil E2E di: [`testing/e2e/report/2026-10-03-operational-reliability-batch-e-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-operational-reliability-batch-e-e2e-report.md)

- [x] **Fase 5: Verification & Commit Integrity**
  - [x] `go vet ./...` (0 errors)
  - [x] `go test -v ./...` (100% pass, 25/25 E2E test cases passed)
  - [x] Audit selektif git status dan perubahan file sebelum konfirmasi commit ke user

