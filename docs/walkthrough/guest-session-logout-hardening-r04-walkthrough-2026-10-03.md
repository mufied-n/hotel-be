# Walkthrough Tracking — Guest Session Logout Hardening & Cookie Security (BE-R04)

**Fitur:** Guest Session Logout Hardening, Secure Cookies, and Private Cache Headers  
**Gap ID:** BE-R04 (P1)  
**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed (Ready for Commit)  

---

## 1. Checklist Rencana Kerja

- [x] **Tahap 1: Riset Mendalam**
  - Analisis kode di [`internal/api/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_auth.go) dan [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go).
  - Identifikasi blind logout di mana error pencabutan sesi diabaikan.
  - Identifikasi ketiadaan flag `Secure` pada cookie dan ketiadaan header `Cache-Control: no-store, private`.
- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - PRD: [`docs/prd/guest-session-logout-hardening-r04-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/guest-session-logout-hardening-r04-2026-10-03.md)
  - SRS: [`docs/srs/guest-session-logout-hardening-r04-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/guest-session-logout-hardening-r04-2026-10-03.md)
  - Tech Architecture: [`docs/tech/guest-session-logout-hardening-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-session-logout-hardening-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Dokumen: [`docs/walkthrough/guest-session-logout-hardening-r04-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/guest-session-logout-hardening-r04-walkthrough-2026-10-03.md)
- [x] **Tahap 4: Implementasi TDD (Target $\ge$ 80% Coverage)**
  - Update `handleGuestLogout` di [`internal/api/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_auth.go#L234) untuk menangani error `RevokeSession` (503 `LOGOUT_FAILED`) dan menambahkan `Cache-Control: no-store, private`.
  - Update `handleGuestVerify` dan `handleGuestLogout` untuk menerapkan flag `Secure` dinamis (`isSecureCookie`).
  - Tambahkan header `Cache-Control: no-store, private` dan `Pragma: no-cache` pada `handleGuestVerify` dan `handleGuestMe`.
  - Update `ValidateSession` di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go#L158) agar log error jika `TouchSession` gagal dan tidak memperpanjang waktu kedaluwarsa sesi secara in-memory.
  - Tulis unit & table tests di [`internal/api/guest_api_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_api_test.go) dan [`internal/guest/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service_test.go).
  - Hasil coverage: `internal/api` (**85.2%**), `internal/guest` (**89.5%**).
  - Lulus `go vet ./...` (0 issue).
- [x] **Tahap 5: End-to-End Testing & Verification**
  - Skrip E2E: [`testing/e2e/script/guest_session_logout_hardening_r04_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/guest_session_logout_hardening_r04_e2e.sh).
  - Hasil pengujian: **19/19 PASS (100% SUKSES)**.
  - Laporan E2E: [`testing/e2e/report/2026-10-03-205100-guest-session-logout-hardening-r04-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-205100-guest-session-logout-hardening-r04-e2e-report.md).
- [x] **Tahap 6: Verification & Git Commit**
  - Update dokumen audit gap: [`docs/gap/07-backend-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/07-backend-reaudit-2026-10-03.md) dan [`docs/gap/08-security-and-guest-session-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/08-security-and-guest-session-reaudit-2026-10-03.md).
  - Git commit BE-R04.
