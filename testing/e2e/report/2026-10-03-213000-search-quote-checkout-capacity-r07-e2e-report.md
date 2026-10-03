# Laporan Pengujian End-to-End (E2E): Invariant Kapasitas Kamar Search, Quote & Checkout (BE-R07)

**Nomor Laporan:** E2E-REP-PULANG-BE-R07  
**Tanggal Pengujian:** 3 Oktober 2026, 21:32 WIB  
**Target Fitur:** `BE-R07` (Kapasitas search belum menjadi invariant checkout)  
**Terkait:** BE-G03, F01, F08  
**Lingkungan Uji:** PostgreSQL 18 (`r07-pg`), Valkey 8 (`r07-vk`), Go 1.24 HTTP Server (:28080)  
**Skrip Automasi:** [`testing/e2e/script/search_quote_checkout_capacity_r07_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/search_quote_checkout_capacity_r07_e2e.sh)  
**Hasil Eksekusi:** **35/35 Assertions PASSED (100% Lulus)**  

---

## 1. Ringkasan Eksekusi Pengujian

Pengujian end-to-end ini memverifikasi penegakan invariant kapasitas fisik varian kamar katalog hotel Pulang ke Uttara (Yogyakarta) secara ketat dan konsisten di seluruh alur reservasi:
1. Validasi fail-fast query parameters pada `GET /api/v1/search`.
2. Evaluasi status ketersediaan varian kamar pada Search terhadap batas okupansi (`max_capacity`, `max_adults`, `max_children`).
3. Penjagaan kapasitas fisik kamar pada endpoint kuotasi harga `POST /api/v1/quotes` terhadap katalog `CatalogStore`.
4. Penjagaan direct API bypass overcapacity pada endpoint pembuatan booking `POST /api/v1/bookings` dan `booking.Service.Create`.
5. Siklus lengkap pemesanan boundary valid dari pencarian, kuotasi harga, pemesanan pending, hingga konfirmasi pembayaran.

```
=================================================================
  Total Assertions : 35
  Passed           : 35
  Failed           : 0
  ALL E2E ASSERTIONS PASSED!
=================================================================
```

---

## 2. Rincian Skenario & Bukti Respons HTTP

### 2.1 Validasi Parameter Search (FR-01)
* **Skenario 1A: Dewasa kurang dari jumlah kamar (`adults=1&rooms=2`)**
  * HTTP Request: `GET /api/v1/search?check_in=2026-10-13&check_out=2026-10-15&adults=1&rooms=2`
  * Respons Status: `400 Bad Request`
  * Error Code: `INVALID_GUEST_COUNT`
  * Hasil: **PASS**

* **Skenario 1B: `children=0` namun menyertakan `child_ages=5`**
  * HTTP Request: `GET /api/v1/search?check_in=2026-10-13&check_out=2026-10-15&adults=2&children=0&child_ages=5`
  * Respons Status: `400 Bad Request`
  * Error Code: `CHILD_AGE_COUNT_MISMATCH`
  * Hasil: **PASS**

* **Skenario 1C: Jumlah `child_ages` tidak sama dengan `children` (`children=2, child_ages=5`)**
  * HTTP Request: `GET /api/v1/search?check_in=2026-10-13&check_out=2026-10-15&adults=2&children=2&child_ages=5`
  * Respons Status: `400 Bad Request`
  * Error Code: `CHILD_AGE_COUNT_MISMATCH`
  * Hasil: **PASS**

* **Skenario 1D: Umur anak di luar batas legal $0 \le age \le 17$ (`child_ages=18`)**
  * HTTP Request: `GET /api/v1/search?check_in=2026-10-13&check_out=2026-10-15&adults=2&children=1&child_ages=18`
  * Respons Status: `400 Bad Request`
  * Error Code: `INVALID_CHILD_AGE`
  * Hasil: **PASS**

* **Skenario 1E: Query search valid dengan kombinasi legal**
  * HTTP Request: `GET /api/v1/search?check_in=2026-10-13&check_out=2026-10-15&adults=2&children=1&child_ages=6&rooms=1`
  * Respons Status: `200 OK`
  * Hasil: **PASS**

---

### 2.2 Evaluasi Okupansi Varian Kamar pada Search (FR-02)
* **Skenario 2A: 3 Dewasa di 1 Kamar Superior King (MaxAdults=2)**
  * Query: `adults=3&rooms=1`
  * Hasil: `room_variant.code == "sup-king"` mendapatkan `available = false` dan `unavailable_reason = "EXCEEDS_CAPACITY"`.
  * Hasil: **PASS**

* **Skenario 2B: 2 Anak di 1 Kamar Superior King (MaxChildren=1)**
  * Query: `adults=1&children=2&child_ages=4,8&rooms=1`
  * Hasil: `room_variant.code == "sup-king"` mendapatkan `available = false` dan `unavailable_reason = "EXCEEDS_CAPACITY"`.
  * Hasil: **PASS**

---

### 2.3 Penjagaan Kapasitas Quote terhadap Katalog (FR-03)
* **Skenario 3A: Kuotasi 5 Tamu di 1 Kamar Superior King (MaxCapacity=3)**
  * Payload: `{"room_type_id": "01900000-0000-7000-8000-000000000001", "num_rooms": 1, "num_guests": 5}`
  * Respons: `400 Bad Request` dengan code `EXCEEDS_CAPACITY`.
  * Hasil: **PASS**

* **Skenario 3B: Kuotasi 2 Kamar, 1 Tamu (`num_guests < num_rooms`)**
  * Payload: `{"num_rooms": 2, "num_guests": 1}`
  * Respons: `400 Bad Request` dengan code `INVALID_GUEST_COUNT`.
  * Hasil: **PASS**

* **Skenario 3C: Kuotasi 3 Dewasa di 1 Kamar Superior King (`adults > MaxAdults`)**
  * Payload: `{"num_rooms": 1, "adults": 3, "children": 0}`
  * Respons: `400 Bad Request` dengan code `EXCEEDS_CAPACITY`.
  * Hasil: **PASS**

* **Skenario 3D: Kuotasi 2 Anak di 1 Kamar Superior King (`children > MaxChildren`)**
  * Payload: `{"num_rooms": 1, "adults": 1, "children": 2, "child_ages": [4, 7]}`
  * Respons: `400 Bad Request` dengan code `EXCEEDS_CAPACITY`.
  * Hasil: **PASS**

* **Skenario 3E: Kuotasi dengan ID Tipe Kamar Tidak Dikenal**
  * Respons: `404 Not Found` dengan code `ROOM_NOT_FOUND`.
  * Hasil: **PASS**

* **Skenario 3F: Kuotasi Boundary Valid (1 Kamar, 3 Tamu: 2 Dewasa, 1 Anak)**
  * Respons: `200 OK`, menghasilkan Locked Quote valid berformat UUID.
  * Hasil: **PASS**

---

### 2.4 Penjagaan Direct Checkout / Bypass Search Overcapacity (FR-04)
* **Skenario 4A: Direct Create Booking Overcapacity (6 Tamu di 1 Kamar)**
  * Endpoint: `POST /api/v1/bookings`
  * Respons: `400 Bad Request` dengan code `EXCEEDS_CAPACITY`.
  * Verifikasi: Ditolak di lapisan transport dan domain sebelum hold/payment dibuat.
  * Hasil: **PASS**

* **Skenario 4B: Direct Create Booking dengan `num_guests < num_rooms`**
  * Respons: `400 Bad Request` dengan code `INVALID_GUEST_COUNT`.
  * Hasil: **PASS**

* **Skenario 4C: Direct Create Booking dengan Tipe Kamar Tidak Ada di Katalog**
  * Respons: `404 Not Found` dengan code `ROOM_NOT_FOUND`.
  * Hasil: **PASS**

---

### 2.5 Siklus Lengkap Reservasi Boundary Valid (FR-05)
* Menggunakan Locked Quote valid dari Skenario 3F:
  1. `POST /api/v1/bookings` berhasil dengan status `201 Created`, booking status `pending`.
  2. `POST /fake-pay/ref-success?booking_id={id}` memproses simulasi konfirmasi pembayaran secara aman (`200 OK`).
  3. `GET /api/v1/bookings/{id}` memverifikasi status reservasi terkonfirmasi (`confirmed`).
  * Hasil: **PASS**

---

## 3. Kesimpulan Verifikasi
Fitur penegakan invariant kapasitas fisik kamar katalog hotel Pulang ke Uttara (`BE-R07`) telah terverifikasi secara tuntas pada pengujian unit, tabel test, dan pengujian E2E integrasi penuh dengan zero regression.
