# SRS — Front Desk Daily Operations Roster & Shift Handover Board
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Dokumen PRD Pasangan:** [PRD-Front-Desk-Roster](../prd/front-desk-daily-operations-roster-2026-10-03.md)
- **Desain Arsitektur Teknis:** [TECH-Front-Desk-Roster](../tech/front-desk-daily-operations-roster-architecture-2026-10-03.md)
- **Pelacak Eksekusi:** [Walkthrough Front Desk](../walkthrough/front-desk-daily-operations-roster-walkthrough-2026-10-03.md)
- **Status:** Approved / Ready for Implementation

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-FD-01: Agregasi Roster Harian Meja Depan (Daily Operations Roster)
Sistem menyediakan endpoint terpadu untuk menyajikan data operasional harian hotel pada tanggal tertentu (*operational date*):
1. **Metrik Okupansi & Kamar:**
   - Total kapasitas hotel (95 kamar).
   - Kamar siap jual (*sellable rooms* = 95 - kamar OOO).
   - Kamar terisi (*occupied*), kamar siap huni (*inspected*), kamar kotor (*vacant dirty*), kamar sedang dibersihkan (*cleaning*), dan kamar rusak (*out of order*).
   - Rasio okupansi harian dalam persentase (%).
2. **Daftar Expected Arrivals:**
   - Reservasi berstatus `confirmed` dengan `check_in = operational_date`.
   - Menampilkan: ID booking, nama tamu, no telepon tamu, tipe kamar, nomor kamar yang ditugaskan (jika ada), jumlah kamar, jumlah tamu, jam kedatangan (*estimated_arrival_time*), dan *special_requests*.
3. **Daftar Expected Departures:**
   - Reservasi berstatus `checked_in` dengan `check_out = operational_date`.
   - Menampilkan: ID booking, nama tamu, nomor kamar fisik, tanggal check-in, dan tanggal check-out.
4. **Daftar In-House Guests:**
   - Seluruh reservasi berstatus `checked_in` yang menghuni hotel pada tanggal operasional.

### FR-FD-02: Dukungan Parameter Tanggal Forecast
Endpoint roster menerima query parameter opsional `date` (format: `YYYY-MM-DD`, default: tanggal hari ini WIB). Jika tanggal tidak valid, sistem mengembalikan HTTP 400 Bad Request (`INVALID_DATE_FORMAT`).

### FR-FD-03: Pencatatan Log Serah Terima Shift (Handover Logbook)
Sistem menyediakan endpoint pencatatan dan penelusuran riwayat serah terima shift antar-regu meja depan:
- Input: `shift` (`morning`, `afternoon`, `night`), `cash_float_minor`, `pending_issues`, `vip_guest_notes`.
- Audit Identity: Aktor dan role dicatat otomatis dari context autentikasi (`GetAuthContext`).
- Riwayat catatan shift dapat dibaca secara terurut waktu menurun (*latest first*).

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. GET /api/v1/front-desk/daily-roster
* **Otorisasi:** Role `receptionist`, `housekeeping`, `revenue_mgr`, `gm_admin`
* **Query Parameters:**
  * `date` (optional, string `YYYY-MM-DD`, default: hari ini waktu operasional WIB)
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
          "room_type_id": "01900000-0000-7000-8000-000000000001",
          "room_type_name": "Superior King",
          "assigned_rooms": ["201"],
          "num_rooms": 1,
          "num_guests": 2,
          "estimated_arrival_time": "14:00",
          "special_requests": "Lantai tinggi, non-smoking",
          "total_price_minor": 1100000
        }
      ]
    },
    "expected_departures": {
      "total": 8,
      "bookings": [
        {
          "booking_id": "01900000-0000-7000-8000-000000000002",
          "guest_name": "Siti Nurhaliza",
          "room_numbers": ["204"],
          "check_in_date": "2026-10-01",
          "check_out_date": "2026-10-03"
        }
      ]
    },
    "in_house_count": 68
  }
  ```
* **Error Response:**
  * `400 Bad Request` (`INVALID_DATE_FORMAT`) bila parameter tanggal bukan format YYYY-MM-DD yang valid.
  * `403 Forbidden` (`FORBIDDEN`) bila role pemanggil adalah `guest` atau tidak diizinkan.

---

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
    "status": "ok",
    "note": {
      "id": "01900000-0000-7000-8000-000000000001",
      "shift": "morning",
      "cash_float_minor": 1500000,
      "pending_issues": "Kamar 205 komplain AC kurang dingin, teknisi dijadwalkan jam 16:00",
      "vip_guest_notes": "Tamu VIP Pak Budi (kamar 501) minta taksi bandara jam 18:00",
      "actor_id": "staff:receptionist_01",
      "actor_role": "receptionist",
      "created_at": "2026-10-03T15:00:00Z"
    }
  }
  ```
* **Error Response:**
  * `400 Bad Request` (`INVALID_SHIFT`) bila shift bukan salah satu dari `morning`, `afternoon`, atau `night`.
  * `403 Forbidden` bila bukan staf meja depan / GM.

---

### 2.3. GET /api/v1/front-desk/handover-notes
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Query Parameters:** `limit` (integer, default: 20, max: 100), `offset` (integer, default: 0)
* **Response: 200 OK**
  ```json
  {
    "total": 1,
    "notes": [
      {
        "id": "01900000-0000-7000-8000-000000000001",
        "shift": "morning",
        "cash_float_minor": 1500000,
        "pending_issues": "Kamar 205 komplain AC kurang dingin",
        "vip_guest_notes": "Tamu VIP Pak Budi minta taksi bandara",
        "actor_id": "staff:receptionist_01",
        "actor_role": "receptionist",
        "created_at": "2026-10-03T15:00:00Z"
      }
    ]
  }
  ```

---

## 3. Spesifikasi Skema Database & Migrasi (Goose 00011)

```sql
-- Migrations: 00011_front_desk_roster_and_handover.sql

CREATE TABLE IF NOT EXISTS front_desk_handover_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shift VARCHAR(16) NOT NULL CHECK (shift IN ('morning', 'afternoon', 'night')),
    cash_float_minor BIGINT NOT NULL DEFAULT 0,
    pending_issues TEXT NOT NULL DEFAULT '',
    vip_guest_notes TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(64) NOT NULL,
    actor_role VARCHAR(64) NOT NULL DEFAULT 'receptionist',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_handover_notes_created_at ON front_desk_handover_notes(created_at DESC);

-- Aturan Casbin RBAC untuk Modul Front Desk
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'receptionist', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'receptionist', '/api/v1/front-desk/handover-notes', 'GET'),
    ('p', 'receptionist', '/api/v1/front-desk/handover-notes', 'POST'),
    ('p', 'housekeeping', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'revenue_mgr', '/api/v1/front-desk/daily-roster', 'GET'),
    ('p', 'gm_admin', '/api/v1/front-desk/*', '*')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;
```
