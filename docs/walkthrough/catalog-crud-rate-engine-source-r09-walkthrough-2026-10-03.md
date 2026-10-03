# Walkthrough: Integrasi Harga Katalog CRUD Sebagai Sumber Rate Engine Dinamis (BE-R09)

**Nomor Dokumen:** WT-PULANG-BE-R09  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1–5 Selesai, Siap Review & Commit)  
**Author:** AI Engineering Agent  
**Terkait:** BE-R09, BE-G01, BE-G04, BE-G05, F08, F09  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Investigasi keterikatan hardcoded `baseRates` di `main.go`.
  - [x] Analisis celah kamar gratis Rp 0 pada `searchRooms` di `internal/api/router.go`.
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: `docs/prd/catalog-crud-rate-engine-source-r09-2026-10-03.md`
  - [x] SRS: `docs/srs/catalog-crud-rate-engine-source-r09-2026-10-03.md`
  - [x] Tech Architecture: `docs/tech/catalog-crud-rate-engine-source-architecture-2026-10-03.md`
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Update `internal/rates/engine.go`:
    - Port `BaseRateSource` interface
    - Pointer `baseSource` dan proteksi `sync.RWMutex`
    - Method `SetBaseRateSource(src BaseRateSource)` dan `SetBaseRate(roomTypeID string, rateMinor int64)`
    - Resolusi dinamis pada `Quote(...)`: cek `baseSource`, validasi `base > 0`, return `ErrUnpricedRoomType`, fallback ke map
  - [x] Update `internal/api/router.go`:
    - `NewRouter`: otomatis inject `d.RateEngine.SetBaseRateSource(d.CatalogStore)` bila keduanya tersedia
    - `searchRooms`: evaluasi error `RateSvc.Quote` dan nilai total; tandai `available = false`, `unavailable_reason = "RATE_UNAVAILABLE"` bila unpriced
    - `calculateQuote`: tangani `rates.ErrUnpricedRoomType` dengan 400 `RATE_UNAVAILABLE`
    - `createCatalogRoom` & `updateCatalogRoom`: perbarui `d.RateEngine.SetBaseRate(...)`
  - [x] Unit & router tests di `internal/rates/engine_test.go` dan `internal/api/router_test.go`
  - [x] Verifikasi `rtk go test -v -cover ./...` (831 tests passing, `internal/rates` 95.8%, `internal/api` 85.1%) dan `rtk go vet ./...` (0 errors)
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: `testing/e2e/script/catalog_crud_rate_engine_r09_e2e.sh`
  - [x] Jalankan skrip E2E terhadap PostgreSQL & Valkey (24/24 assertions PASS)
  - [x] Catat hasil di `testing/e2e/report/2026-10-03-221000-catalog-crud-rate-engine-r09-e2e-report.md`
- [ ] **Tahap 6: Review & Final Report**
  - [x] Update audit gap manifest
  - [ ] Git commit & lapor ke user

---

## 2. Log Eksekusi & Catatan Modifikasi File

| File | Status | Keterangan Modifikasi |
| :--- | :--- | :--- |
| `internal/rates/engine.go` | Selesai | Port `BaseRateSource`, `SetBaseRateSource`, `SetBaseRate`, dynamic resolution `Quote`, error `ErrUnpricedRoomType` |
| `internal/rates/engine_test.go` | Selesai | Table-driven unit tests resolusi katalog, fallback map, unpriced room, dan tes konkurensi (Coverage: **95.8%**) |
| `internal/api/router.go` | Selesai | Inject `BaseRateSource` di `NewRouter`, pricing guard pada `searchRooms`, penanganan `ErrUnpricedRoomType` di `calculateQuote`, base rate sync di CRUD rooms |
| `internal/api/router_test.go` | Selesai | Router tests untuk search guard, quote error unpriced, dan sinkronisasi CRUD katalog real-time (Coverage: **85.1%**) |
| `cmd/server/main.go` | Selesai | Wire `rateEngine.SetBaseRateSource(catalogStore)` saat booting |
| `testing/e2e/script/e2e_runner_test.go` | Selesai | Wire `rateEngine.SetBaseRateSource(catalogStore)` di runner harness |
| `testing/e2e/script/catalog_crud_rate_engine_r09_e2e.sh` | Selesai | Skrip E2E live PostgreSQL & Valkey (24/24 assertions PASS) |
| `testing/e2e/report/2026-10-03-221000-catalog-crud-rate-engine-r09-e2e-report.md` | Selesai | Laporan lengkap pengujian E2E |
