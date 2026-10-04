# Software Requirements Specification (SRS)
# Dynamic Rates, Room Allotment & Stop-Sell Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `SRS-F08-F09-RATES-ALLOTMENT-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - PRD: [`docs/prd/dynamic-rates-and-stop-sell-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/dynamic-rates-and-stop-sell-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/dynamic-rates-and-stop-sell-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/dynamic-rates-and-stop-sell-architecture-2026-10-04.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

* **FR-RATE-01 (Calendar Matrix Query):** Sistem wajib menyediakan endpoint bagi peran berwenang (`revenue_mgr`, `gm_admin`, `receptionist`) untuk mengambil kalender tarif, status stop-sell, CTA/CTD, dan batas durasi menginap per kamar dalam rentang tanggal tertentu (maksimal 90 hari per request).
* **FR-RATE-02 (Bulk Override Management):** Sistem wajib menyediakan endpoint transaksi atomik untuk mengubah tarif, status stop-sell, CTA, CTD, dan MLOS untuk beberapa tipe kamar dan rentang tanggal sekaligus.
* **FR-RATE-03 (Stop-Sell Search Enforcement):** Endpoint publik `/api/v1/search` wajib mengevaluasi ketersediaan kalender; jika ada satu tanggal dalam rentang menginap yang berstatus `is_stop_sell = true`, kamar tersebut ditandai tidak tersedia untuk tanggal tersebut.
* **FR-RATE-04 (Restriction Enforcement on Quotes):** Endpoint `/api/v1/quotes` wajib menolak pembuatan quote jika tanggal kedatangan berstatus CTA (`is_cta = true`), tanggal keberangkatan berstatus CTD (`is_ctd = true`), atau durasi inap melanggar `min_los` / `max_los`.
* **FR-RATE-05 (Dynamic Nightly Breakdown):** Kalkulasi quote wajib menghitung harga per malam secara persis mengikuti kalender override (atau fallback ke base price katalog jika tidak ada override).
* **FR-RATE-06 (Dynamic Promo Campaigns):** Sistem wajib menyediakan endpoint CRUD untuk mengelola kampanye promo dengan validasi kuota penggunaan atomik, batas minimum malam menginap, serta tanggal kedaluwarsa.
* **FR-RATE-07 (Cache Invalidation Event):** Setiap kali terjadi mutasi kalender tarif atau promo, sistem wajib secara asinkron atau langsung menghapus cache terkait di Valkey (`rates.ValkeyQuoteStore`) agar data pencarian selalu segar.

---

## 2. Spesifikasi Antarmuka HTTP (RESTful Contracts)

Semua endpoint dilindungi middleware standar: Request ID, CORS, Access Log, Body Size Limit (1MB), dan Casbin RBAC Authorization. Format error mengikuti **RFC 7807 Problem Details dual-shape format**.

---

### 2.1 GET `/api/v1/revenue/calendar`
Mengambil data kalender tarif dan pembatasan operasional harian.

* **Headers:**
  - `Authorization: Bearer stf_<token>` (Role: `revenue_mgr`, `gm_admin`, `receptionist`)
* **Query Parameters:**
  - `start_date` (string, format `YYYY-MM-DD`, required)
  - `end_date` (string, format `YYYY-MM-DD`, required, selisih maksimal 90 hari)
  - `room_type_id` (UUID, optional, jika ingin memfilter satu kamar saja)
* **Response `200 OK`:**
  ```json
  {
    "start_date": "2026-12-24",
    "end_date": "2026-12-26",
    "items": [
      {
        "date": "2026-12-24",
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "room_type_name": "Deluxe King Bay Window",
        "rate_plan_code": "RO",
        "base_price_idr": 1131500,
        "price_override_idr": 1500000,
        "effective_price_idr": 1500000,
        "is_stop_sell": false,
        "is_cta": false,
        "is_ctd": false,
        "min_los": 1,
        "max_los": 30,
        "physical_stock": 20,
        "allotment_limit": 15
      },
      {
        "date": "2026-12-25",
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "room_type_name": "Deluxe King Bay Window",
        "rate_plan_code": "RO",
        "base_price_idr": 1131500,
        "price_override_idr": 1750000,
        "effective_price_idr": 1750000,
        "is_stop_sell": true,
        "is_cta": true,
        "is_ctd": false,
        "min_los": 2,
        "max_los": 30,
        "physical_stock": 20,
        "allotment_limit": null
      }
    ]
  }
  ```

---

### 2.2 PUT `/api/v1/revenue/calendar/bulk`
Memperbarui tarif dan aturan pembatasan secara massal (*bulk upsert*).

* **Headers:**
  - `Authorization: Bearer stf_<token>` (Role: `revenue_mgr`, `gm_admin`)
  - `Content-Type: application/json`
* **Request Body:**
  ```json
  {
    "room_type_ids": [
      "01900000-0000-7000-8000-000000000001",
      "01900000-0000-7000-8000-000000000002"
    ],
    "start_date": "2026-12-30",
    "end_date": "2027-01-02",
    "rate_plan_code": "RO",
    "price_override_idr": 2200000,
    "is_stop_sell": false,
    "is_cta": true,
    "is_ctd": false,
    "min_los": 2,
    "max_los": 30,
    "allotment_limit": null
  }
  ```
* **Response `200 OK`:**
  ```json
  {
    "status": "success",
    "message": "bulk calendar updated successfully",
    "affected_records": 8,
    "cache_invalidated": true
  }
  ```

---

### 2.3 POST `/api/v1/revenue/promos`
Membuat kampanye kode promo baru.

* **Headers:**
  - `Authorization: Bearer stf_<token>` (Role: `revenue_mgr`, `gm_admin`)
  - `Content-Type: application/json`
* **Request Body:**
  ```json
  {
    "code": "JOGJASERU",
    "name": "Promo Liburan Akhir Pekan Yogyakarta",
    "discount_type": "PERCENT",
    "discount_value": 20,
    "max_discount_idr": 500000,
    "min_stay_nights": 2,
    "quota_total": 50,
    "valid_from": "2026-10-01T00:00:00+07:00",
    "valid_to": "2026-12-31T23:59:59+07:00",
    "applicable_room_types": null,
    "is_active": true
  }
  ```
* **Response `201 Created`:**
  ```json
  {
    "id": "a90f11ee-4822-4a0b-b152-78d1033230a1",
    "code": "JOGJASERU",
    "discount_type": "PERCENT",
    "discount_value": 20,
    "max_discount_idr": 500000,
    "min_stay_nights": 2,
    "quota_total": 50,
    "quota_used": 0,
    "is_active": true
  }
  ```

---

### 2.4 Error Codes & RFC 7807 Schema

Jika terjadi pelanggaran validasi atau restriksi, server mengembalikan format error dual-shape yang konsisten:

| HTTP Status | Error Code (`code`) | Deskripsi Masalah |
| :---: | :--- | :--- |
| `400` | `ROOM_STOP_SELL` | Kamar yang dipilih telah ditutup penjualannya untuk tanggal yang diminta. |
| `400` | `CLOSED_TO_ARRIVAL` | Tanggal kedatangan yang dipilih tidak mengizinkan check-in baru. |
| `400` | `CLOSED_TO_DEPARTURE` | Tanggal keberangkatan yang dipilih tidak mengizinkan check-out. |
| `400` | `MIN_LENGTH_OF_STAY_VIOLATED` | Durasi menginap kurang dari syarat minimum yang ditentukan hotel. |
| `400` | `PROMO_QUOTA_EXHAUSTED` | Kuota promo telah habis digunakan tamu lain. |
| `400` | `PROMO_EXPIRED` | Masa berlaku kode promo telah lewat atau belum dimulai. |
| `400` | `PROMO_MIN_STAY_NOT_MET` | Syarat minimal malam menginap untuk kode promo ini belum terpenuhi. |
| `403` | `FORBIDDEN` | Pengguna tidak memiliki hak akses role Casbin (`revenue_mgr` / `gm_admin`). |
| `404` | `PROMO_NOT_FOUND` | Kode promo tidak terdaftar di sistem. |

Contoh Respon Error `400 Bad Request`:
```json
{
  "type": "https://httpstatuses.com/400",
  "title": "Bad Request",
  "status": 400,
  "detail": "room type is closed for sale on 2026-12-31 due to stop-sell restriction",
  "error": "room type is closed for sale on 2026-12-31 due to stop-sell restriction",
  "code": "ROOM_STOP_SELL",
  "request_id": "8f302b1f8c1248a3a0e632b7194f4832"
}
```
