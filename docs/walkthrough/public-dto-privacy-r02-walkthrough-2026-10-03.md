# Walkthrough Tracking — Public DTO Privacy & Token Protection (BE-R02)

**Fitur:** Public DTO Privacy, Sensitive Free-Text & Token Protection  
**ID Gap:** BE-R02 (P1)  
**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed & Verified  

---

## 1. Checklist Rencana Kerja

- [x] **Tahap 1: Riset Mendalam**
  - Analisis kode eksisting di `internal/booking/booking.go` dan `internal/api/router.go`.
  - Pemetaan kepatuhan UU PDP No. 27/2022 dan OWASP ASVS V3 / API Security.
- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - PRD: [public-dto-privacy-r02-2026-10-03.md](../../docs/prd/public-dto-privacy-r02-2026-10-03.md)
  - SRS: [public-dto-privacy-r02-2026-10-03.md](../../docs/srs/public-dto-privacy-r02-2026-10-03.md)
  - Tech Architecture: [public-dto-privacy-architecture-2026-10-03.md](../../docs/tech/public-dto-privacy-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Dokumen: [public-dto-privacy-r02-walkthrough-2026-10-03.md](../../docs/walkthrough/public-dto-privacy-r02-walkthrough-2026-10-03.md)
- [x] **Tahap 4: Implementasi TDD (Target $\ge$ 80% Coverage)**
  - Hapus field `EstimatedArrivalTime` dan `SpecialRequests` dari struct `PublicDTO` di `internal/booking/booking.go`.
  - Sesuaikan `ToPublicDTO()` di `internal/booking/booking.go`.
  - Perbarui table tests di `internal/booking/booking_test.go`.
  - Pasang sanitasi `b.GuestToken = ""` pada `getBooking` di `internal/api/router.go`.
  - Tambah table-driven test komprehensif di `internal/api/router_test.go` (uji unauthenticated, wrong token, other booking token, valid guest token, dan staff token).
  - Verifikasi: `go test -v -cover ./internal/booking/... ./internal/api/...` (405 tests pass, booking.go 100%, router.go getBooking 88.9%) dan `go vet ./...` (clean).
- [x] **Tahap 5: End-to-End Testing & Verification**
  - Buat skrip E2E: [public_dto_privacy_r02_e2e.sh](../../testing/e2e/script/public_dto_privacy_r02_e2e.sh).
  - Jalankan pengujian terhadap test stack live (34/34 assertions PASS).
  - Dokumentasikan laporan: [2026-10-03-202200-public-dto-privacy-r02-e2e-report.md](../../testing/e2e/report/2026-10-03-202200-public-dto-privacy-r02-e2e-report.md).
- [x] **Tahap 6: Verification & Git Commit**
  - Update [07-backend-reaudit-2026-10-03.md](../../docs/gap/07-backend-reaudit-2026-10-03.md) dan [08-security-and-guest-session-reaudit-2026-10-03.md](../../docs/gap/08-security-and-guest-session-reaudit-2026-10-03.md).
  - Git commit BE-R02.
