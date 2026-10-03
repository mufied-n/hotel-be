# SRS: Spesifikasi Kebutuhan Perangkat Lunak Invariant Kapasitas Kamar (BE-R07)

**Nomor Dokumen:** SRS-PULANG-BE-R07  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R07, BE-G03, F01, F08  

---

## 1. Pendahuluan

Dokumen ini mendefinisikan spesifikasi kebutuhan teknis, antarmuka REST API, skema payload JSON, dan kode status kesalahan untuk penegakan invariant kapasitas varian kamar hotel di seluruh alur reservasi (Search, Quote, dan Create Booking).

---

## 2. Kebutuhan Fungsional (Functional Requirements)

- **FR-01 (Search Parameter Integrity):**
  - Parameter `rooms` wajib integer positif dalam rentang $[1, 8]$.
  - Parameter `adults` wajib integer positif $\ge 1$ dan wajib $\ge rooms$. Pelanggaran mengembalikan kode error `INVALID_GUEST_COUNT`.
  - Parameter `children` wajib integer non-negatif $\ge 0$.
  - Parameter `child_ages`:
    - Jika `children == 0` dan `child_ages` dikirim dengan nilai bukan string kosong, sistem mengembalikan status 400 dengan error `CHILD_AGE_COUNT_MISMATCH`.
    - Jika `children > 0`:
      - Setiap umur anak wajib dalam rentang $0 \le age \le 17$. Pelanggaran mengembalikan 400 `INVALID_CHILD_AGE`.
      - Jumlah umur yang terurai dari pemisah koma wajib tepat sama dengan `children`. Pelanggaran mengembalikan 400 `CHILD_AGE_COUNT_MISMATCH`.

- **FR-02 (Search Variant Capacity Evaluation):**
  - Pada iterasi varian kamar dalam katalog:
    - Jika $(adults + children) > v.MaxCapacity \times rooms$ atau $adults > v.MaxAdults \times rooms$ atau $children > v.MaxChildren \times rooms$:
      - Item varian diset `available = false` dengan `unavailable_reason = "EXCEEDS_CAPACITY"`.

- **FR-03 (Quote Calculation Capacity Enforcement):**
  - Endpoint `POST /api/v1/quotes` memverifikasi keberadaan varian pada `CatalogStore`:
    - Jika varian tidak ditemukan $\rightarrow$ HTTP 404 `ROOM_NOT_FOUND`.
    - Memeriksa batas kapasitas kamar:
      - Jika $num\_rooms < 1$ atau $num\_rooms > 8$ $\rightarrow$ HTTP 400 `INVALID_ROOM_COUNT`.
      - Jika $num\_guests < num\_rooms$ $\rightarrow$ HTTP 400 `INVALID_GUEST_COUNT`.
      - Jika $num\_guests > v.MaxCapacity \times num\_rooms$ $\rightarrow$ HTTP 400 `EXCEEDS_CAPACITY`.
      - Jika payload menyertakan breakdown $adults$ dan $children$:
        - $adults < num\_rooms \rightarrow$ HTTP 400 `INVALID_GUEST_COUNT`.
        - $adults > v.MaxAdults \times num\_rooms \rightarrow$ HTTP 400 `EXCEEDS_CAPACITY`.
        - $children > v.MaxChildren \times num\_rooms \rightarrow$ HTTP 400 `EXCEEDS_CAPACITY`.
        - $child\_ages$ wajib divalidasi identik dengan aturan FR-01.

- **FR-04 (Booking Creation Invariant & Guard):**
  - Domain `booking.Service.Create` dan handler `POST /api/v1/bookings` memvalidasi:
    - $num\_guests < num\_rooms \rightarrow$ HTTP 400 `INVALID_GUEST_COUNT` (atau `INVALID_CAPACITY`).
    - Membaca data varian dari `CatalogReader`:
      - Jika $num\_guests > variant.MaxCapacity \times num\_rooms \rightarrow$ mengembalikan domain error `booking.ErrExceedsCapacity` yang dipetakan ke HTTP 400 `EXCEEDS_CAPACITY`.

---

## 3. Spesifikasi Antarmuka HTTP & Kontrak JSON

### 3.1 `GET /api/v1/search`
* Query Params:
  * `check_in`: YYYY-MM-DD (wajib)
  * `check_out`: YYYY-MM-DD (wajib)
  * `rooms`: int (default 1, range 1-8)
  * `adults`: int (default 1, $\ge rooms$)
  * `children`: int (default 0)
  * `child_ages`: comma-separated integers (misal: `5,8`)

* Respons Error Contoh (400 Bad Request):
```json
{
  "error": "child_ages count (2) must match children count (1)",
  "code": "CHILD_AGE_COUNT_MISMATCH"
}
```

```json
{
  "error": "adults count must be at least equal to rooms count",
  "code": "INVALID_GUEST_COUNT"
}
```

### 3.2 `POST /api/v1/quotes`
* Request Body JSON:
```json
{
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "rate_plan_code": "room_only",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "num_rooms": 1,
  "num_guests": 5
}
```

* Respons Error Overcapacity (400 Bad Request):
```json
{
  "error": "jumlah tamu (5) melebihi kapasitas maksimum varian kamar (3)",
  "code": "EXCEEDS_CAPACITY"
}
```

### 3.3 `POST /api/v1/bookings`
* Request Body JSON (Direct call / bypass attempt):
```json
{
  "quote_id": "qt_mocked_or_bypassed",
  "terms_accepted": true,
  "privacy_accepted": true,
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "num_rooms": 1,
  "num_guests": 6,
  "guest_name": "Bypass Attacker",
  "guest_email": "attacker@example.com"
}
```

* Respons Error Overcapacity (400 Bad Request):
```json
{
  "error": "booking: number of guests exceeds room maximum capacity",
  "code": "EXCEEDS_CAPACITY"
}
```
