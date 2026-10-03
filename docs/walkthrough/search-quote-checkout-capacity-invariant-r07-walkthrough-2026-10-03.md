# Walkthrough Tracking: Invariant Kapasitas Kamar Search, Quote, dan Checkout (BE-R07)

**Tanggal Mulai:** 3 Oktober 2026  
**Status:** Completed (Tahap 5 E2E Selesai, Siap Commit)  
**Terkait:** BE-R07, BE-G03, F01, F08  

---

## 1. Rencana Eksekusi Bertahap

- [x] **Tahap 1: Riset Mendalam (Research Phase)**
  - [x] Analisis gap `BE-R07` pada audit backend.
  - [x] Verifikasi spesifikasi kapasitas 7 varian kamar resmi Pulang ke Uttara.
  - [x] Evaluasi celah bypass search pada quote dan checkout.

- [x] **Tahap 2: Analisis & Dokumen Spesifikasi**
  - [x] PRD: `docs/prd/search-quote-checkout-capacity-invariant-r07-2026-10-03.md`
  - [x] SRS: `docs/srs/search-quote-checkout-capacity-invariant-r07-2026-10-03.md`
  - [x] Desain Arsitektur Teknis: `docs/tech/search-quote-checkout-capacity-invariant-architecture-2026-10-03.md`

- [x] **Tahap 3: Penyusunan Rencana & Walkthrough Tracking**
  - [x] Dokumen walkthrough: `docs/walkthrough/search-quote-checkout-capacity-invariant-r07-walkthrough-2026-10-03.md`

- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Tambahkan domain error `ErrExceedsCapacity` di `internal/booking/booking.go`.
  - [x] Tambahkan port `CatalogReader` dan method `SetCatalogStore` di `internal/booking/service.go`.
  - [x] Terapkan penegakan kapasitas fisik katalog pada `booking.Service.Create`.
  - [x] Terapkan validasi `adults < rooms`, `child_ages` count equality, dan `MaxChildren` pada `searchRooms` di `internal/api/router.go`.
  - [x] Terapkan validasi kapasitas varian terhadap `CatalogStore` pada `calculateQuote` di `internal/api/router.go`.
  - [x] Terapkan validasi kapasitas varian terhadap `CatalogStore` pada `createBooking` di `internal/api/router.go`.
  - [x] Sambungkan `bkSvc.SetCatalogStore(catalogStore)` di `cmd/server/main.go` dan `testing/e2e/script/e2e_runner_test.go`.
  - [x] Tulis table-driven unit tests di `internal/api/router_test.go` dan `internal/booking/service_test.go`.
  - [x] Verifikasi `rtk go test -v -cover ./...` $\ge 80\%$ (Coverage `internal/api`: 85.1%, `booking.Create`: 94.0%) dan `rtk go vet ./...` 100% clean.

- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian `testing/e2e/script/search_quote_checkout_capacity_r07_e2e.sh`.
  - [x] Jalankan pengujian terhadap skenario: bypass search overcapacity, quote overcapacity, child age count mismatch, adults < rooms, valid boundary.
  - [x] Dokumentasikan laporan E2E di `testing/e2e/report/2026-10-03-213000-search-quote-checkout-capacity-r07-e2e-report.md` (35/35 assertions passed, 100%).

- [ ] **Tahap 6: Verification & Commit Integrity**
  - [ ] Update status `BE-R07` menjadi `RESOLVED` di dokumen audit gap (`docs/gap/07-backend-reaudit-2026-10-03.md` dan `docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md`).
  - [ ] Git commit dengan pesan konvensional.

---

## 2. File yang Dimodifikasi / Dibuat

| File | Status | Keterangan |
| :--- | :--- | :--- |
| `docs/prd/search-quote-checkout-capacity-invariant-r07-2026-10-03.md` | Selesai | PRD Invariant Kapasitas |
| `docs/srs/search-quote-checkout-capacity-invariant-r07-2026-10-03.md` | Selesai | SRS Kontrak API & Error |
| `docs/tech/search-quote-checkout-capacity-invariant-architecture-2026-10-03.md` | Selesai | Desain Teknis Arsitektur |
| `docs/walkthrough/search-quote-checkout-capacity-invariant-r07-walkthrough-2026-10-03.md` | Selesai | Tracking Dokumen ini |
| `internal/booking/booking.go` | Selesai | Domain error `ErrExceedsCapacity` |
| `internal/booking/service.go` | Selesai | Port `CatalogReader` dan penegakan invariant di `Create` |
| `internal/catalog/catalog.go` | Selesai | Dukungan mock alias `std` di `MemoryStore.GetVariant` |
| `internal/catalog/postgres.go` | Selesai | Dukungan alias `std` di `PostgresStore.GetVariant` |
| `internal/api/router.go` | Selesai | Validasi search, quote, dan create booking |
| `cmd/server/main.go` | Selesai | Wiring `bkSvc.SetCatalogStore` dan `bkSvc.SetQuoteStore` |
| `testing/e2e/script/e2e_runner_test.go` | Selesai | Injeksi test harness |
| `internal/api/router_test.go` | Selesai | Table-driven unit tests (23 subtests) |
| `internal/booking/service_test.go` | Selesai | Table-driven unit tests (5 subtests) |
| `testing/e2e/script/search_quote_checkout_capacity_r07_e2e.sh` | Selesai | Skrip E2E test (35 assertions) |
| `testing/e2e/report/2026-10-03-213000-search-quote-checkout-capacity-r07-e2e-report.md` | Selesai | Laporan hasil uji E2E (100% lulus) |
