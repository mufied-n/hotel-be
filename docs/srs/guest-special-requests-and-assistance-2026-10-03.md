# SRS — Guest Special Requests & Stay Assistance Desk
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Status:** Approved / Specification Freeze
- **Target Role:** `guest`, `receptionist`, `housekeeping`, `gm_admin`
- **Dokumen PRD Pasangan:** [PRD Guest Assistance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/guest-special-requests-and-assistance-2026-10-03.md)
- **Dokumen Tech Architecture:** [Tech Architecture Guest Assistance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-special-requests-architecture-2026-10-03.md)

---

## 1. Ruang Lingkup & Kebutuhan Fungsional

### FR-REQ-01: Pengajuan Permintaan Khusus oleh Tamu (Guest Request Submission)
Sistem menyediakan antarmuka bagi tamu terotentikasi untuk mengajukan satu atau lebih permintaan khusus terstruktur pada pemesanan miliknya sebelum tanggal kedatangan.
- **Validasi Kategori:** `early_arrival`, `late_departure`, `high_floor`, `quiet_room`, `bed_type`, `celebration_setup`, `baby_crib`, `dietary_allergy`, `other`.
- **Auto-Routing:** Sistem secara deterministik memetakan departemen:
  - `housekeeping`: `celebration_setup`, `baby_crib`, `quiet_room`, `high_floor`, `bed_type`.
  - `front_desk`: `early_arrival`, `late_departure`, `dietary_allergy`, `other`.

### FR-REQ-02: Penelusuran Permintaan oleh Tamu (Guest Request Inquiries)
Tamu dapat memeriksa status pemenuhan dan catatan tindak lanjut staf untuk setiap permintaan khusus melalui portal My Bookings.
- **Anti-IDOR:** Tamu hanya dapat mengakses permintaan dari booking miliknya sendiri (`session.email == booking.guest_email`). Akses ke booking milik tamu lain menghasilkan HTTP 404 Not Found.

### FR-REQ-03: Antrean Tugas Staf Lintas Departemen (Departmental Task Queue)
Staf Meja Depan dan Tata Graha (*Front Desk & Housekeeping*) dapat melihat daftar permintaan khusus aktif terfilter berdasarkan departemen yang bertanggung jawab (`front_desk` atau `housekeeping`) serta status pemenuhan.

### FR-REQ-04: Transisi Status Pemenuhan (Fulfillment Status Machine)
Staf dapat memperbarui status pemenuhan:
- `pending` ➔ `acknowledged` (diterima dan dijadwalkan).
- `acknowledged` ➔ `fulfilled` (sudah disiapkan di kamar / disetujui).
- `pending` / `acknowledged` ➔ `declined` (ditolak dengan alasan wajib di `staff_notes`).
- Mencatat identitas staf (`handled_by`) dan waktu pembaruan (`handled_at`).

### FR-REQ-05: Penutupan Celah PII BE-R02 (Public DTO Masking)
Kolom `special_requests` pada DTO publik `GET /api/v1/bookings/{id}` disamarkan (*masked*) bagi pemanggil anonim tanpa token tamu privat (`X-Guest-Token`) atau sesi staf hotel.

---

## 2. Spesifikasi Kontrak HTTP RESTful

### 2.1. POST /api/v1/guest/bookings/{id}/special-requests
* **Otorisasi:** Sesi Tamu Valid (`requireGuestSession`).
* **Path Parameter:** `id` (UUID booking).
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
    "id": "019241b7-7200-7c22-9011-000000000001",
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "category": "celebration_setup",
    "department": "housekeeping",
    "description": "Ulang tahun pernikahan ke-5, mohon dekorasi handuk angsa dan kartu ucapan",
    "target_time": "15:00",
    "status": "pending",
    "staff_notes": "",
    "created_at": "2026-10-03T11:00:00Z",
    "updated_at": "2026-10-03T11:00:00Z"
  }
  ```
* **Error Response:**
  - `400 Bad Request`: Kategori tidak valid atau deskripsi kosong (`INVALID_CATEGORY`, `EMPTY_DESCRIPTION`).
  - `404 Not Found`: Reservasi tidak ditemukan atau milik tamu lain (Anti-IDOR).
  - `503 Service Unavailable`: Feature flag `ff_guest_special_requests` nonaktif.

### 2.2. GET /api/v1/guest/bookings/{id}/special-requests
* **Otorisasi:** Sesi Tamu Valid (`requireGuestSession`).
* **Path Parameter:** `id` (UUID booking).
* **Response: 200 OK**
  ```json
  {
    "booking_id": "01900000-0000-7000-8000-000000000001",
    "requests": [
      {
        "id": "019241b7-7200-7c22-9011-000000000001",
        "category": "celebration_setup",
        "department": "housekeeping",
        "description": "Ulang tahun pernikahan ke-5, mohon dekorasi handuk angsa dan kartu ucapan",
        "target_time": "15:00",
        "status": "acknowledged",
        "staff_notes": "Akan disiapkan sebelum tamu check-in pukul 14:00",
        "handled_by": "receptionist",
        "handled_at": "2026-10-03T11:30:00Z",
        "created_at": "2026-10-03T11:00:00Z",
        "updated_at": "2026-10-03T11:30:00Z"
      }
    ]
  }
  ```

### 2.3. GET /api/v1/front-desk/special-requests
* **Otorisasi:** Role Staf (`receptionist`, `housekeeping`, `gm_admin`).
* **Query Parameters:**
  - `department` (opsional: `front_desk`, `housekeeping`)
  - `status` (opsional: `pending`, `acknowledged`, `fulfilled`, `declined`)
  - `booking_id` (opsional: UUID booking)
* **Response: 200 OK**
  ```json
  {
    "items": [
      {
        "id": "019241b7-7200-7c22-9011-000000000001",
        "booking_id": "01900000-0000-7000-8000-000000000001",
        "guest_name": "Budi Santoso",
        "category": "celebration_setup",
        "department": "housekeeping",
        "description": "Ulang tahun pernikahan ke-5, mohon dekorasi handuk angsa dan kartu ucapan",
        "target_time": "15:00",
        "status": "pending",
        "staff_notes": "",
        "created_at": "2026-10-03T11:00:00Z"
      }
    ],
    "total": 1
  }
  ```

### 2.4. PUT /api/v1/front-desk/special-requests/{id}/status
* **Otorisasi:** Role Staf (`receptionist`, `housekeeping`, `gm_admin`).
* **Path Parameter:** `id` (UUID permintaan khusus).
* **Request JSON:**
  ```json
  {
    "to_status": "fulfilled",
    "staff_notes": "Handuk angsa dan mawar telah tertata di kamar 301"
  }
  ```
* **Response: 200 OK**
  ```json
  {
    "id": "019241b7-7200-7c22-9011-000000000001",
    "status": "fulfilled",
    "staff_notes": "Handuk angsa dan mawar telah tertata di kamar 301",
    "handled_by": "housekeeping",
    "handled_at": "2026-10-03T12:00:00Z"
  }
  ```
* **Error Response:**
  - `400 Bad Request`: `staff_notes` kosong saat status `declined`, atau transisi status tidak valid (`INVALID_STATUS_TRANSITION`).
  - `404 Not Found`: ID permintaan khusus tidak ditemukan.

---

## 3. Spesifikasi Skema Database PostgreSQL

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
    handled_by VARCHAR(64) NOT NULL DEFAULT '',
    handled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_special_requests_booking ON booking_special_requests(booking_id);
CREATE INDEX IF NOT EXISTS idx_special_requests_dept_status ON booking_special_requests(department, status);
```
