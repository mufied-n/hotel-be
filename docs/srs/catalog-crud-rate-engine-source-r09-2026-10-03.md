# SRS: Sinkronisasi Harga Katalog CRUD ke Dynamic Rate Engine & Pricing Guard (BE-R09)

**Nomor Dokumen:** SRS-PULANG-BE-R09  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R09, BE-G01, BE-G04, BE-G05, F08, F09  

---

## 1. Pendahuluan

Dokumen ini memuat spesifikasi teknis, antarmuka fungsional, dan kontrak HTTP untuk pengintegrasian tarif dasar kamar dari manajemen katalog CRUD (`room_types.base_price_minor`) ke dalam dynamic rate engine (`rates.Engine`), serta pencegahan kebocoran kamar gratis/tanpa tarif pada endpoint pencarian (`searchRooms`).

---

## 2. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Dynamic Base Rate Resolution pada Rate Engine
- `rates.Engine` mengadopsi interface port `BaseRateSource` dengan kontrak:
  ```go
  type BaseRateSource interface {
      GetVariant(ctx context.Context, idOrCode string) (catalog.RoomVariant, error)
  }
  ```
- Evaluasi tarif dasar per malam pada `Quote(ctx, roomTypeID, from, to)`:
  1. Jika `BaseRateSource` terkonfigurasi, panggil `GetVariant(ctx, roomTypeID)`.
     - Bila ditemukan dan `variant.BasePriceMinor > 0`, gunakan nilai tersebut sebagai tarif dasar.
     - Bila ditemukan namun `variant.BasePriceMinor <= 0`, kembalikan error `rates.ErrUnpricedRoomType`.
  2. Bila tidak ditemukan atau `BaseRateSource` bernilai nil, lakukan fallback ke map statis in-memory `e.base[roomTypeID]`.
     - Bila ada dan `base > 0`, gunakan tarif tersebut.
     - Bila ada namun `base <= 0`, kembalikan error `rates.ErrUnpricedRoomType`.
     - Bila tidak ada di kedua sumber, kembalikan `rates.ErrUnknownRoomType`.
- Perhitungan pengali akhir pekan (weekend multiplier) tetap berlaku (Jumat & Sabtu tarif dasar $\times$ `weekendFactor`).

### FR-02: Pricing Guard pada Search Rooms (`GET /api/v1/search`)
- Saat melakukan iterasi ketersediaan kamar pada varian katalog:
  - Panggil `d.RateSvc.Quote(ctx, v.ID, from, to)`.
  - Jika terjadi error (`err != nil`), atau panjang slice kuotasi kosong (`len(quotes) == 0`), atau kalkulasi `total_price_minor <= 0`:
    - Kamar **DILARANG KERAS** ditandai sebagai `available = true` atau `total_price_minor = 0`.
    - Set `item.Available = false`.
    - Set `item.UnavailableReason = "RATE_UNAVAILABLE"`.
    - Masukkan `item` ke `results` dan `continue` ke varian kamar berikutnya (tidak dihitung dalam `available_count`).

### FR-03: Quote Guard pada Locked Quotes (`POST /api/v1/quotes`)
- Jika kamar belum memiliki konfigurasi tarif dasar (`rates.ErrUnpricedRoomType`):
  - Handler mengembalikan HTTP 400 Bad Request:
    ```json
    {
      "error": "tarif dasar kamar belum dikonfigurasi",
      "code": "RATE_UNAVAILABLE"
    }
    ```
- Jika tipe kamar tidak dikenali di katalog maupun map tarif (`rates.ErrUnknownRoomType`):
  - Handler mengembalikan HTTP 404 Not Found:
    ```json
    {
      "error": "tipe kamar tidak ditemukan",
      "code": "ROOM_NOT_FOUND"
    }
    ```

### FR-04: Real-time Sinkronisasi Pembaruan Katalog CRUD
- Pada endpoint `PUT /api/v1/catalog/rooms/:id`:
  - Setelah `d.CatalogStore.UpdateVariant` sukses dieksekusi:
    - Jika `d.RateEngine != nil`, panggil `d.RateEngine.SetBaseRate(updated.ID, updated.BasePriceMinor)`.
    - Permintaan kuotasi baru (`POST /api/v1/quotes`) dan pencarian berikutnya (`GET /api/v1/search`) secara langsung menggunakan tarif baru tersebut.
- Pada endpoint `POST /api/v1/catalog/rooms`:
  - Setelah `d.CatalogStore.CreateVariant` sukses dieksekusi:
    - Jika `d.RateEngine != nil`, panggil `d.RateEngine.SetBaseRate(created.ID, created.BasePriceMinor)`.
    - Varian baru tersebut langsung dapat dicari di search dan dibuat kuotasinya.

### FR-05: Imutabilitas Quote yang Telah Terkunci
- Quote yang telah tersimpan dalam `QuoteStore` (`LockedQuote`) mempertahankan seluruh komponen harga beku (`PricingBreakdown`) sampai masa berlaku TTL (15 menit) berakhir, terlepas dari apakah ada perubahan harga katalog setelah quote diterbitkan.

---

## 3. Spesifikasi Kontrak HTTP

### 3.1 `GET /api/v1/search`
* Query Parameters: `check_in`, `check_out`, `rooms`, `adults`, `children`.
* Respons Item saat Kamar Belum Memiliki Tarif Valid:
```json
{
  "search_criteria": {
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "nights": 2,
    "rooms": 1,
    "adults": 2,
    "children": 0
  },
  "total_variants": 7,
  "available_count": 6,
  "variants": [
    {
      "id": "01900000-0000-7000-8000-000000000099",
      "code": "unpriced-room",
      "name": "Unpriced Attic",
      "available": false,
      "unavailable_reason": "RATE_UNAVAILABLE",
      "available_rooms": 3,
      "total_price_minor": 0,
      "quotes": null
    }
  ]
}
```

### 3.2 `POST /api/v1/quotes`
* Request Body:
```json
{
  "room_type_id": "01900000-0000-7000-8000-000000000099",
  "rate_plan_code": "room_only",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "num_rooms": 1,
  "num_guests": 2
}
```
* Error Response (HTTP 400 Bad Request):
```json
{
  "error": "tarif dasar kamar belum dikonfigurasi",
  "code": "RATE_UNAVAILABLE"
}
```
