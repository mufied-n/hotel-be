# SRS — Proposed 04: Guest Assistance & Special Requests Management
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen PRD Pasangan:** [PRD-Proposed-04](../../prd/proposed/04-guest-assistance-and-special-requests.md)
- **Status:** Proposed / Under Review
- **Target Role:** `guest`, `receptionist`, `housekeeping`, `gm_admin`

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-REQ-01: Pengajuan Permintaan Khusus oleh Tamu (Guest Request Submission)
Sistem menyediakan endpoint bagi tamu terotentikasi untuk mengajukan satu atau lebih permintaan khusus terstruktur pada pemesanan miliknya sebelum tanggal kedatangan.

### FR-REQ-02: Penelusuran Permintaan oleh Tamu (Guest Request Inquiries)
Tamu dapat memeriksa status pemenuhan setiap permintaan khusus melalui portal My Bookings (termasuk catatan persetujuan dari staf hotel).

### FR-REQ-03: Antrean Tugas Staf (Departmental Task Queue)
Staf Front Desk dan Housekeeping dapat melihat daftar permintaan khusus aktif terfilter berdasarkan departemen yang bertanggung jawab (`front_desk` atau `housekeeping`) dan tanggal kedatangan tamu.

### FR-REQ-04: Pembaruan Status Pemenuhan (Fulfillment Status Transition)
Staf dapat memperbarui status permintaan:
- `pending` ➔ `acknowledged` (diterima dan dicatat)
- `acknowledged` ➔ `fulfilled` (sudah disiapkan di kamar / disetujui)
- `acknowledged` / `pending` ➔ `declined` (ditolak dengan alasan, misal: kamar lantai tinggi penuh)

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. POST /api/v1/guest/bookings/{id}/special-requests
* **Otorisasi:** Sesi Tamu Valid (`requireGuestSession`), Anti-IDOR verifikasi kepemilikan email.
* **Path Parameter:** `id` (UUID booking)
* **Request JSON:**
  ```json
  {
    "category": "celebration_setup",
    "description": "Ulang tahun pernikahan ke-5, mohon dekorasi handuk angsa dan kartu ucapan",
    "target_time": "15:00"
  }
  ```
* **Response: 201 Created**
  ```json
  {
    "id": "req_01900000_5678",
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "category": "celebration_setup",
    "department": "housekeeping",
    "status": "pending",
    "created_at": "2026-10-03T11:00:00Z"
  }
  ```

### 2.2. GET /api/v1/guest/bookings/{id}/special-requests
* **Otorisasi:** Sesi Tamu Valid (`requireGuestSession`)
* **Response: 200 OK**
  ```json
  {
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "requests": [
      {
        "id": "req_01900000_5678",
        "category": "celebration_setup",
        "description": "Ulang tahun pernikahan ke-5, mohon dekorasi handuk angsa",
        "department": "housekeeping",
        "status": "acknowledged",
        "staff_notes": "Akan disiapkan sebelum tamu check-in pukul 14:00",
        "created_at": "2026-10-03T11:00:00Z"
      }
    ]
  }
  ```

### 2.3. GET /api/v1/front-desk/special-requests
* **Otorisasi:** Role `receptionist`, `housekeeping`, `gm_admin`
* **Query Parameters:**
  * `department` (optional: `front_desk`, `housekeeping`)
  * `status` (optional: `pending`, `acknowledged`, `fulfilled`, `declined`)
  * `check_in_date` (optional: `YYYY-MM-DD`)
* **Response: 200 OK** (Daftar antrean tugas staf).

### 2.4. PUT /api/v1/front-desk/special-requests/{id}/status
* **Otorisasi:** Role `receptionist`, `housekeeping`, `gm_admin`
* **Request JSON:**
  ```json
  {
    "to_status": "fulfilled",
    "staff_notes": "Handuk angsa dan bunga mawar telah disiapkan di kamar 301"
  }
  ```
* **Response: 200 OK**

---

## 3. Spesifikasi Skema Database

```sql
CREATE TABLE IF NOT EXISTS booking_special_requests (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    category VARCHAR(32) NOT NULL CHECK (category IN (
        'early_arrival', 'late_departure', 'high_floor', 'quiet_room',
        'bed_type', 'celebration_setup', 'baby_crib', 'dietary_allergy', 'other'
    )),
    department VARCHAR(16) NOT NULL CHECK (department IN ('front_desk', 'housekeeping')),
    description TEXT NOT NULL,
    target_time VARCHAR(8) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged', 'fulfilled', 'declined')),
    staff_notes TEXT NOT NULL DEFAULT '',
    handled_by VARCHAR(64),
    handled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_special_requests_booking ON booking_special_requests(booking_id);
CREATE INDEX IF NOT EXISTS idx_special_requests_dept_status ON booking_special_requests(department, status);
```
