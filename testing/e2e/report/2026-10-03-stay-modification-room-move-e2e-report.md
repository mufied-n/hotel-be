# End-to-End (E2E) Test Report: Stay Modification, Room Move & Stay Extension (Proposed 03)
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**  
**Tanggal Eksekusi:** 2026-10-03 11:36:00 WIB  
**Dokumen Terkait:** [PRD Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/stay-modification-room-move-and-extension-2026-10-03.md) | [SRS Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/stay-modification-room-move-and-extension-2026-10-03.md) | [Tech Architecture Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/stay-modification-room-move-architecture-2026-10-03.md) | [Walkthrough Tracking](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/stay-modification-room-move-walkthrough-2026-10-03.md)  
**Status Pengujian:** **100% PASS (60/60 Scenarios)**

---

## 1. Ringkasan Eksekusi (Executive Summary)

Pengujian End-to-End (E2E) untuk modul **Proposed 03: Stay Modification, Room Move & Stay Extension** telah dieksekusi secara otomatis menggunakan test runner Go di [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) serta skrip executable bash di [`testing/e2e/script/stay_modification_room_move_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/stay_modification_room_move_e2e.sh).

Seluruh 5 skenario baru (E2E-56 hingga E2E-60) dan 55 skenario regresi sebelumnya (E2E-01 hingga E2E-55) berhasil lulus 100% tanpa error, pelanggaran constraint PostgreSQL GiST, ataupun kebocoran data.

| Kategori Pengujian | Total Skenario | Lulus (Pass) | Gagal (Fail) | Status |
| :--- | :---: | :---: | :---: | :---: |
| **Skenario Regresi (E2E-01 s/d E2E-55)** | 55 | 55 | 0 | **PASS** |
| **Fitur STAY: Mid-Stay Room Move & Auto-Dirty (E2E-56)** | 1 | 1 | 0 | **PASS** |
| **Fitur STAY: Readiness Guard on Dirty Target Room (E2E-57)** | 1 | 1 | 0 | **PASS** |
| **Fitur STAY: Extension with Dynamic Pricing (E2E-58)** | 1 | 1 | 0 | **PASS** |
| **Fitur STAY: Extension Zero Nights Rejection (E2E-59)** | 1 | 1 | 0 | **PASS** |
| **Fitur STAY: Room Move Audit Log Inquiry (E2E-60)** | 1 | 1 | 0 | **PASS** |
| **TOTAL** | **60** | **60** | **0** | **100% PASS** |

---

## 2. Detil Verifikasi Skenario Baru (Stay Modification)

### A. E2E-56: Mid-Stay Room Move on Inspected Room & Auto-Dirty Transition (FR-STAY-01)
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/room-move`
- **Uji Otorisasi RBAC:**
  - `guest` mencoba akses: **`403 Forbidden`** (Tamu publik dilarang memanipulasi alokasi kamar hotel).
  - `receptionist` mengakses: **`200 OK`** (Staf Front Desk memindahkan tamu in-house ke kamar target yang siap huni).
- **Hasil Response Payload:**
  ```json
  {
    "status": "ok",
    "booking_id": "bk-e2e-001",
    "previous_room_number": "301",
    "new_room_number": "202",
    "move_date": "2026-10-03",
    "message": "pemindahan kamar berhasil; kamar 301 telah ditandai vacant_dirty"
  }
  ```
- **Verifikasi Integritas Data & Housekeeping Board:**
  - Kamar asal (301) seketika bertransisi menjadi `vacant_dirty`.
  - Kamar tujuan (202) yang awalnya `inspected` bertransisi menjadi `occupied`.
  - Penugasan masa menginap tamu di database berpindah ke kamar 202 tanpa melanggar constraint GiST exclusion.

---

### B. E2E-57: Room Move Readiness Guard Rejection on Dirty Target Room (FR-STAY-01)
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/room-move`
- **Skenario:** Kamar target 204 berstatus `vacant_dirty`.
- **Hasil:**
  - HTTP Status: **`409 Conflict`**
  - Error Response:
    ```json
    {
      "code": "TARGET_ROOM_NOT_READY",
      "error": "stay: kamar target belum siap huni (wajib berstatus inspected)"
    }
    ```
- **Verifikasi Kualitas:** Tamu terhindar dari pemindahan ke kamar kotor yang belum dibersihkan oleh tim Housekeeping.

---

### C. E2E-58: Stay Extension with Dynamic Pricing & Inventory Calculation (FR-STAY-02)
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/extend-stay`
- **Request Payload:**
  ```json
  {
    "additional_nights": 2,
    "payment_method": "front_desk_edc"
  }
  ```
- **Hasil Response (200 OK):**
  ```json
  {
    "status": "ok",
    "booking_id": "bk-e2e-001",
    "previous_check_out": "2026-10-05",
    "new_check_out": "2026-10-07",
    "additional_nights": 2,
    "additional_amount_minor": 1237500,
    "new_total_price_minor": 2337500,
    "payment_status": "settled"
  }
  ```
- **Verifikasi Mesin Tarif & Stok:**
  - Tanggal checkout bertambah tepat 2 hari dari tanggal sebelumnya.
  - Tambahan tarif dihitung secara dinamis melalui mesin tarif hotel (termasuk penyesuaian akhir pekan).
  - Kuota inventaris untuk malam-malam baru berkurang secara atomik.

---

### D. E2E-59: Stay Extension Validation: Rejection on Zero Nights (FR-STAY-02)
- **Endpoint:** `POST /api/v1/bookings/bk-e2e-001/extend-stay`
- **Request Payload:** `{"additional_nights": 0}`
- **Hasil:**
  - HTTP Status: **`400 Bad Request`**
  - Error Response:
    ```json
    {
      "code": "INVALID_ADDITIONAL_NIGHTS",
      "error": "stay: jumlah malam perpanjangan harus antara 1 dan 30 malam"
    }
    ```

---

### E. E2E-60: Room Move Audit Log Inquiry & RBAC Isolation (FR-STAY-03)
- **Endpoint:** `GET /api/v1/bookings/bk-e2e-001/room-moves`
- **Uji Otorisasi & Respon:**
  - `guest` mengakses: **`403 Forbidden`**.
  - `receptionist` mengakses: **`200 OK`**.
- **Payload Response:**
  ```json
  {
    "booking_id": "bk-e2e-001",
    "moves": [
      {
        "id": "mov-e2e-001",
        "booking_id": "bk-e2e-001",
        "from_room_number": "301",
        "to_room_number": "202",
        "move_date": "2026-10-03",
        "reason_category": "maintenance_defect",
        "notes": "AC 301 bocor",
        "actor_id": "staff:receptionist",
        "created_at": "2026-10-03T11:35:49Z"
      }
    ]
  }
  ```

---

## 3. Log Verifikasi Terminal

```text
=== RUN   TestEndToEndHotelBookingRBACLifecycle
...
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-55:_Front_Desk_List_Shift_Handover_Notes_History_(200_OK_&_403_Forbidden) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-56:_Mid-Stay_Room_Move_on_Inspected_Room_&_Auto-Dirty_Transition_(200_OK_&_403_Forbidden) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-57:_Room_Move_Readiness_Guard_Rejection_on_Dirty_Target_Room_(409_Conflict) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-58:_Stay_Extension_with_Dynamic_Pricing_&_Inventory_Calculation_(200_OK_&_403_Forbidden) (0.01s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-59:_Stay_Extension_Validation:_Rejection_on_Zero_Nights_(400_Bad_Request) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-60:_Room_Move_Audit_Log_Inquiry_&_RBAC_Isolation_(200_OK_&_403_Forbidden) (0.00s)
PASS
ok  	github.com/example/hotel-booking/testing/e2e/script	0.056s
```

---

## 4. Kesimpulan
Fitur **Stay Modification: Room Move & Stay Extension (Proposed 03)** telah teruji dan terverifikasi secara tuntas memenuhi seluruh kriteria penerimaan (Acceptance Criteria) pada PRD dan spesifikasi teknis pada SRS.
