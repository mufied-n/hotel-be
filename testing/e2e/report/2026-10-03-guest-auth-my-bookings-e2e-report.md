# End-to-End (E2E) Test Execution Report
# Akses Tamu, Manajemen Sesi & Booking Saya (F02 & F03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal & Waktu Eksekusi:** 3 Oktober 2026, 10:27 WIB
- **Target Pengujian:** Akses Tamu Passwordless (Email OTP via Resend), Token Sesi Kriptografis, Riwayat Booking Privat, dan Proteksi IDOR (UU PDP No. 27/2022 & OWASP ASVS V2/V3).
- **Hasil Akhir:** **PASS 100% (36 dari 36 Skenario E2E Lulus)**.

---

## 1. Lingkungan Pengujian (Test Environment)

| Komponen | Spesifikasi / Konfigurasi |
| :--- | :--- |
| **Sistem Operasi** | Linux 6.6 / x86_64 |
| **Bahasa & Runtime** | Go 1.24.0 linux/amd64 |
| **Database** | PostgreSQL 18.0 Container (`booking_test`, migration up to version 8) |
| **Queue / Cache** | Valkey 8.0 Container (Redis compatible) |
| **Runner Skrip** | `testing/e2e/script/e2e_runner_test.go` & `guest_auth_my_bookings_e2e.sh` |

---

## 2. Ringkasan Eksekusi Skenario Baru (E2E-29 s/d E2E-36)

| ID Skenario | Deskripsi Pengujian | HTTP Method & Path | Status Code | Hasil |
| :--- | :--- | :--- | :--- | :--- |
| **E2E-29** | Permintaan tantangan kode OTP dengan email tamu valid | `POST /api/v1/auth/guest/challenge` | `200 OK` | **PASS** |
| **E2E-30** | Verifikasi kode OTP 6-digit yang benar, penerbitan token sesi & Cookie | `POST /api/v1/auth/guest/verify` | `200 OK` | **PASS** |
| **E2E-31** | Inspeksi data profil sesi aktif tamu | `GET /api/v1/auth/guest/me` | `200 OK` | **PASS** |
| **E2E-32** | Akses daftar seluruh reservasi milik email tamu terotentikasi (*My Bookings*) | `GET /api/v1/guest/bookings?status=all` | `200 OK` | **PASS** |
| **E2E-33** | Akses detail pemesanan milik sendiri dengan allowed actions | `GET /api/v1/guest/bookings/bk-e2e-001` | `200 OK` | **PASS** |
| **E2E-34** | **Mitigasi IDOR**: Tamu mencoba mengakses booking ID milik orang lain | `GET /api/v1/guest/bookings/bk-other-user` | `404 Not Found` | **PASS** |
| **E2E-35** | Logout tamu mencabut sesi secara permanen (panggilan ulang ke `/me` ditolak) | `POST /api/v1/auth/guest/logout` $\rightarrow$ `GET /me` | `200 OK` $\rightarrow$ `401 Unauthorized` | **PASS** |
| **E2E-36** | Dispatch email OTP resmi bermerek Pulang ke Uttara via Resend REST API | Mock Resend REST Server | `200 OK` | **PASS** |

---

## 3. Bukti Payload Nyata Request & Response

### A. E2E-29: Request Challenge OTP
* **Request:**
  ```http
  POST /api/v1/auth/guest/challenge HTTP/1.1
  Content-Type: application/json

  {"email": "rian@example.com"}
  ```
* **Response (HTTP 200 OK):**
  ```json
  {
    "cooldown_seconds": 60,
    "message": "Jika email terdaftar atau valid, kode verifikasi 6 digit telah dikirimkan ke email Anda.",
    "status": "ok"
  }
  ```

### B. E2E-30: Verify Challenge OTP & Session Issuance
* **Request:**
  ```http
  POST /api/v1/auth/guest/verify HTTP/1.1
  Content-Type: application/json

  {"email": "rian@example.com", "code": "654321"}
  ```
* **Response (HTTP 200 OK):**
  ```http
  Set-Cookie: guest_session=gst_sess_38f2a...; Path=/; Expires=Sun, 04 Oct 2026 10:27:01 GMT; HttpOnly; SameSite=Lax
  Content-Type: application/json

  {
    "email": "rian@example.com",
    "expires_at": "2026-10-04T10:27:01Z",
    "token": "gst_sess_38f2a58b..."
  }
  ```

### C. E2E-33 & E2E-34: Detail Pemesanan & Pembuktian Proteksi IDOR (UU PDP No. 27/2022)
* **Pemesanan Sah Milik Tamu (`bk-e2e-001`):**
  * `GET /api/v1/guest/bookings/bk-e2e-001`
  * **HTTP 200 OK:** Mengembalikan data reservasi lengkap, nama, email, nomor telepon, dan `allowed_actions: {"can_download_receipt": true}`.
* **Percobaan Akses Reservasi Tamu Lain (`bk-other-user`):**
  * `GET /api/v1/guest/bookings/bk-other-user`
  * **HTTP 404 Not Found:**
    ```json
    {
      "error": "BOOKING_NOT_FOUND",
      "message": "Pemesanan tidak ditemukan atau Anda tidak memiliki akses ke pemesanan ini."
    }
    ```
  * Menolak konfirmasi eksistensi booking orang lain, mematuhi prinsip *least-privilege* dan kerahasiaan data pribadi.

---

## 4. Kesimpulan Verifikasi

1. Seluruh 36 skenario E2E (mulai dari siklus awal reservasi, hold, pembayaran Xendit, outbox Resend, hingga akses tamu passwordless dan Booking Saya) lulus 100%.
2. Fitur F02 & F03 telah terintegrasi secara modular, memenuhi prinsip Ponytail (anti-overengineering), dan siap untuk tahap komitmen git.
