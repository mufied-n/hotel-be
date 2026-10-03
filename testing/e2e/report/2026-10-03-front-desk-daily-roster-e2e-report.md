# End-to-End (E2E) Test Report: Front Desk Daily Operations Roster & Shift Handover Board (Proposed 02)
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**  
**Tanggal Eksekusi:** 2026-10-03 11:27:00 WIB  
**Dokumen Terkait:** [PRD Front Desk](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/front-desk-daily-operations-roster-2026-10-03.md) | [SRS Front Desk](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/front-desk-daily-operations-roster-2026-10-03.md) | [Tech Architecture Front Desk](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/front-desk-daily-operations-roster-architecture-2026-10-03.md) | [Walkthrough Tracking](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/front-desk-daily-operations-roster-walkthrough-2026-10-03.md)  
**Status Pengujian:** **100% PASS (55/55 Scenarios)**

---

## 1. Ringkasan Eksekusi (Executive Summary)

Pengujian End-to-End (E2E) untuk modul **Proposed 02: Front Desk Daily Operations Roster & Shift Handover Board** telah dieksekusi secara otomatis menggunakan test runner Go di [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) serta skrip executable bash di [`testing/e2e/script/front_desk_daily_roster_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/front_desk_daily_roster_e2e.sh).

Seluruh 4 skenario baru (E2E-52 hingga E2E-55) dan 51 skenario regresi sebelumnya (E2E-01 hingga E2E-51) berhasil lulus 100% tanpa adanya error, kebocoran data, ataupun regresi otorisasi.

| Kategori Pengujian | Total Skenario | Lulus (Pass) | Gagal (Fail) | Status |
| :--- | :---: | :---: | :---: | :---: |
| **Skenario Regresi (E2E-01 s/d E2E-51)** | 51 | 51 | 0 | **PASS** |
| **Fitur FDR: Daily Operations Roster & Multi-Role RBAC (E2E-52)** | 1 | 1 | 0 | **PASS** |
| **Fitur FDR: Roster Forecast Date Query & Validation (E2E-53)** | 1 | 1 | 0 | **PASS** |
| **Fitur FDR: Record Shift Handover Note (E2E-54)** | 1 | 1 | 0 | **PASS** |
| **Fitur FDR: List Shift Handover History & Anti-Leakage (E2E-55)** | 1 | 1 | 0 | **PASS** |
| **TOTAL** | **55** | **55** | **0** | **100% PASS** |

---

## 2. Detil Verifikasi Skenario Baru (Front Desk Operations)

### A. E2E-52: Front Desk Daily Operations Roster Query & Multi-Role RBAC (FR-FDR-01, FR-FDR-02)
- **Endpoint:** `GET /api/v1/front-desk/daily-roster`
- **Uji Otorisasi RBAC:**
  - `guest` mencoba akses: **`403 Forbidden`** (Tamu publik diblokir dari dashboard operasional hotel).
  - `receptionist` mengakses: **`200 OK`** (Staf Front Desk memantau kedatangan, keberangkatan, dan okupansi).
  - `housekeeping` mengakses: **`200 OK`** (Staf HK melihat pergerakan tamu untuk alokasi attendant).
  - `revenue_mgr` mengakses: **`200 OK`** (Revenue Manager memantau tingkat okupansi real-time untuk penyesuaian dinamis).
- **Hasil Response Payload:**
  ```json
  {
    "date": "2026-10-03",
    "metrics": {
      "total_rooms": 95,
      "sellable_rooms": 94,
      "out_of_order_rooms": 1,
      "occupied_rooms": 1,
      "vacant_inspected_rooms": 1,
      "vacant_dirty_rooms": 4,
      "cleaning_rooms": 0,
      "occupancy_rate_percent": 1.06
    },
    "expected_arrivals": [
      {
        "booking_id": "bk-e2e-001",
        "guest_name": "Budi Santoso",
        "guest_phone": "081234567890",
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "room_type_name": "Superior King",
        "assigned_rooms": ["301"],
        "num_rooms": 1,
        "num_guests": 2,
        "estimated_arrival_time": "14:00",
        "special_requests": "Quiet high floor room",
        "total_price_minor": 1100000
      }
    ],
    "expected_departures": [
      {
        "booking_id": "bk-e2e-001",
        "guest_name": "Budi Santoso",
        "room_numbers": ["301"],
        "check_in_date": "2026-10-03",
        "check_out_date": "2026-10-05"
      }
    ],
    "in_house_count": 1
  }
  ```
- **Verifikasi Bisnis:**
  - `total_rooms` tepat 95 kamar fisik.
  - Kamar 203 yang diisolasi ke status Out of Order (OOO) pada E2E-51 berhasil mengurangi `sellable_rooms` menjadi 94 kamar fisik ($95 - 1 = 94$).
  - Perhitungan persentase okupansi dihitung dari kamar yang dapat dijual: $(1 / 94) \times 100\% = 1.06\%$.

---

### B. E2E-53: Front Desk Daily Roster Forecast Date Query & Validation (FR-FDR-01)
- **Endpoint:** `GET /api/v1/front-desk/daily-roster?date=YYYY-MM-DD`
- **Uji Validasi:**
  1. Parameter tanggal salah format (`?date=10-10-2026`):
     - HTTP Status: **`400 Bad Request`**
     - Payload Error: `{"code":"INVALID_DATE_FORMAT","error":"parameter 'date' harus berformat YYYY-MM-DD"}`
  2. Parameter tanggal valid ISO 8601 (`?date=2026-10-10`):
     - HTTP Status: **`200 OK`**
     - Memuat data perkiraan kedatangan dan keberangkatan khusus pada tanggal 10 Oktober 2026.

---

### C. E2E-54: Front Desk Record Shift Handover Note (FR-FDR-03)
- **Endpoint:** `POST /api/v1/front-desk/handover-notes`
- **Uji Otorisasi & Validasi:**
  1. `guest` mencoba mencatat logbook shift:
     - HTTP Status: **`403 Forbidden`**.
  2. Input shift tidak valid (`shift: "evening"`):
     - HTTP Status: **`400 Bad Request`** (`INVALID_SHIFT`).
  3. Catatan penting kosong (`pending_issues` dan `vip_guest_notes` dua-duanya string kosong):
     - HTTP Status: **`400 Bad Request`** (`INVALID_INPUT`).
  4. Resepsionis mencatat shift pagi (`shift: "morning"`):
     - Request Body:
       ```json
       {
         "shift": "morning",
         "cash_float_minor": 1500000,
         "pending_issues": "Kunci kamar 201 perlu baterai baru",
         "vip_guest_notes": "VIP Mr. Tan check-in jam 14.00"
       }
       ```
     - HTTP Status: **`201 Created`**
     - Response Body:
       ```json
       {
         "status": "ok",
         "note": {
           "id": "hnd-e2e-001",
           "shift": "morning",
           "cash_float_minor": 1500000,
           "pending_issues": "Kunci kamar 201 perlu baterai baru",
           "vip_guest_notes": "VIP Mr. Tan check-in jam 14.00",
           "actor_id": "staff:receptionist",
           "actor_role": "receptionist",
           "created_at": "2026-10-03T11:25:20Z"
         }
       }
       ```

---

### D. E2E-55: Front Desk List Shift Handover History & Anti-Leakage (FR-FDR-03)
- **Endpoint:** `GET /api/v1/front-desk/handover-notes`
- **Uji Fungsional & RBAC Isolation:**
  1. Resepsionis menambahkan catatan shift kedua (`afternoon`):
     - HTTP Status: **`201 Created`**.
  2. Resepsionis membaca riwayat catatan serah terima shift (`?limit=10&offset=0`):
     - HTTP Status: **`200 OK`**.
     - Memuat 2 entri catatan (`total: 2`).
  3. Staf Housekeeping mencoba membaca catatan serah terima shift Meja Depan:
     - HTTP Status: **`403 Forbidden`**.
     - **Verifikasi Keamanan:** Catatan internal kas meja depan (*cash float*) dan isu staf meja depan terlindungi dari departemen lain yang tidak berkepentingan langsung.

---

## 3. Log Verifikasi Terminal

```text
=== RUN   TestEndToEndHotelBookingRBACLifecycle
...
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-51:_GM_Admin_Out-of-Order_(OOO)_Isolation_&_Non-GM_Rejection_(200_OK_&_403_Forbidden) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-52:_Front_Desk_Daily_Operations_Roster_Query_&_Multi-Role_RBAC_(200_OK_&_403_Forbidden) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-53:_Front_Desk_Daily_Roster_Forecast_Date_Query_&_Validation_(200_OK_&_400_Bad_Request) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-54:_Front_Desk_Record_Shift_Handover_Note_(201_Created,_400_Bad_Request,_&_403_Forbidden) (0.00s)
    --- PASS: TestEndToEndHotelBookingRBACLifecycle/E2E-55:_Front_Desk_List_Shift_Handover_Notes_History_(200_OK_&_403_Forbidden) (0.00s)
PASS
ok  	github.com/example/hotel-booking/testing/e2e/script	0.050s
```

```text
=== [E2E] Front Desk Daily Operations Roster & Shift Handover Board Testing ===
Target Base URL: http://localhost:8080
--- 1. Healthcheck ---
✓ Service Healthcheck OK
--- 2. Negative Test: Guest Access Denial to Roster (403 Forbidden) ---
✓ Guest ditolak akses Daily Roster (403 Forbidden)
--- 3. Receptionist Query Daily Operations Roster (GET /api/v1/front-desk/daily-roster) ---
✓ Receptionist Daily Roster Query Sukses (200 OK, total 95 physical rooms)
--- 4. Cross-Department Daily Roster Access (Housekeeping & Revenue Mgr) ---
✓ Housekeeping (200 OK) dan Revenue Manager (200 OK) sukses mengakses Daily Roster
--- 5. Future Date Forecast Query (GET ?date=2026-10-10) ---
✓ Forecast Daily Roster Sukses untuk tanggal 2026-10-10 (200 OK)
--- 6. Receptionist Record Shift Handover Note (POST /api/v1/front-desk/handover-notes) ---
✓ Pencatatan Handover Note Sukses (201 Created)
--- 7. Negative Test: Guest Rejection on Handover Note (403 Forbidden) ---
✓ Guest ditolak mencatat Handover Note (403 Forbidden)
--- 8. Negative Test: Invalid Shift Rejection (400 Bad Request) ---
✓ Shift tidak valid ditolak dengan 400 Bad Request
--- 9. Receptionist List Handover Notes History (GET /api/v1/front-desk/handover-notes) ---
✓ Riwayat Handover Notes Sukses Dimuat (200 OK)
--- 10. Negative Test: Housekeeping Forbidden from Handover Notes (403 Forbidden) ---
✓ Housekeeping dilarang membaca Handover Notes internal Meja Depan (403 Forbidden)
=== Selesai: Seluruh pengujian skrip Front Desk Daily Operations Roster lulus! ===
```

---

## 4. Kesimpulan & Rekomendasi
Fitur **Front Desk Daily Operations Roster & Shift Handover Board (Proposed 02)** telah terverifikasi secara tuntas memenuhi seluruh kriteria penerimaan (Acceptance Criteria) pada PRD dan spesifikasi teknis pada SRS. Modul siap untuk dikomit dan diintegrasikan ke lingkungan staging/produksi.
