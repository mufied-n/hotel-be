# Software Requirements Specification (SRS) — Penyeragaman Batas Payload & Skema Error API (BE-R18)

**Nomor Dokumen:** SRS-PULANG-BE-R18-2026-10-04  
**Target Rilis:** v1.0.0-rc1  
**Status:** Approved  
**Author:** AI Engineering & Architecture Agent  
**Terkait:** [`BE-R18`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md), [`PRD-PULANG-BE-R18`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/payload-boundary-and-unified-error-r18-2026-10-04.md)  

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Pembatasan Ukuran Request Body (Body Size Limit Middleware & Handlers)
- **Deskripsi:** Seluruh endpoint HTTP yang menerima request body (POST, PUT, PATCH) wajib dibatasi konsumsi memorinya maksimal 1 MB (1,048,576 bytes) menggunakan `http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)`.
- **Perilaku:**
  - Middleware `BodySizeLimit(maxBytes int64)` dipasang pada router level API.
  - Handler pembaca `io.ReadAll` (`createBooking`, `xenditWebhook`) secara eksplisit membungkus request body dengan `http.MaxBytesReader`.
  - Handler pembaca `json.UnmarshalRead` / decoder menangani `*http.MaxBytesError` atau string `"request body too large"`.
  - Jika body melebihi 1 MB, server wajib menolak request dengan status **HTTP 413 Payload Too Large**, machine error code `PAYLOAD_TOO_LARGE`.

### FR-02: Validasi Panjang Header `Idempotency-Key` (1–64 Karakter)
- **Deskripsi:** Header `Idempotency-Key` pada endpoint transaksi pembuatan reservasi (`POST /api/v1/bookings`) wajib divalidasi panjangnya sebelum pemrosesan store atau reservasi atomik.
- **Perilaku:**
  - Jika header `Idempotency-Key` dikirim dan setelah di-trim panjangnya $> 64$ karakter, request wajib langsung ditolak tanpa menyentuh cache, DB, atau inventory engine.
  - Respons penolakan: **HTTP 400 Bad Request**, machine error code `INVALID_IDEMPOTENCY_KEY`, pesan `"idempotency key must be between 1 and 64 characters"`.

### FR-03: Unifikasi Kontrak Error Dual-Shape Kompatibel
- **Deskripsi:** Seluruh respons error dari API (baik modul booking RFC 7807 problem details maupun portal tamu `guest_auth.go`) wajib memuat pasangan field yang konsisten untuk interoperabilitas frontend:
  1. `code` (string): Machine-readable error code unik (misal: `INVALID_PAYLOAD`, `PAYLOAD_TOO_LARGE`, `BOOKING_NOT_FOUND`, `INVALID_JSON`).
  2. `error` (string): Field error legacy; pada modul portal tamu berisi *machine code* (misal: `BOOKING_NOT_FOUND`), pada problem details berisi fallback pesan/kode.
  3. `message` (string): Pesan deskriptif ramah pengguna (human-readable).
  4. `detail` (string): Pesan deskriptif ramah pengguna (human-readable), identik dengan `message`.
  5. `status` (integer): HTTP status code numerik yang identik dengan header HTTP response.
  6. `title` (string, optional): HTTP status text (misal: `Bad Request`, `Payload Too Large`).

### FR-04: Penanganan JSON Malformed yang Seragam
- **Deskripsi:** Setiap payload JSON yang rusak secara sintaksis (misal tanda kurung tidak lengkap, tipe data primitif salah total) wajib ditolak pada lapisan transport sebelum menyentuh domain layer.
- **Respons:** **HTTP 400 Bad Request**, machine error code `INVALID_JSON`, pesan `"body JSON tidak valid"` atau `"Format request tidak valid."`.

### FR-05: Perlindungan Kebocoran Error Internal (Anti-Leak Internal/SQL)
- **Deskripsi:** Error basis data (misal syntax error, connection pool exhausted, duplicate key database raw) dilarang bocor ke client.
- **Respons:** Server membungkus error dengan pesan umum `"terjadi kesalahan internal"` atau log internal, mengembalikan **HTTP 500 Internal Server Error** dengan code `INTERNAL_ERROR`.

---

## 2. Spesifikasi Kontrak HTTP JSON Error

### Format Dual-Shape Problem Details & Portal Tamu

```json
{
  "code": "PAYLOAD_TOO_LARGE",
  "error": "request body melebihi batas 1MB",
  "message": "request body melebihi batas 1MB",
  "detail": "request body melebihi batas 1MB",
  "title": "Payload Too Large",
  "status": 413
}
```

```json
{
  "code": "INVALID_IDEMPOTENCY_KEY",
  "error": "idempotency key must be between 1 and 64 characters",
  "message": "idempotency key must be between 1 and 64 characters",
  "detail": "idempotency key must be between 1 and 64 characters",
  "title": "Bad Request",
  "status": 400
}
```

---

## 3. Matriks Error Code & Status HTTP

| Error Code | HTTP Status | Pemicu | Target Handler |
| :--- | :--- | :--- | :--- |
| `PAYLOAD_TOO_LARGE` | 413 | Request body melampaui 1 MB (1,048,576 byte). | Seluruh endpoint JSON (`POST`, `PUT`, `PATCH`). |
| `INVALID_IDEMPOTENCY_KEY` | 400 | Header `Idempotency-Key` memiliki panjang > 64 karakter. | `POST /api/v1/bookings`. |
| `INVALID_JSON` | 400 | Payload body bukan sintaks JSON valid. | Seluruh endpoint penerima JSON. |
| `INVALID_BODY` | 400 | Gagal membaca body I/O (selain max bytes). | `POST /api/v1/bookings`, `POST /api/v1/webhooks/xendit`. |
| `INTERNAL_ERROR` | 500 | Kesalahan internal server tanpa membocorkan SQL. | Seluruh endpoint. |
