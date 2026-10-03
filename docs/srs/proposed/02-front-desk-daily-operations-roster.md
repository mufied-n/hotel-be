# SRS — Proposed 02: Front Desk Daily Operations Roster & Shift Board
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen PRD Pasangan:** [PRD-Proposed-02](../../prd/proposed/02-front-desk-daily-operations-roster.md)
- **Status:** Proposed / Under Review
- **Target Role:** `receptionist`, `gm_admin`, `revenue_mgr`, `housekeeping`

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-FD-01: Agregasi Roster Harian Meja Depan (Daily Roster Aggregation)
Sistem wajib menyediakan satu endpoint efisien yang mengumpulkan data operasional hari berjalan (*operational date*):
1. **Ringkasan Statistik:** Total kamar (95), jumlah kamar terisi (*occupied*), jumlah kamar siap huni (*vacant ready / inspected*), kamar kotor (*vacant dirty*), kamar perbaikan (*out of order*), dan rasio okupansi harian (%).
2. **Daftar Expected Arrivals:** Seluruh reservasi berstatus `confirmed` dengan `check_in = CURRENT_DATE`.
3. **Daftar Expected Departures:** Seluruh reservasi berstatus `checked_in` dengan `check_out = CURRENT_DATE`.
4. **Daftar In-House Guests:** Seluruh tamu yang sedang aktif menginap di kamar fisik.

### FR-FD-02: Filter & Parameter Tanggal
Endpoint roster harian wajib mendukung query parameter `date` (format: `YYYY-MM-DD`, default: tanggal hari ini WIB). Hal ini memungkinkan staf meninjau perkiraan kedatangan besok (*forecasted arrivals*).

### FR-FD-03: Pencatatan Log Serah Terima Shift (Handover Logbook)
Sistem wajib menyediakan endpoint bagi staf resepsionis untuk mencatat log serah terima shift (pagi, sore, malam), mencakup ringkasan kas kecil (*cash float balance*), titipan tamu, dan isu pending yang memerlukan perhatian regu berikutnya.

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. GET /api/v1/front-desk/daily-roster
* **Otorisasi:** Role `receptionist`, `gm_admin`, `revenue_mgr`, `housekeeping`
* **Query Parameters:**
  * `date` (optional, string `YYYY-MM-DD`, default: hari ini)
* **Response: 200 OK**
  ```json
  {
    "date": "2026-10-03",
    "metrics": {
      "total_rooms": 95,
      "sellable_rooms": 94,
      "out_of_order_rooms": 1,
      "occupied_rooms": 68,
      "vacant_inspected_rooms": 20,
      "vacant_dirty_rooms": 6,
      "occupancy_rate_percent": 72.34
    },
    "expected_arrivals": {
      "total": 12,
      "bookings": [
        {
          "booking_id": "01900000-0000-7000-8000-000000000001",
          "guest_name": "Rian Ardianto",
          "guest_phone": "+6281234567890",
          "room_type_name": "Deluxe King",
          "assigned_room": "301",
          "num_rooms": 1,
          "num_guests": 2,
          "estimated_arrival_time": "14:00",
          "special_requests": "Lantai tinggi, non-smoking",
          "payment_status": "paid",
          "total_price_minor": 1500000
        }
      ]
    },
    "expected_departures": {
      "total": 8,
      "bookings": [
        {
          "booking_id": "01900000-0000-7000-8000-000000000002",
          "guest_name": "Siti Nurhaliza",
          "room_number": "204",
          "check_in_date": "2026-10-01",
          "check_out_date": "2026-10-03",
          "balance_remaining_minor": 0
        }
      ]
    },
    "in_house_count": 68
  }
  ```

### 2.2. POST /api/v1/front-desk/handover-notes
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Request JSON:**
  ```json
  {
    "shift": "morning",
    "cash_float_minor": 1500000,
    "pending_issues": "Kamar 205 komplain AC kurang dingin, teknisi dijadwalkan jam 16:00",
    "vip_guest_notes": "Tamu VIP Pak Budi (kamar 501) minta taksi bandara jam 18:00"
  }
  ```
* **Response: 201 Created**
  ```json
  {
    "id": "hnd_01900000_1234",
    "shift": "morning",
    "actor_id": "staff:receptionist_01",
    "created_at": "2026-10-03T15:00:00Z",
    "status": "recorded"
  }
  ```

### 2.3. GET /api/v1/front-desk/handover-notes
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Query Parameters:** `limit` (default: 20), `offset` (default: 0)
* **Response: 200 OK** (Daftar catatan shift terurut waktu menurun).

---

## 3. Spesifikasi Skema Database

```sql
CREATE TABLE IF NOT EXISTS front_desk_handover_notes (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    shift VARCHAR(16) NOT NULL CHECK (shift IN ('morning', 'afternoon', 'night')),
    cash_float_minor BIGINT NOT NULL DEFAULT 0,
    pending_issues TEXT NOT NULL DEFAULT '',
    vip_guest_notes TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(64) NOT NULL,
    actor_name VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_handover_notes_created_at ON front_desk_handover_notes(created_at DESC);
```
