# Laporan Pengujian End-to-End (E2E): Integrasi Harga Katalog CRUD ke Dynamic Rate Engine & Pricing Guard (BE-R09)

**Nomor Laporan:** E2E-REP-PULANG-BE-R09  
**Tanggal Pengujian:** 3 Oktober 2026, 22:15 WIB  
**Target Fitur:** `BE-R09` (Harga katalog CRUD tidak menjadi sumber rate engine)  
**Terkait:** BE-G01, BE-G04, BE-G05, F08, F09  
**Lingkungan Uji:** PostgreSQL 18 (`r09-pg`), Valkey 8 (`r09-vk`), Go 1.24 HTTP Server (:28080)  
**Skrip Automasi:** [`testing/e2e/script/catalog_crud_rate_engine_r09_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/catalog_crud_rate_engine_r09_e2e.sh)  
**Hasil Eksekusi:** **24/24 Assertions PASSED (100% Lulus)**  

---

## 1. Ringkasan Eksekusi Pengujian

Pengujian end-to-end ini menguji secara langsung integrasi harga dasar varian kamar dari manajemen katalog CRUD (`room_types.base_price_minor`) dengan dynamic rate engine (`rates.Engine`), serta memverifikasi penegakan invariant *pricing guard* pada endpoint pencarian (`searchRooms`) dan kuotasi harga (`calculateQuote`):
1. **Resolusi Dinamis Tarif Awal:** `rates.Engine` membaca tarif dasar secara langsung dari tabel `room_types` melalui port `BaseRateSource` (Catalog Store).
2. **Pembaruan Harga Real-time (CRUD PUT):** Revenue Manager mengubah harga dasar kamar via `PUT /api/v1/catalog/rooms/:id`, perubahan tersebut langsung tercermin pada kuotasi baru (`POST /api/v1/quotes`) dan pencarian kamar (`GET /api/v1/search`) tanpa perlu restart aplikasi.
3. **Pendaftaran Varian Baru (CRUD POST):** Admin membuat varian kamar baru via `POST /api/v1/catalog/rooms`, varian baru tersebut langsung dapat dihitung kuotasinya dan dicari oleh tamu.
4. **Pencegahan Celah Kamar Gratis (Pricing Guard):** Kamar yang tidak memiliki tarif dasar atau bernilai Rp 0 tidak lagi bocor ke tamu publik dengan status `available: true`. Kamar tersebut otomatis ditandai `available: false` dengan alasan `RATE_UNAVAILABLE` dan kuotasi kamar tersebut ditolak HTTP 400 `RATE_UNAVAILABLE`.
5. **Imutabilitas Quote Terkunci (Locked Quote Price Freeze):** Quote yang telah terkunci sebelum pembaruan harga katalog tetap mempertahankan tarif snapshot aslinya saat tamu menyelesaikan proses checkout dan pembayaran.

```
=================================================================
  E2E Test: BE-R09 Sinkronisasi Harga Katalog ke Rate Engine     
=================================================================
  Total Assertions : 24
  Passed           : 24
  Failed           : 0
  ALL E2E ASSERTIONS PASSED!
=================================================================
```

---

## 2. Rincian Skenario & Bukti Respons HTTP

### 2.1 Resolusi Tarif Kamar Awal (Superior King: Rp 550.000)
* **Request:** `POST /api/v1/quotes` (2 malam weekday)
* **Kalkulasi:** 2 malam $\times$ Rp 550.000 = Rp 1.100.000 + PB1 10% (Rp 110.000) = Rp 1.210.000
* **Hasil:**
  * HTTP Status: `200 OK`
  * Subtotal: `1100000` (PASS)
  * Total: `1210000` (PASS)

### 2.2 Pembaruan Harga Katalog CRUD (PUT /api/v1/catalog/rooms/:id)
* **Request:** `PUT /api/v1/catalog/rooms/01900000-0000-7000-8000-000000000001`
  * Authorization: `Bearer <T_REVENUE_MGR>`
  * Base Price Minor: `700000`
* **Hasil Pembaruan:** `200 OK`, `base_price_minor: 700000`
* **Verifikasi Quote Baru:**
  * Subtotal: `1400000` (2 $\times$ 700.000) (PASS)
  * Total: `1540000` (PASS)
* **Verifikasi Search Rooms:**
  * `SK_SEARCH_PRICE`: `1400000` (PASS)

### 2.3 Pembuatan Varian Baru CRUD (POST /api/v1/catalog/rooms)
* **Request:** `POST /api/v1/catalog/rooms`
  * Authorization: `Bearer <T_GM_ADMIN>`
  * Code: `penthouse-<timestamp>`, Base Price Minor: `4500000`
* **Hasil Pembuatan:** `201 Created`
* **Verifikasi Quote Baru:**
  * Subtotal: `9000000` (2 $\times$ 4.500.000) (PASS)
  * Total: `9900000` (PASS)

### 2.4 Pricing Guard pada Search & Quote (Pencegahan Kamar Gratis / Unpriced)
* **Kondisi:** Simulasi varian kamar korup/tanpa tarif (`base_price_minor: 0`) pada PostgreSQL.
* **Pengujian Quote:**
  * Request: `POST /api/v1/quotes` dengan room type ID kamar unpriced
  * Respons Status: `400 Bad Request` (PASS)
  * Error Code: `RATE_UNAVAILABLE` (PASS)
  * Payload:
    ```json
    {
      "error": "tarif dasar kamar belum dikonfigurasi",
      "code": "RATE_UNAVAILABLE"
    }
    ```
* **Pengujian Search:**
  * `available`: `false` (PASS)
  * `unavailable_reason`: `"RATE_UNAVAILABLE"` (PASS)
  * `total_price_minor`: `0` (PASS)
  * `quotes`: `[]` (PASS)

### 2.5 Imutabilitas Quote yang Telah Terkunci
* **Kondisi:** Quote diterbitkan saat tarif Rp 700.000 (total Rp 1.540.000). Kemudian tarif kamar dinaikkan menjadi Rp 900.000 di katalog.
* **Proses Booking:** `POST /api/v1/bookings` membawa `quote_id` beku.
  * Status: `201 Created` (PASS)
  * Tagihan Booking: `1540000` (Tetap beku pada Rp 1.540.000, bukan Rp 1.980.000) (PASS)
* **Pembayaran Fake Gateway:** `200 OK` (PASS)
* **Status Akhir Reservasi:** `confirmed` (PASS)

---

## 3. Kesimpulan Verifikasi

Fitur BE-R09 telah teruji 100% lulus pada lingkungan live PostgreSQL 18 dan Valkey 8. Tidak ada lagi disparitas antara harga CRUD katalog dan perhitungan mesin tarif, dan potensi kebocoran kamar gratis (Rp 0) pada search publik telah tertutup rapat.
