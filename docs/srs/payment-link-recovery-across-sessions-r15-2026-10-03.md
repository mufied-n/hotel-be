# Software Requirements Specification (SRS) — Pemulihan Tautan Pembayaran Lintas Sesi (BE-R15)

**Nomor Dokumen:** SRS-PULANG-BE-R15-2026-10-03  
**Target Rilis:** v1.0.0-rc1  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** [`BE-R15`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md), PRD-PULANG-BE-R15-2026-10-03  

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Endpoint Pemulihan Tautan Pembayaran (`GET /api/v1/bookings/:id/payment`)
- **Deskripsi:** Mengembalikan informasi pembayaran aktif dan tautan checkout gateway yang sah untuk reservasi berstatus `pending`.
- **Pemeriksaan Otorisasi (IDOR Guard):**
  - Staf hotel (Role bukan `guest`), ATAU
  - Tamu dengan token kepemilikan valid (`X-Guest-Token: <token>` cocok dengan `bookings.guest_token`), ATAU
  - Tamu dengan sesi aktif (`guest_session`) dengan email yang cocok dengan `bookings.guest_email`.
  - Jika otorisasi gagal: kembalikan HTTP 404 `BOOKING_NOT_FOUND` (untuk mencegah eksploitasi enumerasi ID) atau HTTP 403 `FORBIDDEN`.
- **Validasi State Machine & Batas Waktu Hold:**
  - Jika status reservasi bukan `pending`: kembalikan HTTP 409 Conflict dengan kode `BOOKING_NOT_PENDING`.
  - Jika batas waktu hold telah terlampaui (`time.Now() >= *expires_at`): kembalikan HTTP 410 Gone dengan kode `HOLD_EXPIRED`.
- **Idempotensi Penerbitan:**
  - Sistem mencari percobaan pembayaran terbaru dari tabel `payment_attempts` yang memiliki atribut `payment_url` pada kolom `payload`.
  - Jika ditemukan: tautan pembayaran eksisting dikembalikan langsung tanpa memanggil gateway eksternal (*zero duplicate invoices*).
  - Jika belum ada URL (misal karena timeout sebelumnya): sistem memanggil gateway secara terkontrol, menyimpan URL ke attempt, dan mengembalikannya ke klien.

### FR-02: Konsistensi DTO Portal Tamu (`GET /api/v1/guest/bookings/:id`)
- **Deskripsi:** Menambahkan field `payment_url` pada skema `BookingDetail`.
- **Aturan Pengisian:**
  - Jika `allowed_actions.can_pay == true`: resolusi URL pembayaran aktif dilakukan dan disematkan pada respons JSON.
  - Jika `allowed_actions.can_pay == false`: field `payment_url` dikosongkan (`""` atau `omitempty`).

### FR-03: Endpoint Shortcut Portal Tamu (`GET /api/v1/guest/bookings/:id/payment`)
- **Deskripsi:** Menyediakan endpoint pemulihan pembayaran khusus di dalam rute terproteksi sesi tamu (`guestGroup`).
- **Verifikasi:** Memeriksa kepemilikan email secara *case-insensitive* sesuai standar `BE-R05`.

---

## 2. Kontrak HTTP RESTful

### Endpoint 1: Pemulihan Tautan Pembayaran Reservasi

```http
GET /api/v1/bookings/{id}/payment
X-Guest-Token: gst_01900000-0000-7000-8000-000000000001
```

#### Respons Sukses (HTTP 200 OK)
```json
{
  "booking_id": "01900000-0000-7000-8000-000000000001",
  "status": "pending",
  "payment_url": "https://checkout.xendit.co/web/inv_mock_01900000-0000-7000-8000-000000000001",
  "provider_reference": "inv_mock_01900000-0000-7000-8000-000000000001",
  "amount_minor": 1100000,
  "currency": "IDR",
  "expires_at": "2026-11-02T14:30:00Z"
}
```

#### Respons Kesalahan (Error Responses)
- **HTTP 404 Not Found (IDOR / Booking Tidak Ditemukan):**
  ```json
  {
    "status": 404,
    "code": "BOOKING_NOT_FOUND",
    "title": "Not Found",
    "detail": "booking tidak ditemukan atau Anda tidak memiliki akses"
  }
  ```
- **HTTP 409 Conflict (Bukan Status Pending):**
  ```json
  {
    "status": 409,
    "code": "BOOKING_NOT_PENDING",
    "title": "Conflict",
    "detail": "pembayaran tidak dapat dilanjutkan karena reservasi tidak berstatus pending"
  }
  ```
- **HTTP 410 Gone (Hold Kedaluwarsa):**
  ```json
  {
    "status": 410,
    "code": "HOLD_EXPIRED",
    "title": "Gone",
    "detail": "batas waktu pembayaran reservasi telah kedaluwarsa, kamar telah dilepas ke publik"
  }
  ```

---

### Endpoint 2: Detail Pemesanan Tamu (Guest Portal)

```http
GET /api/v1/guest/bookings/{id}
Cookie: guest_session=gst_sess_token_abc123
```

#### Respons Sukses (HTTP 200 OK)
```json
{
  "booking": {
    "id": "01900000-0000-7000-8000-000000000001",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "room_type_name": "Superior King Room",
    "check_in": "2026-11-10",
    "check_out": "2026-11-12",
    "num_rooms": 1,
    "num_guests": 2,
    "status": "pending",
    "total_price_minor": 1100000,
    "currency": "IDR",
    "guest_name": "Budi Santoso",
    "guest_email": "budi@example.com",
    "guest_phone": "+628123456789",
    "payment_url": "https://checkout.xendit.co/web/inv_mock_01900000",
    "expires_at": "2026-10-10T14:30:00Z",
    "allowed_actions": {
      "can_pay": true,
      "can_cancel": true,
      "can_download_receipt": false,
      "can_request_assistance": true
    }
  },
  "allowed_actions": {
    "can_pay": true,
    "can_cancel": true,
    "can_download_receipt": false,
    "can_request_assistance": true
  }
}
```
