# SRS — Housekeeping Room Status & Readiness Lifecycle
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Terkait:
- **PRD Pasangan:** [PRD-Housekeeping-Readiness](../prd/housekeeping-room-status-and-readiness-2026-10-03.md)
- **Tech Architecture:** [TECH-Housekeeping-Readiness](../tech/housekeeping-room-status-architecture-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough Housekeeping](../walkthrough/housekeeping-room-status-walkthrough-2026-10-03.md)

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-HK-01: Monitoring Status 95 Kamar (Room Board Query)
Sistem wajib menyediakan endpoint bagi staf Housekeeping dan Resepsionis untuk memantau status fisik 95 kamar hotel Pulang ke Uttara, dapat difilter berdasarkan lantai (*floor*), status kebersihan, atau tipe kamar.

### FR-HK-02: Pembaruan Status Kebersihan Kamar
Sistem wajib menyediakan endpoint pembaruan status kamar dengan validasi transisi terpusat:
- `vacant_dirty` ➔ `cleaning` (oleh Room Attendant saat memulai pekerjaan)
- `cleaning` ➔ `vacant_clean` (oleh Room Attendant saat selesai membersihkan)
- `vacant_clean` ➔ `inspected` (oleh HK Supervisor setelah inspeksi kualitas)
- `vacant_clean` ➔ `vacant_dirty` (penolakan inspeksi / rejected bila masih ditemukan kotor)

### FR-HK-03: Penanganan Kamar Rusak (Out of Order & Out of Service)
Sistem menyediakan mekanisme penandaan kamar rusak:
- `out_of_service`: Masalah minor, tidak mengurangi kuota inventaris web.
- `out_of_order`: Masalah struktural/renovasi, memotong inventaris kamar terkait secara atomik pada rentang tanggal perbaikan.

### FR-HK-04: Validasi Kesiapan Kamar pada Alur Check-in
Saat resepsionis memanggil `POST /api/v1/bookings/{id}/check-in`, sistem wajib memverifikasi bahwa kamar yang ditugaskan (*assigned room*) berada pada status `inspected`. Jika tidak, proses check-in digagalkan dengan kode error `ROOM_NOT_READY` (HTTP 409 Conflict).

### FR-HK-05: Otomasi Status Saat Check-out
Saat tamu melakukan check-out (`POST /api/v1/bookings/{id}/check-out`), sistem secara otomatis mengubah status kamar fisik yang ditinggalkan menjadi `vacant_dirty` dan `occupancy_status` menjadi `vacant`.

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. GET /api/v1/housekeeping/rooms
* **Otorisasi:** Role `housekeeping`, `receptionist`, `gm_admin`
* **Query Parameters:**
  * `floor` (optional, integer: 1, 2, 3, 5, dst.)
  * `status` (optional: `vacant_dirty`, `cleaning`, `vacant_clean`, `inspected`, `occupied`, `out_of_service`, `out_of_order`)
  * `room_type_id` (optional, UUID)
* **Response: 200 OK**
  ```json
  {
    "total_rooms": 95,
    "summary": {
      "inspected": 45,
      "occupied": 30,
      "vacant_clean": 10,
      "cleaning": 5,
      "vacant_dirty": 3,
      "out_of_service": 1,
      "out_of_order": 1
    },
    "rooms": [
      {
        "room_number": "101",
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "room_type_name": "Superior King",
        "floor": 1,
        "cleanliness_status": "inspected",
        "occupancy_status": "vacant",
        "current_booking_id": null,
        "guest_name": null,
        "maintenance_notes": "",
        "updated_at": "2026-10-03T10:00:00Z",
        "updated_by": "staff:hk_supervisor_01"
      }
    ]
  }
  ```

### 2.2. PUT /api/v1/housekeeping/rooms/{room_number}/status
* **Otorisasi:** Role `housekeeping`, `gm_admin`
* **Path Parameter:** `room_number` (string, misal: "101")
* **Request JSON:**
  ```json
  {
    "to_status": "inspected",
    "notes": "QC passed, linen replaced, amenities complete"
  }
  ```
* **Response: 200 OK**
  ```json
  {
    "status": "ok",
    "room_number": "101",
    "cleanliness_status": "inspected",
    "updated_at": "2026-10-03T11:00:00Z"
  }
  ```
* **Error: 409 Conflict (`INVALID_STATUS_TRANSITION`)**
  ```json
  {
    "error": "transisi dari vacant_dirty langsung ke inspected tidak diizinkan; kamar wajib melalui status clean terlebih dahulu",
    "code": "INVALID_STATUS_TRANSITION"
  }
  ```

### 2.3. POST /api/v1/housekeeping/rooms/{room_number}/out-of-order
* **Otorisasi:** Role `gm_admin`
* **Request JSON:**
  ```json
  {
    "start_date": "2026-10-05",
    "end_date": "2026-10-08",
    "reason": "AC compressor replacement & bathroom tile repair",
    "is_out_of_order": true
  }
  ```
* **Response: 200 OK**
  ```json
  {
    "status": "ok",
    "room_number": "205",
    "status": "out_of_order",
    "inventory_deducted_dates": ["2026-10-05", "2026-10-06", "2026-10-07"]
  }
  ```

---

## 3. Spesifikasi Skema Database

```sql
-- Penambahan atribut operasional pada tabel rooms
ALTER TABLE rooms
    ADD COLUMN IF NOT EXISTS cleanliness_status VARCHAR(32) NOT NULL DEFAULT 'inspected'
        CHECK (cleanliness_status IN ('vacant_dirty', 'cleaning', 'vacant_clean', 'inspected', 'occupied', 'out_of_service', 'out_of_order')),
    ADD COLUMN IF NOT EXISTS maintenance_notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_by VARCHAR(64) NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_rooms_cleanliness ON rooms(cleanliness_status);

-- Aturan Hak Akses Casbin untuk Modul Housekeeping
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'housekeeping', '/api/v1/housekeeping/rooms', 'GET'),
    ('p', 'housekeeping', '/api/v1/housekeeping/rooms/:id/status', 'PUT'),
    ('p', 'receptionist', '/api/v1/housekeeping/rooms', 'GET'),
    ('p', 'gm_admin', '/api/v1/housekeeping/*', '*')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;
```
