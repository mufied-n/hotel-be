# Software Requirements Specification (SRS)
# Akses Tamu, Manajemen Sesi & Booking Saya (F02 & F03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** **APPROVED FOR IMPLEMENTATION**
- **Dokumen Teknis Terkait:** [`docs/tech/guest-access-my-bookings-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-access-my-bookings-architecture-2026-10-03.md)
- **PRD Rujukan:** [`docs/prd/guest-access-my-bookings-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/guest-access-my-bookings-2026-10-03.md)

---

## 1. Spesifikasi Kebutuhan Fungsional (Functional Requirements)

### FR-01: Permintaan Kode Tantangan OTP (Request Challenge)
* **Method & Path:** `POST /api/v1/auth/guest/challenge`
* **Deskripsi:** Menghasilkan kode OTP 6-digit acak aman, menyimpannya dalam bentuk SHA-256 hash dengan masa kedaluwarsa 10 menit, dan mengirimkannya ke email tamu melalui Resend Notifier.
* **Headers:** `Content-Type: application/json`
* **Request Body:**
  ```json
  {
    "email": "tamu@example.com"
  }
  ```
* **Response (HTTP 200 OK):**
  ```json
  {
    "status": "ok",
    "message": "Jika email terdaftar atau valid, kode verifikasi 6 digit telah dikirimkan ke email Anda.",
    "cooldown_seconds": 60
  }
  ```
* **Error Response (HTTP 400 Bad Request):**
  ```json
  {
    "error": "INVALID_EMAIL",
    "message": "Format alamat email tidak valid."
  }
  ```
* **Error Response (HTTP 429 Too Many Requests):**
  ```json
  {
    "error": "RATE_LIMIT_EXCEEDED",
    "message": "Harap tunggu sebelum meminta kode verifikasi baru."
  }
  ```

---

### FR-02: Verifikasi Kode OTP & Pembuatan Sesi (Verify Challenge)
* **Method & Path:** `POST /api/v1/auth/guest/verify`
* **Deskripsi:** Memvalidasi kode OTP yang dimasukkan oleh tamu. Jika sah, sistem menandai OTP sebagai terpakai (*consumed*), menerbitkan token sesi acak 32-byte hex, menyimpannya dalam bentuk SHA-256 hash di database dengan masa berlaku 24 jam (*idle*) dan 7 hari (*absolute*), serta mengembalikan data sesi.
* **Headers:** `Content-Type: application/json`
* **Request Body:**
  ```json
  {
    "email": "tamu@example.com",
    "code": "847291"
  }
  ```
* **Response (HTTP 200 OK):**
  ```json
  {
    "token": "gst_sess_3f9a7b2c1d8e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a",
    "email": "tamu@example.com",
    "expires_at": "2026-10-04T10:15:00Z"
  }
  ```
* **Error Responses:**
  * `400 Bad Request`: `{"error": "INVALID_INPUT", "message": "Email dan kode OTP 6 digit wajib diisi."}`
  * `401 Unauthorized`: `{"error": "INVALID_OR_EXPIRED_CODE", "message": "Kode verifikasi salah atau telah kedaluwarsa."}`
  * `403 Forbidden`: `{"error": "MAX_ATTEMPTS_EXCEEDED", "message": "Batas percobaan terlampaui. Silakan minta kode baru."}`

---

### FR-03: Profil Sesi Tamu Aktif (Current Guest Session)
* **Method & Path:** `GET /api/v1/auth/guest/me`
* **Deskripsi:** Menampilkan informasi sesi tamu aktif serta jumlah pemesanan terkait.
* **Headers:** `Authorization: Bearer <session_token>` ATAU `X-Guest-Session: <session_token>`
* **Response (HTTP 200 OK):**
  ```json
  {
    "email": "tamu@example.com",
    "active_bookings_count": 2,
    "last_active_at": "2026-10-03T10:20:00Z",
    "expires_at": "2026-10-04T10:15:00Z"
  }
  ```
* **Error Response (HTTP 401 Unauthorized):**
  ```json
  {
    "error": "UNAUTHORIZED",
    "message": "Sesi tamu tidak valid atau telah berakhir."
  }
  ```

---

### FR-04: Mengakhiri Sesi Tamu (Logout)
* **Method & Path:** `POST /api/v1/auth/guest/logout`
* **Deskripsi:** Mencabut (*revoke*) token sesi aktif dari database secara permanen.
* **Headers:** `Authorization: Bearer <session_token>` ATAU `X-Guest-Session: <session_token>`
* **Response (HTTP 200 OK):**
  ```json
  {
    "status": "ok",
    "message": "Sesi Anda telah berhasil diakhiri."
  }
  ```

---

### FR-05: Daftar Riwayat Pemesanan Tamu (My Bookings)
* **Method & Path:** `GET /api/v1/guest/bookings`
* **Deskripsi:** Mengambil seluruh reservasi milik email tamu yang terautentikasi, diurutkan dari yang terbaru, dengan opsi filter status.
* **Headers:** `Authorization: Bearer <session_token>` ATAU `X-Guest-Session: <session_token>`
* **Query Parameters:**
  * `status` (string, opsional): `all` (default), `upcoming`, `completed`, `cancelled`.
  * `limit` (integer, opsional, default 20, max 100).
* **Response (HTTP 200 OK):**
  ```json
  {
    "data": [
      {
        "id": "0192a6c8-1111-7000-8000-000000000001",
        "room_type_id": "0192a6c8-2222-7000-8000-000000000002",
        "room_type_name": "Deluxe Room",
        "check_in": "2026-10-10",
        "check_out": "2026-10-12",
        "num_rooms": 1,
        "num_guests": 2,
        "status": "confirmed",
        "total_price_minor": 1500000,
        "currency": "IDR",
        "created_at": "2026-10-03T09:00:00Z"
      }
    ],
    "total": 1
  }
  ```

---

### FR-06: Detail Pemesanan Privat & Allowed Actions
* **Method & Path:** `GET /api/v1/guest/bookings/{id}`
* **Deskripsi:** Mengambil detail lengkap pemesanan milik tamu, termasuk informasi PII penuh, breakdown harga, dan daftar aksi yang diizinkan (*allowed actions*).
* **Headers:** `Authorization: Bearer <session_token>` ATAU `X-Guest-Session: <session_token>`
* **Response (HTTP 200 OK):**
  ```json
  {
    "booking": {
      "id": "0192a6c8-1111-7000-8000-000000000001",
      "room_type_id": "0192a6c8-2222-7000-8000-000000000002",
      "room_type_name": "Deluxe Room",
      "check_in": "2026-10-10",
      "check_out": "2026-10-12",
      "num_rooms": 1,
      "num_guests": 2,
      "status": "confirmed",
      "total_price_minor": 1500000,
      "currency": "IDR",
      "guest_name": "Rian Anggoro",
      "guest_email": "tamu@example.com",
      "guest_phone": "+6281234567890",
      "created_at": "2026-10-03T09:00:00Z"
    },
    "allowed_actions": {
      "can_pay": false,
      "can_cancel": true,
      "can_download_receipt": true,
      "can_request_assistance": true
    }
  }
  ```
* **Error Response (HTTP 404 Not Found):**
  ```json
  {
    "error": "BOOKING_NOT_FOUND",
    "message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini."
  }
  ```
  *(Catatan IDOR: Mengakses booking milik orang lain wajib mengembalikan 404 Not Found)*.
