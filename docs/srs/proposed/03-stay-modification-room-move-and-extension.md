# SRS — Proposed 03: Stay Modification: Room Move & Stay Extension
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen PRD Pasangan:** [PRD-Proposed-03](../../prd/proposed/03-stay-modification-room-move-and-extension.md)
- **Status:** Proposed / Under Review
- **Target Role:** `receptionist`, `gm_admin`, `finance`, `housekeeping`

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-STAY-01: Eksekusi Pemindahan Kamar Fisik (Room Move)
Sistem wajib menyediakan endpoint bagi resepsionis untuk memindahkan tamu berstatus `checked_in` dari satu kamar fisik ke kamar fisik lainnya:
1. Memverifikasi kamar tujuan berada dalam status `inspected`.
2. Mengubah rentang tanggal penugasan kamar lama menjadi `[check_in, CURRENT_DATE)`.
3. Menambahkan baris penugasan kamar baru di `room_assignments` dengan rentang `[CURRENT_DATE, check_out)`.
4. Mengubah status kamar lama menjadi `vacant_dirty`.
5. Mencatat riwayat ke `room_move_logs`.

### FR-STAY-02: Perpanjangan Menginap (Stay Extension)
Sistem wajib menyediakan endpoint perpanjangan malam menginap:
1. Menerima jumlah malam tambahan (`additional_nights`, minimal 1).
2. Memeriksa ketersediaan kuota inventaris (`inventory.available_rooms > 0`) untuk rentang malam tambahan.
3. Menghitung tarif malam tambahan menggunakan pricing engine.
4. Memperpanjang tanggal `check_out` pada tabel `bookings`, menambahkan record pada `reservation_room_nights`, dan memperbarui rentang `daterange` pada `room_assignments`.
5. Menghasilkan total biaya tambahan (`additional_amount_minor`).

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. POST /api/v1/bookings/{id}/room-move
* **Otorisasi:** Role `receptionist`, `gm_admin`
* **Path Parameter:** `id` (UUID booking yang sedang `checked_in`)
* **Request JSON:**
  ```json
  {
    "target_room_number": "305",
    "reason_category": "maintenance_defect",
    "notes": "AC kamar 101 bocor dan berisik, teknisi perlu 2 hari perbaikan"
  }
  ```
* **Response: 200 OK**
  ```json
  {
    "status": "ok",
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "previous_room_number": "101",
    "new_room_number": "305",
    "effective_from": "2026-10-03",
    "stay_until": "2026-10-05",
    "message": "pemindahan kamar berhasil; kamar 101 telah ditandai vacant_dirty"
  }
  ```
* **Error: 409 Conflict (`TARGET_ROOM_NOT_READY`)**
  ```json
  {
    "error": "kamar target 305 belum siap huni (status saat ini: cleaning). Mohon tunggu status inspected",
    "code": "TARGET_ROOM_NOT_READY"
  }
  ```

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
    "new_total_price_minor": 3000000,
    "payment_status": "settled"
  }
  ```
* **Error: 409 Conflict (`NO_AVAILABILITY_FOR_EXTENSION`)**
  ```json
  {
    "error": "kamar tipe Deluxe King telah habis terjual untuk tanggal perpanjangan 2026-10-06",
    "code": "NO_AVAILABILITY_FOR_EXTENSION"
  }
  ```

---

## 3. Spesifikasi Skema Database

```sql
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
```
