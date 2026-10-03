# Walkthrough Tracking — Cancellation Deadline Timezone Alignment (BE-R12)

**Fitur:** Cancellation Deadline WIB Timezone Alignment  
**ID Gap:** BE-R12 (P1)  
**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed & Verified  

---

## 1. Checklist Rencana Kerja

- [x] **Tahap 1: Riset Mendalam**
  - Analisis kode eksisting di `internal/booking/service.go` baris 368-375 dan `internal/rates/engine.go` baris 235.
  - Perhitungan konversi zona waktu: 14:00 WIB (Asia/Jakarta, UTC+7) = 07:00 UTC.
- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - PRD: [cancellation-timezone-r12-2026-10-03.md](../../docs/prd/cancellation-timezone-r12-2026-10-03.md)
  - SRS: [cancellation-timezone-r12-2026-10-03.md](../../docs/srs/cancellation-timezone-r12-2026-10-03.md)
  - Tech Architecture: [cancellation-timezone-architecture-2026-10-03.md](../../docs/tech/cancellation-timezone-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - Dokumen: [cancellation-timezone-r12-walkthrough-2026-10-03.md](../../docs/walkthrough/cancellation-timezone-r12-walkthrough-2026-10-03.md)
- [x] **Tahap 4: Implementasi TDD (Target $\ge$ 80% Coverage)**
  - Definisikan `LocationWIB` dan `FreeCancellationDeadline` di paket `internal/booking`.
  - Pasang `s.nowFunc` dan `SetNowFunc` pada `booking.Service`.
  - Sesuaikan `Cancel()` di `internal/booking/service.go` menggunakan `FreeCancellationDeadline` dan `s.now()`.
  - Tulis table-driven test komprehensif di `internal/booking/service_test.go` yang menguji clock tepat sebelum, tepat pada, 1 detik sesudah, serta di dalam gap 7 jam lama (104 tests pass, Cancel statement coverage 85.7%).
  - Verifikasi: `go test -v -cover ./internal/booking/...` dan `go vet ./...` (clean).
- [x] **Tahap 5: End-to-End Testing & Verification**
  - Buat skrip E2E: [cancellation_timezone_r12_e2e.sh](../../testing/e2e/script/cancellation_timezone_r12_e2e.sh).
  - Jalankan pengujian terhadap test stack live (9/9 assertions PASS).
  - Dokumentasikan laporan: [2026-10-03-202800-cancellation-timezone-r12-e2e-report.md](../../testing/e2e/report/2026-10-03-202800-cancellation-timezone-r12-e2e-report.md).
- [x] **Tahap 6: Verification & Git Commit**
  - Update [07-backend-reaudit-2026-10-03.md](../../docs/gap/07-backend-reaudit-2026-10-03.md) dan [09-checkout-pricing-and-policy-reaudit-2026-10-03.md](../../docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md).
  - Git commit BE-R12.
