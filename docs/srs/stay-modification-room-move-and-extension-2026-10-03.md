# SRS — Proposed 03: Stay Modification, Room Move & Stay Extension
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Status:** Approved / Ready for Implementation
- **Target Role:** `receptionist`, `gm_admin`, `finance`, `housekeeping`
- **Dokumen PRD Pasangan:** [PRD Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/stay-modification-room-move-and-extension-2026-10-03.md)
- **Dokumen Tech Architecture:** [Tech Architecture Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/stay-modification-room-move-architecture-2026-10-03.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-STAY-01: Pemindahan Kamar Fisik (Room Move)
Sistem wajib menyediakan API bagi staf Meja Depan untuk memindahkan tamu berstatus `checked_in` ke kamar fisik target:
1. Memverifikasi bahwa reservasi berstatus `checked_in`.
2. Memverifikasi kamar target terdaftar di hotel dan berstatus `inspected`.
3. Memotong rentang tanggal kamar lama di `room_assignments` menjadi `[check_in, CURRENT_DATE)` (atau menghapusnya jika hari pindah sama dengan check-in).
4. Menambahkan penugasan kamar baru di `room_assignments` dengan rentang `[CURRENT_DATE, check_out)`.
5. Mengubah status kamar lama menjadi `vacant_dirty`.
6. Mengubah status kamar baru menjadi `occupied`.
7. Mencatat log riwayat ke `room_move_logs`.

### FR-STAY-02: Perpanjangan Masa Menginap (Stay Extension)
Sistem wajib menyediakan API perpanjangan malam menginap:
1. Memverifikasi bahwa reservasi berstatus `checked_in` atau `confirmed`.
2. Menerima jumlah malam tambahan (`additional_nights` $\ge 1$).
3. Menghitung tanggal check-out baru: `new_check_out = old_check_out + additional_nights`.
4. Memeriksa ketersediaan kuota inventaris pada rentang `[old_check_out, new_check_out)`.
5. Mendekremen kuota inventaris untuk malam-malam tambahan.
6. Menghitung tarif malam tambahan menggunakan pricing engine.
7. Memperbarui rentang penugasan kamar di `room_assignments` menjadi `[assignment_start, new_check_out)`.
8. Menambahkan catatan malam tambahan pada `reservation_room_nights` dan memperbarui `check_out` serta `total_price_minor` pada tabel `bookings`.

### FR-STAY-03: Riwayat Pemindahan Kamar (Room Move Audit Log)
Sistem wajib menyediakan API untuk mengambil riwayat pemindahan kamar berdasarkan ID booking atau daftar riwayat operasional harian.

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. POST /api/v1/bookings/{id}/room-move
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Path Parameter:** `id` (UUID booking)
* **Request JSON:**
  ```json
  {
    "target_room_number": "305",
    "reason_category": "maintenance_defect",
    "notes": "AC kamar 101 bocor dan berisik"
  }
  ```
  * `reason_category`: salah satu dari `maintenance_defect`, `noise_complaint`, `upgrade`, `guest_request`.
* **Response: 200 OK**
  ```json
  {
    "status": "ok",
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "previous_room_number": "101",
    "new_room_number": "305",
    "move_date": "2026-10-03",
    "message": "pemindahan kamar berhasil; kamar 101 telah ditandai vacant_dirty"
  }
  ```
* **Error Response:**
  * `400 Bad Request` (`INVALID_INPUT`) bila kamar target sama dengan kamar asal, atau alasan tidak valid.
  * `404 Not Found` (`BOOKING_NOT_FOUND`) bila booking tidak ditemukan.
  * `409 Conflict` (`INVALID_BOOKING_STATUS`) bila booking bukan dalam status `checked_in`.
  * `409 Conflict` (`TARGET_ROOM_NOT_READY`) bila kamar target belum berstatus `inspected`.
  * `409 Conflict` (`ROOM_PHYSICAL_OVERLAP`) bila kamar target telah dipesan/ditempati oleh tamu lain.
  * `403 Forbidden` bila bukan staf meja depan / GM.

---

### 2.2. POST /api/v1/bookings/{id}/extend-stay
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Path Parameter:** `id` (UUID booking)
* **Request JSON:**
  ```json
  {
    "additional_nights": 2,
    "payment_method": "front_desk_edc"
  }
  ```
* **Response: 200 OK**
  ```json
  {
    "status": "ok",
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "previous_check_out": "2026-10-05",
    "new_check_out": "2026-10-07",
    "additional_nights": 2,
    "additional_amount_minor": 1500000,
    "new_total_price_minor": 3000000
  }
  ```
* **Error Response:**
  * `400 Bad Request` (`INVALID_ADDITIONAL_NIGHTS`) bila `additional_nights` $< 1$ atau $> 30$.
  * `404 Not Found` (`BOOKING_NOT_FOUND`) bila booking tidak ditemukan.
  * `409 Conflict` (`NO_AVAILABILITY_FOR_EXTENSION`) bila inventaris kamar tipe tersebut telah habis.
  * `409 Conflict` (`ROOM_PHYSICAL_OVERLAP`) bila kamar fisik yang ditempati telah ditugaskan ke tamu lain pada rentang tanggal perpanjangan.
  * `403 Forbidden` bila bukan staf meja depan / GM.

---

### 2.3. GET /api/v1/bookings/{id}/room-moves
* **Otorisasi:** Role `receptionist`, `gm_admin`, `finance`
* **Path Parameter:** `id` (UUID booking)
* **Response: 200 OK**
  ```json
  {
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "moves": [
      {
        "id": "01900000-0000-7000-8000-000000000099",
        "from_room_number": "101",
        "to_room_number": "305",
        "move_date": "2026-10-03",
        "reason_category": "maintenance_defect",
        "notes": "AC kamar 101 bocor dan berisik",
        "actor_id": "staff:receptionist_01",
        "created_at": "2026-10-03T14:30:00Z"
      }
    ]
  }
  ```

---

## 3. Spesifikasi Skema Database & Migrasi (Goose 00012)

```sql
-- Migrations: 00012_stay_modification_and_room_move.sql

CREATE TABLE IF NOT EXISTS room_move_logs (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    from_room_number TEXT NOT NULL REFERENCES rooms(room_number),
    to_room_number TEXT NOT NULL REFERENCES rooms(room_number),
    move_date DATE NOT NULL DEFAULT CURRENT_DATE,
    reason_category VARCHAR(32) NOT NULL, -- maintenance_defect, noise_complaint, upgrade, guest_request
    notes TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_room_move_booking ON room_move_logs(booking_id);

-- Aturan Casbin RBAC untuk Modul Stay Modification
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'receptionist', '/api/v1/bookings/:id/room-move', 'POST'),
    ('p', 'receptionist', '/api/v1/bookings/:id/extend-stay', 'POST'),
    ('p', 'receptionist', '/api/v1/bookings/:id/room-moves', 'GET'),
    ('p', 'finance', '/api/v1/bookings/:id/room-moves', 'GET');
```
