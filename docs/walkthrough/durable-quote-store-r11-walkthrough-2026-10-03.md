# Walkthrough: Penyimpanan Kuotasi Harga Terdistribusi (Durable Quote Store) (BE-R11)

**Nomor Dokumen:** WT-PULANG-BE-R11  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1 ➔ Stage 6)  
**Author:** AI Engineering Agent  
**Terkait:** BE-R11, BE-G06, F01, F05  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Analisis kelemahan `MemoryQuoteStore` pada multi-instance dan restart container.
  - [x] Analisis isu silent persistence failure pada `engine.go:387`.
  - [x] Verifikasi kapabilitas Valkey 8 (`github.com/redis/go-redis/v9`).
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: `docs/prd/durable-quote-store-r11-2026-10-03.md`
  - [x] SRS: `docs/srs/durable-quote-store-r11-2026-10-03.md`
  - [x] Tech Architecture: `docs/tech/durable-quote-store-architecture-2026-10-03.md`
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Buat adapter `internal/rates/valkey_store.go`:
    - `ValkeyQuoteStore` struct dengan `redis.Cmdable`
    - `SaveQuote` dengan TTL dinamis berbasis `ExpiresAt`
    - `GetQuote` dengan deserialisasi JSON dan verifikasi expired
    - Error `ErrSaveQuoteFailed`
  - [x] Modifikasi `internal/rates/engine.go`:
    - Tambahkan method `SetQuoteStore(qs QuoteStore)`
    - Perbaiki `CalculateLockedQuote` agar mempropagasi error dari `SaveQuote` (fail-closed)
  - [x] Modifikasi `internal/api/router.go`:
    - Tangani `ErrSaveQuoteFailed` dengan HTTP 500 `INTERNAL_ERROR`
  - [x] Wiring `cmd/server/main.go`:
    - Gunakan `NewValkeyQuoteStore(redisClient, 15*time.Minute)`
  - [x] Table-driven unit tests di `internal/rates/valkey_store_test.go` & `internal/rates/engine_test.go`
  - [x] Verifikasi `go test -v -cover ./...` (849 unit tests PASS, coverage `internal/rates` 96.5%) dan `go vet ./...` (0 errors)
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: `testing/e2e/script/durable_quote_store_r11_e2e.sh`
  - [x] Uji multi-instance crossover (Quote dibuat di instance 1, dibooking di instance 2)
  - [x] Uji persistensi quote melintasi instance restart
  - [x] Uji quote expiration di Valkey
  - [x] Uji propagasi HTTP 500 saat persistensi Valkey gagal
  - [x] Catat hasil di `testing/e2e/report/2026-10-03-231500-durable-quote-store-r11-e2e-report.md` (14/14 asersi PASS)
- [x] **Tahap 6: Review & Final Report**
  - [x] Update audit gap manifest
  - [ ] Git commit & lapor ke user

---

## 2. Log Eksekusi & Catatan Modifikasi File

1. **`internal/rates/valkey_store.go`**:
   - Dibuat adapter `ValkeyQuoteStore` mengimplementasikan port `QuoteStore`.
   - Menggunakan format key `hotel:quote:{id}` dengan masa simpan TTL terdistribusi (15 menit).
2. **`internal/rates/engine.go`**:
   - Menambahkan method `SetQuoteStore(qs QuoteStore)`.
   - Menjadikan `CalculateLockedQuote` fail-closed: kegagalan `SaveQuote` membatalkan pembentukan kuotasi dan mengembalikan `ErrSaveQuoteFailed`.
3. **`internal/api/router.go`**:
   - Memetakan `ErrSaveQuoteFailed` ke HTTP 500 Internal Server Error dengan kode `INTERNAL_ERROR`.
4. **`cmd/server/main.go`**:
   - Menyambungkan `rates.NewValkeyQuoteStore(redisClient, 15*time.Minute)` ke dalam `rateEngine`.
5. **`internal/rates/valkey_store_test.go`**:
   - Table-driven unit test dengan mock `redis.Cmdable` mencakup: roundtrip TTL, invalid ID, expired verification, JSON error, connection failure propagation, fail-closed engine test, dan router HTTP 500 response test.
6. **`testing/e2e/script/durable_quote_store_r11_e2e.sh`**:
   - Pengujian live multi-instance dengan 2 replika server, restart instance durability, TTL automated expiration, dan failover test.
