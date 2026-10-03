# Walkthrough: Kebijakan Biaya Sarapan & Multi-Room Guest Tiers (BE-R10)

**Nomor Dokumen:** WT-PULANG-BE-R10  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1 ➔ Stage 6)  
**Author:** AI Engineering Agent  
**Terkait:** BE-R10, BE-G04, BE-G05, F01, F09  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Analisis bug double-counting sarapan pada multi-room booking di `rates/engine.go:296`.
  - [x] Riset kebijakan sarapan anak dan balita hotel bintang 4 Pulang ke Uttara Yogyakarta.
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: `docs/prd/breakfast-pricing-and-guest-tiers-r10-2026-10-03.md`
  - [x] SRS: `docs/srs/breakfast-pricing-and-guest-tiers-r10-2026-10-03.md`
  - [x] Tech Architecture: `docs/tech/breakfast-pricing-and-guest-tiers-architecture-2026-10-03.md`
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Update `internal/rates/engine.go`:
    - Konstanta `BreakfastRateChildPerNight = 50_000`
    - Perluasan struct `QuoteRequest` dan `LockedQuote` dengan `Adults`, `Children`, `ChildAges`
    - Perbaikan rumus sarapan: eliminasi pengali `numRooms`, dukung child tiers (0-5 thn gratis, 6-11 thn 50%, >=12 thn full)
  - [x] Update `internal/api/router.go`:
    - Teruskan `in.Adults`, `in.Children`, `in.ChildAges` ke `rates.QuoteRequest` di handler `calculateQuote`
  - [x] Table-driven unit tests di `internal/rates/engine_test.go` dan `internal/api/router_test.go`
  - [x] Verifikasi `go test -v -cover ./...` (838 tests PASS, `internal/rates` 96.2%, `internal/api` 85.1%) dan `go vet ./...` (0 errors)
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: `testing/e2e/script/breakfast_pricing_guest_tiers_r10_e2e.sh`
  - [x] Jalankan skrip E2E terhadap PostgreSQL & Valkey (23/23 asersi PASS)
  - [x] Catat hasil di `testing/e2e/report/2026-10-03-223500-breakfast-pricing-guest-tiers-r10-e2e-report.md`
- [x] **Tahap 6: Review & Final Report**
  - [x] Update audit gap manifest
  - [ ] Git commit & lapor ke user

---

## 2. Log Eksekusi & Catatan Modifikasi File

1. **`internal/rates/engine.go`**:
   - Menambahkan konstanta `BreakfastRateChildPerNight = 50_000`.
   - Menambahkan field `Adults`, `Children`, `ChildAges []int` pada `QuoteRequest` dan `LockedQuote`.
   - Mengubah rumus kalkulasi sarapan paket `bed_and_breakfast`:
     - Menghitung `dailyBreakfast` secara jujur berdasarkan komposisi tamu (dewasa + jenjang tarif anak: $<6$ gratis, $6-11$ diskon 50%, $\ge 12$ dewasa).
     - Menghapus pengali `* int64(numRooms)` agar pesanan multi-room tidak ditagih ganda/lipat tiga.
2. **`internal/api/router.go`**:
   - Memetakan field `in.Adults`, `in.Children`, `in.ChildAges` dari DTO input `calculateQuote` ke dalam `rates.QuoteRequest`.
3. **`internal/rates/engine_test.go`**:
   - Menambahkan skenario table-driven test: multi-room breakfast parity (2 kamar & 3 kamar) dan evaluasi berjenjang tarif anak.
4. **`internal/api/router_test.go`**:
   - Menambahkan skenario pengujian HTTP API untuk quote dengan child age tiers dan serialisasi JSON snapshot quote.
5. **`testing/e2e/script/breakfast_pricing_guest_tiers_r10_e2e.sh`**:
   - Menguji 5 skenario E2E live: Single-room, Multi-room 2 kamar (tanpa double counting), Multi-room 3 kamar, Family Child Tiers, dan siklus pemesanan multi-room lengkap hingga status `confirmed`.
