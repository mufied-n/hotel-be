# E2E Test Report: Payload Boundary & Unified Error Contract (BE-R18)

- **Tanggal / Waktu:** 2026-10-04 00:30:00 WIB
- **Target Gap / Refinement:** [`BE-R18`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md) (*Boundary payload dan error API belum seragam*)
- **Komponen Pengujian:**
  - [`internal/api/middleware.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go) (`BodySizeLimit`, `isMaxBytesError`, `decodeJSON`, `ProblemDetails` dengan field `Message: detail`)
  - [`internal/api/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_auth.go) (`writeGuestError` dual-shape unified error contract)
  - [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go) (`r.Use(BodySizeLimit(DefaultMaxBodyBytes))`, `createBooking` 64-char `Idempotency-Key` validation, `xenditWebhook` MaxBytesReader)
  - [`internal/api/router_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router_test.go) (`TestPayloadBoundaryAndUnifiedError_TableDriven`)
- **Lingkungan Pengujian:**
  - PostgreSQL 18 Alpine (Port 25432, container `r18-pg`)
  - Valkey 8 Alpine (Port 26379, container `r18-vk`)
  - Pulang Backend Server (Port 28080, `APP_PORT=28080`, `XENDIT_WEBHOOK_TOKEN=test-token`, `ENV=development`)
- **Skrip Eksekusi:** [`testing/e2e/script/payload_boundary_and_unified_error_r18_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/payload_boundary_and_unified_error_r18_e2e.sh)

---

## 1. Ringkasan Eksekusi

| Metrik | Nilai | Status |
| :--- | :--- | :--- |
| **Total Skenario E2E** | 8 skenario pengujian runtime | PASS |
| **Total Asersi Otomatis** | 29 asersi validasi | 100% PASS |
| **Gagal / Error** | 0 | None |
| **Durasi Eksekusi** | ~1.5 detik | Cepat & Stabil |
| **Status Database & Lingkungan** | Terisolasi & Dibersihkan | Verified |

---

## 2. Rincian Skenario & Bukti Asersi

### Skenario 1: Batas Ukuran Payload pada Pembuatan Booking (`POST /api/v1/bookings`)
- **Pengujian:** Mengirimkan payload JSON berukuran 1.2 MB (> 1 MB limit) ke endpoint checkout.
- **Hasil:**
  - Status HTTP **413 Payload Too Large** (`PASS`).
  - Kode error `"code": "PAYLOAD_TOO_LARGE"` (`PASS`).
  - Format dual-shape memuat atribut `error`, `code`, `message`, `detail`, `title`, dan `status: 413` (`PASS`).

### Skenario 2: Batas Ukuran Payload pada Penghitungan Kuotasi (`POST /api/v1/quotes`)
- **Pengujian:** Mengirimkan payload JSON berukuran 1.2 MB ke endpoint kuotasi kamar.
- **Hasil:**
  - Status HTTP **413 Payload Too Large** (`PASS`).
  - Kode error `"code": "PAYLOAD_TOO_LARGE"` (`PASS`).

### Skenario 3: Batas Ukuran Payload pada Portal Tamu (`POST /api/v1/auth/guest/challenge`)
- **Pengujian:** Mengirimkan payload berukuran 1.2 MB ke endpoint permintaan OTP.
- **Hasil:**
  - Status HTTP **413 Payload Too Large** (`PASS`).
  - Atribut `"error": "PAYLOAD_TOO_LARGE"` dan `"code": "PAYLOAD_TOO_LARGE"` hadir bersamaan untuk kompatibilitas frontend (`PASS`).

### Skenario 4: Batas Ukuran Payload pada Webhook Pembayaran (`POST /api/v1/webhooks/xendit`)
- **Pengujian:** Mengirimkan payload berukuran 1.2 MB dengan header token valid.
- **Hasil:**
  - Status HTTP **413 Payload Too Large** (`PASS`).
  - Kode error `"code": "PAYLOAD_TOO_LARGE"` (`PASS`).

### Skenario 5: Penegakan Batas Panjang Header `Idempotency-Key` (Maks 64 Karakter)
- **Pengujian:** Mengirimkan header `Idempotency-Key` sepanjang 74 karakter.
- **Hasil:**
  - Request ditolak sebelum reservasi atau domain logic dengan status HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "INVALID_IDEMPOTENCY_KEY"` (`PASS`).

### Skenario 6: Penolakan Payload Malformed JSON
- **Pengujian:** Mengirimkan body JSON sintaksis rusak tanpa tanda kurung penutup (`{ "guest_name": ... `).
- **Hasil:**
  - Ditolak di transport layer dengan HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "INVALID_JSON"` (`PASS`).

### Skenario 7: Skema Error Dual-Shape Konsisten pada Portal Tamu
- **Pengujian:** Mengirimkan alamat email yang tidak valid (`"email": "invalid-email-format"`).
- **Hasil:**
  - Status HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "INVALID_EMAIL"` dan `"error": "INVALID_EMAIL"` (`PASS`).
  - Deskripsi manusiawi `"message"` dan `"detail"` hadir bersamaan (`PASS`).

### Skenario 8: Non-Regresi Operasional Normal (< 1 MB)
- **Pengujian:** Request kuotasi kamar normal dengan payload sah.
- **Hasil:**
  - Status HTTP **200 OK** (`PASS`).
  - Respons sukses memuat `quote_id` dan kalkulasi harga `total_price_minor` secara tepat (`PASS`).

---

## 3. Kesimpulan Verifikasi
Fitur penyeragaman batas payload dan skema error dual-shape (`BE-R18`) telah teruji dan bekerja 100% pada level unit table test (`router_test.go`, coverage 83.8%) dan level live end-to-end testing (`payload_boundary_and_unified_error_r18_e2e.sh`).
