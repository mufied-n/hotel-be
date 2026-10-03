# PRD: Integrasi Harga Katalog CRUD Sebagai Sumber Rate Engine Dinamis (BE-R09)

**Nomor Dokumen:** PRD-PULANG-BE-R09  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R09, BE-G01, BE-G04, BE-G05, F08, F09  

---

## 1. Konteks Bisnis & Latar Belakang

Pada operasional perhotelan bintang 4 "Pulang ke Uttara" (Yogyakarta, 95 kamar), Revenue Manager dan manajemen hotel secara berkala memperbarui tarif dasar kamar (*base price*) dan dapat menambahkan atau mengaktifkan varian kamar baru melalui portal manajemen katalog hotel (`POST /api/v1/catalog/rooms` dan `PUT /api/v1/catalog/rooms/:id`).

### Permasalahan (Gap BE-R09)
1. **Harga Statis Hardcoded:** `cmd/server/main.go` menginisialisasi `rates.NewEngine` menggunakan sebuah Go map statis (`baseRates`) berisi 7 ID kamar awal. Akibatnya, pembaruan `base_price_minor` oleh Revenue Manager melalui endpoint CRUD katalog tidak pernah berpengaruh pada perhitungan harga di `rateEngine`!
2. **Kamar Baru Tidak Dapat Dihitung Tarifnya:** Bila Revenue Manager membuat varian kamar baru (misal: Family Loft atau Penthouse), kamar tersebut tersimpan di tabel `room_types`, tetapi `rateEngine` menolak perhitungannya dengan `ErrUnknownRoomType`.
3. **Pencarian Membocorkan Kamar Gratis (Rp 0):** Endpoint pencarian publik `GET /api/v1/search` (`searchRooms`) mengabaikan error dari `d.RateSvc.Quote(v.ID)`. Bila suatu kamar tidak memiliki konfigurasi tarif atau rate engine mengembalikan error, kamar tersebut tetap ditampilkan dengan status `available: true` dan `total_price_minor: 0`. Tamu berpotensi memesan kamar tanpa tarif!
4. **Disparitas UI Admin vs Checkout:** UI admin menampilkan harga dari `room_types.base_price_minor`, sedangkan tamu pada alur checkout membayar berdasarkan map statis `baseRates`.

---

## 2. Persona & Pengguna Terdampak

1. **Revenue Manager:**
   - Mengharapkan pembaruan harga dasar kamar pada katalog langsung berlaku secara real-time pada penawaran kuotasi harga dan pencarian kamar baru.
   - Dapat menambah varian kamar baru lengkap dengan tarif dasar yang langsung bookable.

2. **Guest (Tamu Publik):**
   - Mendapatkan kepastian harga yang transparan dan akurat.
   - Tidak disajikan kamar tanpa tarif (*unpriced rooms*) atau kamar gratis palsu yang akan menimbulkan sengketa saat check-in.

3. **Front Desk & Keuangan:**
   - Menghindari konflik tarif antara harga promosi/katalog yang tertera di sistem dengan nominal pada invoice pembayaran Xendit.

---

## 3. Matriks Kebijakan & Invariant Harga

| Komponen | Perilaku Saat Ini | Perilaku Baru (BE-R09) |
| :--- | :--- | :--- |
| **Sumber Tarif Rate Engine** | Hardcoded Go map di `main.go` | Dinamis membaca dari `CatalogReader` (`room_types.base_price_minor`) dengan in-memory fallback |
| **Pembaruan Tarif (CRUD PUT)** | Diabaikan oleh checkout/quote | Langsung memengaruhi kuotasi baru (`calculateQuote` & `searchRooms`) |
| **Varian Kamar Baru (CRUD POST)** | Error 404 pada quote, 0 pada search | Otomatis memiliki tarif sesuai `base_price_minor` dan langsung bookable |
| **Kamar Tanpa Tarif Valid** | `available: true, price: 0` (Kamar Gratis) | `available: false, unavailable_reason: "RATE_UNAVAILABLE"` |
| **Quote Terkunci Lama (TTL 15m)** | - | Snapshot harga quote terkunci tetap dipertahankan (*immutable price freeze*) hingga kedaluwarsa |

---

## 4. Kriteria Penerimaan (Acceptance Criteria)

1. **AC-01 (Dynamic Base Rate Resolution):**
   - `rates.Engine` membaca tarif dasar secara dinamis dari `BaseRateSource` (port `catalog.Store`).
   - Jika varian kamar memiliki `BasePriceMinor > 0`, harga tersebut digunakan sebagai tarif dasar kalkulasi malam hari (termasuk pengali weekend 1.25x).

2. **AC-02 (CRUD Update Affects New Quotes Immediately):**
   - Ketika Revenue Manager memperbarui `base_price_minor` varian kamar via `PUT /api/v1/catalog/rooms/:id`, panggilan berikutnya ke `POST /api/v1/quotes` dan `GET /api/v1/search` mencerminkan harga baru tersebut.

3. **AC-03 (New Variant Bookability):**
   - Varian kamar baru yang dibuat via `POST /api/v1/catalog/rooms` dengan `base_price_minor` valid dapat langsung dicari di search dan dibuat kuotasinya di quotes.

4. **AC-04 (Zero Free/Unpriced Room Leakage in Search):**
   - Endpoint `GET /api/v1/search` mengevaluasi error dari `RateSvc.Quote`.
   - Bila kalkulasi tarif gagal atau kamar tidak memiliki tarif (`base <= 0` atau `ErrUnknownRoomType`), kamar **WAJIB** ditandai `available: false` dengan `unavailable_reason: "RATE_UNAVAILABLE"`. Tidak boleh ada kamar bernilai Rp 0 yang berstatus available.

5. **AC-05 (Quote Snapshot Immutability):**
   - Pembaruan tarif dasar pada katalog TIDAK mengubah `TotalPriceMinor` dari quote yang sudah terkunci (`LockedQuote`) sebelum TTL 15 menit berakhir.
