# Walkthrough Tracking — Penyeragaman Batas Payload & Skema Error API (BE-R18)

**Nomor Dokumen:** WLK-PULANG-BE-R18-2026-10-04  
**Tanggal Mulai:** 4 Oktober 2026  
**Status:** In Progress (Stage 4 TDD Implementation)  
**Author:** AI Engineering Agent  
**Gap Terkait:** [`BE-R18`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md)  

---

## 1. Status Milestone

- [x] **Stage 1: Riset Mendalam**
  - Analisis OWASP API4:2023 Unrestricted Resource Consumption.
  - Investigasi pembacaan body pada `createBooking`, `xenditWebhook`, dan modul portal tamu.
  - Investigasi inkonsistensi error field (`error`, `code`, `detail`, `message`).
- [x] **Stage 2: Spesifikasi Teknis & Analisis**
  - PRD: [`docs/prd/payload-boundary-and-unified-error-r18-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/payload-boundary-and-unified-error-r18-2026-10-04.md)
  - SRS: [`docs/srs/payload-boundary-and-unified-error-r18-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/payload-boundary-and-unified-error-r18-2026-10-04.md)
  - Tech Architecture: [`docs/tech/payload-boundary-and-unified-error-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/payload-boundary-and-unified-error-architecture-2026-10-04.md)
- [x] **Stage 3: Walkthrough Tracking**
  - Dokumen pelacak ini dibuat.
- [x] **Stage 4: Implementasi TDD (Target $\ge 80\%$ Coverage)**
  - [x] Tambahkan `BodySizeLimit` middleware dan `isMaxBytesError` helper di `internal/api/middleware.go`.
  - [x] Harmonisasikan `ProblemDetails` dengan field `Message: detail`.
  - [x] Implementasikan `writeGuestError` di `internal/api/guest_auth.go` dan perbarui seluruh panggilan error.
  - [x] Tambahkan penegakan batas 64 karakter pada header `Idempotency-Key` di `createBooking`.
  - [x] Tambahkan penanganan `MaxBytesError` pada `createBooking` dan `xenditWebhook`.
  - [x] Tulis table-driven unit tests di `internal/api/router_test.go` (`TestPayloadBoundaryAndUnifiedError_TableDriven`).
  - [x] Verifikasi `go test -v -cover ./...` (Coverage `internal/api` 83.8%) dan `go vet ./...` (0 errors).
- [x] **Stage 5: End-to-End (E2E) Testing**
  - [x] Buat skrip `testing/e2e/script/payload_boundary_and_unified_error_r18_e2e.sh`.
  - [x] Eksekusi skrip terhadap live environment (Docker DB + Valkey + Server). 29/29 assertions PASSED (100%).
  - [x] Buat laporan `testing/e2e/report/2026-10-04-003000-payload-boundary-and-unified-error-r18-e2e-report.md`.
- [x] **Stage 6: Review & Final Closure**
  - [x] Perbarui `docs/gap/07-backend-reaudit-2026-10-03.md` dan `docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md` menjadi RESOLVED.
  - [ ] Lapor ke User & minta konfirmasi git commit.

---

## 2. File Modifikasi Rencana

| File | Tindakan | Rincian Perubahan |
| :--- | :--- | :--- |
| `internal/api/middleware.go` | Edit | Tambah `BodySizeLimit`, `isMaxBytesError`, perbarui `ProblemDetails`. |
| `internal/api/guest_auth.go` | Edit | Tambah `writeGuestError` dual-shape, tangani `isMaxBytesError`. |
| `internal/api/router.go` | Edit | Pasang `BodySizeLimit(1 << 20)`, validasi `Idempotency-Key` len 1..64, tangani `isMaxBytesError`. |
| `internal/api/router_test.go` | Edit | Table-driven tests: payload > 1 MB (413), idempotency key > 64 (400), malformed JSON (400). |
| `testing/e2e/script/payload_boundary_and_unified_error_r18_e2e.sh` | Baru | Skrip otomasi E2E pengujian boundary payload & error uniformity. |
| `testing/e2e/report/...` | Baru | Laporan verifikasi runtime. |
