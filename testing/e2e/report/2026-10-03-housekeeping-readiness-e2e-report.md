# End-to-End (E2E) Test Report: Housekeeping Room Status & Readiness Lifecycle (Proposed 01)
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**  
**Tanggal Eksekusi:** 2026-10-03 11:15:07 WIB  
**Dokumen Terkait:** [PRD Housekeeping](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/housekeeping-room-status-and-readiness-2026-10-03.md) | [SRS Housekeeping](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/housekeeping-room-status-and-readiness-2026-10-03.md) | [Tech Architecture Housekeeping](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/housekeeping-room-status-architecture-2026-10-03.md) | [Walkthrough Tracking](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/housekeeping-room-status-walkthrough-2026-10-03.md)  
**Status Pengujian:** **100% PASS (51/51 Scenarios)**

---

## 1. Ringkasan Eksekusi (Executive Summary)

Pengujian End-to-End (E2E) untuk modul **Proposed 01: Housekeeping Room Status & Readiness Lifecycle** telah dieksekusi secara otomatis menggunakan runner Go di [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) serta skrip executable bash di [`testing/e2e/script/housekeeping_room_readiness_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/housekeeping_room_readiness_e2e.sh).

Seluruh skenario pengujian baru (E2E-46 hingga E2E-51) dan skenario regresi sebelumnya (E2E-01 hingga E2E-45) berhasil lulus 100% tanpa adanya error ataupun regresi.

| Kategori Pengujian | Total Skenario | Lulus (Pass) | Gagal (Fail) | Status |
| :--- | :---: | :---: | :---: | :---: |
| **Skenario Regresi (E2E-01 s/d E2E-45)** | 45 | 45 | 0 | **PASS** |
| **Fitur HK: Room Board Query & Floor Filter (E2E-46)** | 1 | 1 | 0 | **PASS** |
| **Fitur HK: Cleanliness Lifecycle & Anti-Bypass (E2E-47)** | 1 | 1 | 0 | **PASS** |
| **Fitur HK: Check-In Guard Rejection on Dirty Room (E2E-48)** | 1 | 1 | 0 | **PASS** |
| **Fitur HK: Check-In on Inspected Room & Auto-Occupied (E2E-49)** | 1 | 1 | 0 | **PASS** |
| **Fitur HK: Check-Out Auto-Dirty Transition (E2E-50)** | 1 | 1 | 0 | **PASS** |
| **Fitur HK: GM Admin Out-of-Order Isolation (E2E-51)** | 1 | 1 | 0 | **PASS** |
| **TOTAL** | **51** | **51** | **0** | **100% PASS** |

---

## 2. Detil Verifikasi Skenario Baru (Housekeeping Readiness)

### A. E2E-46: Housekeeping Room Board Query & Floor Filter (FR-HK-01)
- **Endpoint:** `GET /api/v1/housekeeping/rooms?floor=2`
- **Uji Otorisasi RBAC:**
  - `guest` mencoba akses: **`403 Forbidden`** (Tamu publik diblokir dari operasional internal hotel).
  - `receptionist` mengakses: **`200 OK`** (Staf front desk dapat memantau kesiapan kamar).
  - `housekeeping` mengakses: **`200 OK`**.
- **Hasil Response Payload:**
  ```json
  {
    "total_rooms": 5,
    "summary": {
      "vacant_dirty": 5,
      "cleaning": 0,
      "vacant_clean": 0,
      "inspected": 0,
      "occupied": 0,
      "out_of_service": 0,
      "out_of_order": 0
    },
    "rooms": [
      {
        "room_number": "201",
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "room_type_name": "Superior King",
        "floor": 2,
        "cleanliness_status": "vacant_dirty"
      }
    ]
  }
  ```

---

### B. E2E-47: Cleanliness Lifecycle Transition & Anti-Bypass Guard (FR-HK-02)
- **Alur Transisi Sah (Legal):**
  1. Room Attendant memulai pembersihan kamar 202:
     - `PUT /api/v1/housekeeping/rooms/202/status`
     - Payload: `{"to_status":"cleaning","notes":"Attendant Ahmad started cleaning"}`
     - Status: **`200 OK`** (status berubah menjadi `cleaning`).
  2. Room Attendant menyelesaikan pembersihan:
     - `PUT /api/v1/housekeeping/rooms/202/status`
     - Payload: `{"to_status":"vacant_clean","notes":"Linen replaced, amenities stocked"}`
     - Status: **`200 OK`** (status berubah menjadi `vacant_clean`).
  3. HK Supervisor melakukan QC dan inspeksi:
     - `PUT /api/v1/housekeeping/rooms/202/status`
     - Payload: `{"to_status":"inspected","notes":"QC inspection passed"}`
     - Status: **`200 OK`** (status berubah menjadi `inspected`, kamar siap huni).
- **Uji Guard Anti-Bypass:**
  - Kamar 204 masih berstatus `vacant_dirty`.
  - Upaya langsung mengubah menjadi `inspected` tanpa pembersihan (`dirty` ➔ `inspected`):
    - Payload: `{"to_status":"inspected","notes":"Direct bypass attempt"}`
    - Status: **`409 Conflict`**
    - Error Response: `{"error":"invalid status transition: dari vacant_dirty ke inspected","code":"INVALID_STATUS_TRANSITION"}`.

---

### C. E2E-48: Front Desk Check-In Guard Rejection on Dirty Room (FR-HK-04)
- **Skenario:** Kamar fisik 301 disetel dalam status `vacant_dirty`. Tamu dengan booking confirmed tiba di hotel.
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/check-in`
- **Hasil:**
  - Status HTTP: **`409 Conflict`**
  - Error Response:
    ```json
    {
      "error": "kamar belum siap huni (belum diinspeksi oleh housekeeping)",
      "code": "ROOM_NOT_READY"
    }
    ```
  - **Kesimpulan:** Front desk dilarang secara sistemik menyerahkan kunci kamar yang belum berstatus `inspected`.

---

### D. E2E-49: Front Desk Check-In on Inspected Room & Auto-Occupied Transition (FR-HK-04)
- **Skenario:** HK Supervisor menyelesaikan inspeksi kamar 301 (`vacant_clean` ➔ `inspected`). Resepsionis melakukan check-in ulang.
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/check-in`
- **Hasil:**
  - Status HTTP: **`200 OK`**
  - Response:
    ```json
    {
      "id": "bk-e2e-001",
      "room_numbers": ["301"]
    }
    ```
  - **Efek Samping Otomatis:** Status kamar fisik 301 pada dashboard operasional otomatis bertransisi menjadi **`occupied`**.

---

### E. E2E-50: Front Desk Check-Out & Auto-Dirty Transition (FR-HK-05)
- **Skenario:** Tamu menyelesaikan masa menginap dan melakukan check-out di front desk.
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/check-out`
- **Hasil:**
  - Status HTTP: **`200 OK`**
  - Response: `{"status":"checked_out","id":"bk-e2e-001"}`
  - **Efek Samping Otomatis:** Status kebersihan kamar 301 otomatis bertransisi menjadi **`vacant_dirty`**, memicu tugas pembersihan baru bagi tim Housekeeping.

---

### F. E2E-51: GM Admin Out-of-Order (OOO) Isolation & Non-GM Rejection (FR-HK-03)
- **Endpoint:** `POST /api/v1/housekeeping/rooms/203/out-of-order`
- **Uji Otorisasi RBAC:**
  - Resepsionis mencoba menetapkan status OOO: **`403 Forbidden`**.
  - Housekeeping biasa mencoba menetapkan status OOO: **`403 Forbidden`**.
  - General Manager (`gm_admin`) mengeksekusi OOO:
    - Payload:
      ```json
      {
        "start_date": "2026-10-10",
        "end_date": "2026-10-15",
        "reason": "Major bathroom plumbing overhaul"
      }
      ```
    - Status HTTP: **`200 OK`**
    - Response:
      ```json
      {
        "status": "ok",
        "room_number": "203",
        "cleanliness_status": "out_of_order",
        "inventory_deducted_dates": ["2026-10-10","2026-10-11","2026-10-12","2026-10-13","2026-10-14"]
      }
      ```

---

## 3. Hasil Pengujian Unit & Vet Code Quality

- **Test Suite Internal Housekeeping:**
  - Coverage: **80.3% of statements**
  - Pola Test: Table-Driven Tests (`tests := []struct{...}`)
- **Lint / Vet Status:**
  ```bash
  go vet ./...
  # Output: clean, 0 warnings, 0 errors
  ```
- **Test Suite Global:**
  ```bash
  go test ./...
  # Output: ok (seluruh packages pass 100%)
  ```

---

## 4. Kesimpulan & Status Akhir

Fitur **Housekeeping Room Status & Readiness Lifecycle (Proposed 01)** telah tervalidasi secara menyeluruh, memenuhi standar industri bintang 4 (AHLA / Cloudbeds dual-stage verification), aman terhadap bypass kotoran kamar, dan terintegrasi mulus dengan modul reservasi, front desk check-in/out, dan otorisasi RBAC Casbin.
