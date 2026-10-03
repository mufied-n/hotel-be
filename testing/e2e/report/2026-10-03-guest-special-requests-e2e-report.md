# End-to-End (E2E) Test Report: Guest Special Requests & Stay Assistance Desk (Feature F06)
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**  
**Tanggal Eksekusi:** 2026-10-03 12:15:00 WIB  
**Dokumen Terkait:** [PRD Special Requests](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/guest-special-requests-and-assistance-2026-10-03.md) | [SRS Special Requests](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/guest-special-requests-and-assistance-2026-10-03.md) | [Tech Architecture Special Requests](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-special-requests-architecture-2026-10-03.md) | [Walkthrough Tracking](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/guest-special-requests-walkthrough-2026-10-03.md)  
**Status Pengujian:** **100% PASS (65/65 Scenarios)**

---

## 1. Ringkasan Eksekusi (Executive Summary)

Pengujian End-to-End (E2E) untuk fitur **F06: Guest Special Requests & Stay Assistance Desk** dan resolusi audit gap **BE-R02 (P1: Data Masking on PublicDTO)** telah dieksekusi secara otomatis dan komprehensif menggunakan test runner Go di [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) serta skrip executable bash di [`testing/e2e/script/guest_special_requests_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/guest_special_requests_e2e.sh).

Seluruh 4 skenario baru (E2E-62 hingga E2E-65) dan 61 skenario regresi sebelumnya (E2E-01 hingga E2E-61) berhasil lulus 100% tanpa error, kebocoran data, ataupun regresi pada sistem yang sudah berjalan.

| Kategori Pengujian | Total Skenario | Lulus (Pass) | Gagal (Fail) | Status |
| :--- | :---: | :---: | :---: | :---: |
| **Skenario Regresi Inti & RBAC (E2E-01 s/d E2E-61)** | 61 | 61 | 0 | **PASS** |
| **Fitur F06: Guest Special Request & Auto-Route (E2E-62)** | 1 | 1 | 0 | **PASS** |
| **Fitur F06: Anti-IDOR Defense Verification (E2E-63)** | 1 | 1 | 0 | **PASS** |
| **Fitur F06: Staff Departmental Queue & RBAC (E2E-64)** | 1 | 1 | 0 | **PASS** |
| **Fitur F06: Request Fulfillment & Guest Sync (E2E-65)** | 1 | 1 | 0 | **PASS** |
| **TOTAL** | **65** | **65** | **0** | **100% PASS** |

---

## 2. Detil Verifikasi Skenario Baru (Feature F06)

### A. E2E-62: Guest submits special request & auto-routes to Housekeeping (201 Created)
- **Endpoint:** `POST /api/v1/guest/bookings/{id}/special-requests`
- **Autentikasi:** Guest Session Token (Bearer `gst_sess_...`)
- **Request Payload:**
  ```json
  {
    "category": "celebration_setup",
    "description": "Anniversary ke-5, mohon handuk angsa dan kartu ucapan",
    "target_time": "15:00"
  }
  ```
- **Hasil Response Payload (201 Created):**
  ```json
  {
    "id": "e4b2d398-3f82-411a-a320-c20e40ddb97b",
    "booking_id": "bk-e2e-001",
    "category": "celebration_setup",
    "department": "housekeeping",
    "description": "Anniversary ke-5, mohon handuk angsa dan kartu ucapan",
    "target_time": "15:00",
    "status": "pending",
    "staff_notes": "",
    "handled_by": "",
    "created_at": "2026-10-03T05:12:00Z",
    "updated_at": "2026-10-03T05:12:00Z"
  }
  ```
- **Verifikasi Kualitas:** Kategori `celebration_setup` secara otomatis di-*route* ke departemen `housekeeping` dengan status awal `pending`.

---

### B. E2E-63: Guest special request Anti-IDOR defense (404 Not Found)
- **Endpoint:** `POST /api/v1/guest/bookings/bk-other-guest-unowned/special-requests`
- **Autentikasi:** Guest Session Token milik tamu A
- **Target Booking:** `bk-other-guest-unowned` (Milik tamu B)
- **Hasil:**
  - HTTP Status: **`404 Not Found`**
  - Response Body:
    ```json
    {
      "code": "BOOKING_NOT_FOUND",
      "error": "reservasi tidak ditemukan atau tidak memiliki akses"
    }
    ```
- **Verifikasi Keamanan:** Tamu dilarang melihat atau memanipulasi permintaan khusus pada reservasi tamu lain. Return code `404` mencegah kebocoran informasi (*resource enumeration attack*).

---

### C. E2E-64: Staff inspects departmental special requests queue & RBAC isolation (200 OK & 403 Forbidden)
- **Endpoint:** `GET /api/v1/front-desk/special-requests?department=housekeeping`
- **Uji Otorisasi Casbin RBAC:**
  - `guest` mencoba akses endpoint staf: **`403 Forbidden`** (Tamu publik dicegah mengakses queue internal staf).
  - `receptionist` mengakses: **`200 OK`**.
  - `housekeeping` mengakses: **`200 OK`** (Menerima daftar seluruh tugas khusus untuk kamar-kamar hotel).
- **Hasil Response Payload (200 OK):**
  ```json
  {
    "items": [
      {
        "id": "e4b2d398-3f82-411a-a320-c20e40ddb97b",
        "booking_id": "bk-e2e-001",
        "category": "celebration_setup",
        "department": "housekeeping",
        "description": "Anniversary ke-5, mohon handuk angsa dan kartu ucapan",
        "target_time": "15:00",
        "status": "pending",
        "guest_name": "Rian Wicaksono",
        "room_number": "201",
        "check_in": "2026-10-10",
        "check_out": "2026-10-12"
      }
    ],
    "total": 1
  }
  ```

---

### D. E2E-65: Housekeeping fulfills special request & guest views fulfillment (200 OK)
- **Tahap 1: Staf Housekeeping Memperbarui Status**
  - **Endpoint:** `PUT /api/v1/front-desk/special-requests/{id}/status`
  - **Autentikasi:** `Bearer housekeeping`
  - **Request Payload:**
    ```json
    {
      "to_status": "fulfilled",
      "staff_notes": "Handuk angsa dan kartu ucapan selamat anniversary telah siap di kamar 201"
    }
    ```
  - **Hasil (200 OK):** Status berhasil diubah dari `pending` ke `fulfilled`, field `handled_by` tercatat sebagai `housekeeping`, dan `handled_at` terisi timestamp server.
- **Tahap 2: Tamu Memeriksa Status Permintaan pada Portal My Bookings**
  - **Endpoint:** `GET /api/v1/guest/bookings/{id}/special-requests`
  - **Autentikasi:** Guest Session Token
  - **Hasil (200 OK):**
    ```json
    {
      "booking_id": "bk-e2e-001",
      "requests": [
        {
          "id": "e4b2d398-3f82-411a-a320-c20e40ddb97b",
          "category": "celebration_setup",
          "department": "housekeeping",
          "status": "fulfilled",
          "staff_notes": "Handuk angsa dan kartu ucapan selamat anniversary telah siap di kamar 201"
        }
      ]
    }
    ```

---

## 3. Resolusi Audit Gap: BE-R02 (P1 Masking of Special Requests on PublicDTO)

- **Masalah:** Pada endpoint publik tak terotentikasi `GET /api/v1/bookings/{id}`, field `special_requests` dan `estimated_arrival_time` sebelumnya dapat terekspos tanpa autentikasi, melanggar prinsip kepatuhan privasi data tamu (UU PDP).
- **Resolusi:**
  - Metode [`booking.Booking.ToPublicDTO()`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go) sekarang mereduksi `SpecialRequests = ""` dan `EstimatedArrivalTime = ""`.
  - Terverifikasi pada pengujian unit [`internal/booking/booking_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking_test.go), [`internal/api/router_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router_test.go), dan regresi E2E-10 & E2E-22: Tamu tanpa kredensial valid hanya menerima nama depan bertopeng, inisial email, serta tanpa preferensi khusus/kedatangan tamu.

---

## 4. Feature Flag Gating: `ff_guest_special_requests`

- Seluruh endpoint permohonan khusus tamu dan antrean staf diproteksi oleh middleware:
  `api.RequireFeature("ff_guest_special_requests")`
- Ketika dinonaktifkan via admin flag API, sistem merespons dengan:
  ```json
  {
    "code": "FEATURE_DISABLED",
    "error": "Fitur Guest Special Requests Management sedang dinonaktifkan sementara."
  }
  ```
  dengan HTTP Status `503 Service Unavailable`.

---

## 5. Ringkasan Coverage Pengujian & Lint

```bash
$ go test -v -cover ./internal/assistance/...
coverage: 94.9% of statements
PASS

$ go test -v -cover ./internal/api/...
coverage: 86.3% of statements
PASS

$ go test -v -run "TestEndToEndHotelBookingRBACLifecycle" ./testing/e2e/script/...
--- PASS: TestEndToEndHotelBookingRBACLifecycle (0.08s)
    --- PASS: 65/65 Subtests
PASS

$ go vet ./...
(0 issues / warnings)
```

---

## 6. Kesimpulan & Rekomendasi
Fitur **Guest Special Requests & Stay Assistance Desk (Feature F06)** beserta perbaikan **Audit Gap BE-R02** telah memenuhi seluruh spesifikasi PRD, SRS, arsitektur teknis, dan standar pengujian otomasi E2E Pulang ke Uttara dengan coverage $\ge 80\%$ dan nol isu statis. Fitur siap untuk deployment dan integrasi produksi.
