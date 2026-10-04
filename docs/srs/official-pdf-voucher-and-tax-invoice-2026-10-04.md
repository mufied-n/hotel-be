# Software Requirements Specification (SRS)
# Official PDF Confirmation Voucher & PBJT Tax Invoice Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `SRS-F04-PDF-VOUCHER-INVOICE-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - PRD: [`docs/prd/official-pdf-voucher-and-tax-invoice-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/official-pdf-voucher-and-tax-invoice-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/official-pdf-voucher-and-tax-invoice-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/official-pdf-voucher-and-tax-invoice-architecture-2026-10-04.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

* **FR-DOC-01 (Voucher PDF Generation):** Sistem wajib menyediakan endpoint untuk menghasilkan dan mengalirkan berkas PDF Booking Voucher resmi berukuran A4 untuk reservasi yang berstatus `CONFIRMED` atau `CHECKED_IN`.
* **FR-DOC-02 (Tax Invoice PDF Generation):** Sistem wajib menyediakan endpoint untuk menghasilkan dan mengalirkan berkas Faktur Pajak Daerah Resmi (PBJT Jasa Perhotelan 10% dan Service Charge 10%) yang memuat NPWPD hotel, nomor seri faktur unik, dan rincian DPP.
* **FR-DOC-03 (Tamper-Resistant QR Code):** Setiap dokumen voucher wajib memuat QR Code berukuran minimal $150 \times 150\text{ px}$ yang berisi payload bertanda tangan kriptografis HMAC-SHA256 untuk verifikasi meja depan.
* **FR-DOC-04 (Voucher Verification Endpoint):** Sistem wajib menyediakan endpoint bagi peran resepsionis meja depan (`receptionist`, `gm_admin`) untuk memvalidasi token QR Code dan menampilkan detail reservasi secara instan.
* **FR-DOC-05 (Ownership & Access Guard):** Endpoint dokumen PDF tamu wajib memverifikasi kepemilikan reservasi melalui token sesi tamu (`gst_sess_`) atau header token privat `X-Guest-Token`. Panggilan tanpa hak kepemilikan yang sah wajib ditolak `401` / `403`.
* **FR-DOC-06 (Lifecycle State Guard):** Faktur pajak (`invoice.pdf`) hanya boleh diunduh jika status reservasi telah lunas (`PAID`). Jika reservasi masih pending atau sudah dibatalkan, sistem wajib mengembalikan status `400` atau `409` dengan kode error yang jelas.
* **FR-DOC-07 (In-Memory Streaming):** Pembangkitan PDF tidak boleh menggunakan file sementara di disk (*zero disk temp files*), melainkan dialirkan langsung dari memori via `io.Writer` ke HTTP writer Gin.

---

## 2. Spesifikasi Antarmuka HTTP (Contracts)

---

### 2.1 GET `/api/v1/guest/bookings/:id/voucher.pdf`
Mengunduh atau menampilkan voucher resmi pemesanan hotel dalam format PDF.

* **Headers:**
  - `Authorization: Bearer gst_sess_<token>` (atau `X-Guest-Token: <token>`)
* **Path Parameters:**
  - `id` (UUID, required): ID reservasi booking.
* **Response `200 OK`:**
  - `Content-Type: application/pdf`
  - `Content-Disposition: inline; filename="Voucher-PKU-202610-8849.pdf"`
  - `Content-Length: <panjang byte berkas>`
  - `Cache-Control: private, no-cache, no-store, must-revalidate`
  - `X-Request-Id: <uuid>`
  - *Binary payload:* Stream berkas PDF standar A4.

---

### 2.2 GET `/api/v1/guest/bookings/:id/invoice.pdf`
Mengunduh faktur pajak resmi PBJT (Kabupaten Sleman) dan Service Charge dalam format PDF.

* **Headers:**
  - `Authorization: Bearer gst_sess_<token>` (atau `X-Guest-Token: <token>`)
* **Path Parameters:**
  - `id` (UUID, required): ID reservasi booking.
* **Response `200 OK`:**
  - `Content-Type: application/pdf`
  - `Content-Disposition: inline; filename="Invoice-PKU-202610-8849.pdf"`
  - `Content-Length: <panjang byte berkas>`
  - `Cache-Control: private, no-cache, no-store, must-revalidate`
  - `X-Request-Id: <uuid>`
  - *Binary payload:* Stream berkas PDF resmi faktur pajak hotel A4.

---

### 2.3 GET `/api/v1/front-desk/verify-voucher`
Memvalidasi keaslian voucher via token QR code yang dipindai kamera scanner meja depan.

* **Headers:**
  - `Authorization: Bearer stf_<token>` (Role: `receptionist`, `gm_admin`)
* **Query Parameters:**
  - `ref` (string, required): Nomor referensi booking (misal: `PKU-202610-8849`).
  - `token` (string, required): Token tanda tangan digital HMAC-SHA256 dari QR Code.
* **Response `200 OK`:**
  ```json
  {
    "status": "VALID",
    "reference": "PKU-202610-8849",
    "booking_id": "01900000-0000-7000-8000-000000000010",
    "guest_name": "Dr. Bambang Kusumo",
    "room_type_name": "Deluxe King Bay Window",
    "check_in": "2026-10-15",
    "check_out": "2026-10-17",
    "stay_status": "CONFIRMED",
    "payment_status": "PAID",
    "verified_at": "2026-10-15T13:45:00+07:00",
    "ready_for_checkin": true
  }
  ```

---

## 3. Spesifikasi Kode Kesalahan (RFC 7807 Problem Details)

Jika permintaan dokumen PDF gagal atau melanggar aturan bisnis, server mengembalikan format error dual-shape:

| HTTP Status | Error Code (`code`) | Deskripsi Masalah |
| :---: | :--- | :--- |
| `400` | `INVOICE_NOT_AVAILABLE` | Faktur belum dapat diterbitkan karena reservasi belum berstatus lunas (`PAID`). |
| `400` | `INVALID_QR_TOKEN` | Token tanda tangan digital pada QR Code tidak valid atau telah dimanipulasi. |
| `401` | `AUTHENTICATION_REQUIRED` | Token sesi tamu atau token staf tidak disertakan atau telah kedaluwarsa. |
| `403` | `FORBIDDEN_RESOURCE` | Tamu mencoba mengakses voucher atau faktur milik reservasi tamu lain. |
| `404` | `BOOKING_NOT_FOUND` | ID booking yang diminta tidak ditemukan di database hotel. |
| `500` | `PDF_RENDER_FAILED` | Terjadi kesalahan internal saat merender tata letak dokumen PDF. |

Contoh Respon Error `400 Bad Request`:
```json
{
  "type": "https://httpstatuses.com/400",
  "title": "Bad Request",
  "status": 400,
  "detail": "official tax invoice is only available for confirmed and paid reservations",
  "error": "official tax invoice is only available for confirmed and paid reservations",
  "code": "INVOICE_NOT_AVAILABLE",
  "request_id": "993a4b12c8ef40a7b112001188339922"
}
```
