# Walkthrough Tracker — Batch BE-C: Tarif, Paket, Quote Engine & Kebijakan Pembatalan
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **Completed & Verified**
- **Feature Target:** Implementasi Dynamic Rate Plans, Money Contract, 15-Minute Quote Lock Engine, & Cancellation Policy Enforcement (`BE-G04`, `BE-G05`, `BE-G06`, `BE-G08`, `BE-G19`)

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset & Analisis Kebutuhan**
  - [x] Riset standar rate plan hotel bintang 4 (Room Only vs Bed & Breakfast)
  - [x] Riset quote TTL 15 menit & kepatuhan pajak perhotelan Yogyakarta PB1 (10%)
  - [x] PRD: [`docs/prd/pricing-quote-policies-batch-c-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/pricing-quote-policies-batch-c-2026-10-03.md)
  - [x] SRS: [`docs/srs/pricing-quote-policies-batch-c-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/pricing-quote-policies-batch-c-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/pricing-quote-policies-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/pricing-quote-policies-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/pricing-quote-policies-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/pricing-quote-policies-walkthrough-2026-10-03.md)

- [x] **Fase 2: Rate & Quote Engine (`internal/rates`)**
  - [x] Definisikan `Money`, `PricingBreakdown`, `Quote`, dan `QuoteStore`
  - [x] Implementasikan `MemoryQuoteStore` dengan thread-safety (`sync.RWMutex`)
  - [x] Implementasikan `CalculateLockedQuote` dengan integrasi promo `OCTOBREAK` (diskon 15%), sarapan Rp 100.000, pajak 10% PB1, dan UUIDv7
  - [x] Table-driven tests di `internal/rates/engine_test.go` (**90.5% statement coverage**)

- [x] **Fase 3: Migrasi DB & Booking Domain Service (`internal/booking`)**
  - [x] Migrasi Goose SQL [`migrations/00006_pricing_and_policies.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00006_pricing_and_policies.sql)
  - [x] Perbarui model `Booking` dengan field quote, breakdown harga, kebijakan pembatalan, dan persetujuan syarat
  - [x] Perbarui `service.Create()`: validasi terms consent, ambil quote dari store, validasi TTL (15m), validasi kecocokan parameter, kunci harga dari quote snapshot
  - [x] Perbarui `service.Cancel()`: tolak pembatalan pesanan `non_refundable`, validasi batas waktu H-2 (48 jam) untuk `flexible_48h`, izinkan pelepasan hold `pending`
  - [x] Table-driven tests di `internal/booking/service_test.go` (**PASS 100%, service.Create: 91.0% coverage**)

- [x] **Fase 4: HTTP Transport Layer (`internal/api`)**
  - [x] Endpoint `POST /api/v1/quotes` dengan evaluasi aturan Casbin `guest`
  - [x] Update `POST /api/v1/bookings` (menerima `quote_id`, `terms_accepted`, `privacy_accepted` dengan error mapping RFC 7807)
  - [x] Update `POST /api/v1/bookings/:id/cancel` (penanganan error `NON_REFUNDABLE_BOOKING` & `CANCELLATION_DEADLINE_EXCEEDED`)
  - [x] Table-driven tests di `internal/api/router_test.go` (**89.0% statement coverage**)

- [x] **Fase 5: End-to-End (E2E) Automation & Verification**
  - [x] Tambahkan skenario Quote & Policy pada `testing/e2e/script/e2e_runner_test.go` (E2E-14 sampai E2E-17)
  - [x] Tambahkan skenario Quote & Policy pada `testing/e2e/script/hotel_booking_rbac_e2e.sh` (Tests 9-12)
  - [x] Dokumentasikan laporan E2E di [`testing/e2e/report/2026-10-03-024800-pricing-quote-policies-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-024800-pricing-quote-policies-e2e-report.md)

- [x] **Fase 6: Verification & Final Sign-Off**
  - [x] `go vet ./...` (0 errors)
  - [x] `go test -v ./...` (100% pass)
  - [x] Pelaporan ke user dan permintaan konfirmasi commit

---

## Log Hasil Pengujian & Bukti Eksekusi

```text
=== RUN   TestEndToEndHotelBookingRBACLifecycle
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-14:_Guest_requests_locked_quote_with_BB_and_OCTOBREAK_promo
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-15:_Guest_attempts_to_book_without_consent_(400_CONSENT_REQUIRED)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-16:_Guest_creates_booking_with_locked_quote_snapshot_and_consent
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-17:_Confirmed_promo_booking_cannot_be_cancelled_by_guest_(409_Conflict)
--- PASS: TestEndToEndHotelBookingRBACLifecycle (0.02s)
PASS
ok  	github.com/example/hotel-booking/testing/e2e/script	0.021s

Coverage:
ok  	github.com/example/hotel-booking/internal/api         0.026s  coverage: 89.0% of statements
ok  	github.com/example/hotel-booking/internal/rates       0.062s  coverage: 90.5% of statements
ok  	github.com/example/hotel-booking/internal/catalog     0.003s  coverage: 95.8% of statements
ok  	github.com/example/hotel-booking/internal/inventory   0.003s  coverage: 100.0% of statements
ok  	github.com/example/hotel-booking/internal/platform/auth 0.004s coverage: 93.3% of statements
ok  	github.com/example/hotel-booking/internal/workers     0.002s  coverage: 84.6% of statements
```
