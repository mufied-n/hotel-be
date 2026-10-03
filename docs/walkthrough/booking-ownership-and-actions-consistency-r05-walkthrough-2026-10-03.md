# Walkthrough Tracking — Booking Ownership & Allowed Actions Consistency (BE-R05)

**Fitur:** Canonical Email Normalization & Policy-Driven Allowed Actions  
**Gap ID:** BE-R05 (P1)  
**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed (Stage 1-5 Done, Ready for Git Commit)  

---

## 1. Checklist Rencana Kerja

- [x] **Tahap 1: Riset Mendalam**
  - Analisis kode di [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go), [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go), dan [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go).
  - Identifikasi hilangnya booking akibat perbedaan huruf besar/kecil email pada query.
  - Identifikasi false affordance pembatalan pada `computeAllowedActions`.
- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - PRD: [`docs/prd/booking-ownership-and-actions-consistency-r05-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/booking-ownership-and-actions-consistency-r05-2026-10-03.md)
  - SRS: [`docs/srs/booking-ownership-and-actions-consistency-r05-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/booking-ownership-and-actions-consistency-r05-2026-10-03.md)
  - Tech Architecture: [`docs/tech/booking-ownership-and-actions-consistency-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/booking-ownership-and-actions-consistency-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Dokumen: [`docs/walkthrough/booking-ownership-and-actions-consistency-r05-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/booking-ownership-and-actions-consistency-r05-walkthrough-2026-10-03.md)
- [x] **Tahap 4: Implementasi TDD (Target $\ge$ 80% Coverage)**
  - [x] Normalisasi email di `Create` pada [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go).
  - [x] Normalisasi query `LOWER(TRIM(b.guest_email)) = $1` pada [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go).
  - [x] Tambahkan field `CancellationPolicy`, `RatePlanCode`, `ExpiresAt` ke `BookingDetail` di [`internal/guest/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/model.go) dan scan di `GetBookingDetailByEmail`.
  - [x] Perbarui `computeAllowedActions` di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go) dengan integrasi `booking.FreeCancellationDeadline`.
  - [x] Tulis table tests di [`internal/booking/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service_test.go) dan [`internal/guest/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service_test.go).
  - [x] Verifikasi: `go test -v -cover ./...` (**90.1% coverage** pada `internal/guest`) dan `go vet ./...` (0 issue).
- [x] **Tahap 5: End-to-End Testing & Verification**
  - [x] Buat skrip E2E: [`testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh).
  - [x] Uji skenario:
    1. Booking dibuat dengan `Guest.Owner@EXAMPLE.com`, login OTP dengan `guest.owner@example.com` $\rightarrow$ booking ditemukan.
    2. Confirmed booking non-refundable $\rightarrow$ `can_cancel: false`.
    3. Confirmed booking flexible sebelum batas waktu 14:00 WIB $\rightarrow$ `can_cancel: true`.
    4. Confirmed booking flexible setelah batas waktu 14:00 WIB $\rightarrow$ `can_cancel: false`.
    5. Pending booking kedaluwarsa $\rightarrow$ `can_pay: false`, `can_cancel: false`.
  - [x] Laporan E2E: [`testing/e2e/report/2026-10-03-210800-booking-ownership-actions-consistency-r05-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-210800-booking-ownership-actions-consistency-r05-e2e-report.md) (**19/19 PASS**).
- [ ] **Tahap 6: Verification & Git Commit**
  - Update audit gap backend: `docs/gap/07-backend-reaudit-2026-10-03.md` dan `docs/gap/08-security-and-guest-session-reaudit-2026-10-03.md`.
  - Git commit BE-R05.

---

## 2. Log Modifikasi Berkas

| Berkas | Aksi | Deskripsi |
|---|---|---|
| [`internal/guest/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/model.go) | Edit | Menambahkan field `CancellationPolicy`, `RatePlanCode`, dan `ExpiresAt` pada `BookingDetail`. |
| [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go) | Edit | Menggunakan `LOWER(TRIM(b.guest_email)) = $1` pada seluruh query pencarian tamu dan scan field kebijakan/hold. |
| [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go) | Edit | Mengintegrasikan `booking.FreeCancellationDeadline` pada `computeAllowedActions` serta menguji masa berlaku hold. |
| [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go) | Edit | Menormalisasi input `GuestEmail` ke lowercase trimmed dan `GuestName` ke trimmed format pada `Create`. |
| [`internal/guest/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service_test.go) | Edit | Table-driven tests lengkap untuk `computeAllowedActions` dan case-insensitive email ownership. |
| [`internal/guest/postgres_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres_test.go) | Edit | Integration test dengan PostgreSQL nyata memverifikasi kueri case-insensitive dan parsing policy. |
| [`internal/booking/service_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service_test.go) | Edit | Unit test untuk normalisasi email kanonikal pada checkout. |
| [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) | Edit | Penyesuaian `e2eGuestStore` dan payload webhook `E2E-27` agar konsisten dengan `TotalPriceMinor`. |
| [`testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh) | Baru | Skrip otomasi pengujian E2E 19 asersi. |
| [`testing/e2e/report/2026-10-03-210800-booking-ownership-actions-consistency-r05-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-210800-booking-ownership-actions-consistency-r05-e2e-report.md) | Baru | Laporan hasil eksekusi pengujian E2E BE-R05. |
