# Walkthrough — F04: Booking Confirmation Artifacts (Printable Invoice & iCalendar .ics)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **PRD:** [PRD-F04-Artifacts](../prd/booking-confirmation-artifacts-f04-2026-10-03.md)
- **SRS:** [SRS-F04-Artifacts](../srs/booking-confirmation-artifacts-f04-2026-10-03.md)
- **Tech Architecture:** [TECH-F04-Artifacts](../tech/booking-confirmation-artifacts-f04-architecture-2026-10-03.md)

---

## Progress Tracking Matrix

| Tahap | Aktivitas | Status | Target Evidence |
|---|---|:---:|---|
| **Tahap 1** | Riset Pasar, Kompetitor, User Persona, dan Engineering RFC 5545 | **DONE** | PRD, SRS, Tech Architecture |
| **Tahap 2** | Pembuatan Dokumen PRD, SRS, & Desain Teknis | **DONE** | `docs/prd/`, `docs/srs/`, `docs/tech/` |
| **Tahap 3** | Walkthrough Tracking & Milestone Log | **DONE** | `docs/walkthrough/` |
| **Tahap 4** | Implementasi TDD (Table-Driven Tests $\ge 80\%$ Coverage) | **DONE** | Coverage 81.3%, `go vet` 0 errors |
| **Tahap 5** | Automasi Pengujian E2E & Pembuatan Laporan | **DONE** | 40/40 E2E tests pass, report created |
| **Tahap 6** | Verifikasi Akhir & Konfirmasi Git Commit | **AWAITING USER CONFIRMATION** | Commit selektif tanpa untracked user files |


---

## File Modification Plan

1. **`internal/guest/model.go`**:
   - Tambahkan struct `ReceiptDTO`, `HotelInfo`, `StayDetails`, `GuestDetails`, `RoomItemReceipt`, `PricingBreakdown`, `PaymentSummary`, `PoliciesReceipt`.
2. **`internal/guest/store.go` & `internal/guest/postgres.go`**:
   - Tambahkan method `GetBookingReceiptData(ctx context.Context, email, bookingID string) (*ReceiptDTO, error)` dengan kueri join ke `bookings`, `room_types`, dan `payment_attempts`.
3. **`internal/guest/service.go`**:
   - Tambahkan method `GetBookingReceipt(ctx context.Context, email, bookingID string) (*ReceiptDTO, error)`.
   - Tambahkan method `GenerateBookingICS(receipt *ReceiptDTO) ([]byte, error)` dengan generator iCalendar RFC 5545 pure Go.
4. **`internal/api/guest_auth.go` & `internal/api/router.go`**:
   - Daftarkan endpoint `GET /api/v1/guest/bookings/{id}/receipt` dan `GET /api/v1/guest/bookings/{id}/calendar.ics` di dalam sub-router `requireGuestSession`.
5. **`internal/guest/service_test.go` & `internal/api/guest_api_test.go`**:
   - Table-driven unit test komprehensif menguji skenario valid receipt, status pending (ditolak), IDOR attempt, valid iCal format & RFC 5545 properties.
6. **`testing/e2e/script/e2e_runner_test.go`**:
   - Tambahkan skenario E2E-37 (Download Printable Invoice DTO) dan E2E-38 (Download RFC 5545 iCalendar stream).
