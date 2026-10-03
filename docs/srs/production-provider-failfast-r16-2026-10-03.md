# SRS — Production Provider Fail-Fast & Capability Readiness (BE-R16)

**Fitur:** Production Provider Fail-Fast, Safe Readiness Capability & Fake-Pay Hardening  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** [PRD](../prd/production-provider-failfast-r16-2026-10-03.md) · BE-R16  

---

## 1. Spesifikasi Kebutuhan Fungsional (Functional Requirements)

### FR-01: Validasi Konfigurasi Startup Fail-Fast (Twelve-Factor & PCI-DSS)
- **Deskripsi:** Method `Config.Validate()` pada package `platform` wajib memvalidasi kelengkapan variabel lingkungan pihak ketiga jika `Environment == "production"`.
- **Kondisi Validasi:**
  1. `XENDIT_SECRET_KEY` tidak boleh string kosong.
  2. `XENDIT_WEBHOOK_TOKEN` tidak boleh string kosong.
  3. `RESEND_API_KEY` tidak boleh string kosong.
- **Pesan Error Spesifik:**
  - `config: XENDIT_SECRET_KEY is required in production (fake payment gateway is forbidden)`
  - `config: XENDIT_WEBHOOK_TOKEN is required in production`
  - `config: RESEND_API_KEY is required in production (log notifier is forbidden)`
- **Behavior:** Jika salah satu syarat gagal, proses aplikasi memanggil `os.Exit(1)` saat *startup*.

### FR-02: Defense-in-Depth Adapter di `cmd/server/main.go`
- **Deskripsi:** Logika inisialisasi runtime di composition root wajib menolak inisialisasi `payment.NewFake()` atau `notifier.NewLog()` jika `cfg.IsProduction()`.
- **Behavior:** Bila terdeteksi upaya inisialisasi fake/log adapter pada production, sistem mencatat pesan error terstruktur `payment.gateway.forbidden_in_production` atau `notifier.forbidden_in_production` dan menghentikan proses aplikasi.

### FR-03: Redaksi OTP pada `LogNotifier`
- **Deskripsi:** Struct `notifier.LogNotifier` memiliki opsi konfigurasi `MaskOTP bool`.
- **Behavior:** Jika `MaskOTP` bernilai `true`, log yang dihasilkan mencatat `"otp": "[REDACTED]"` alih-alih angka OTP asli.

### FR-04: Kontrak Endpoint Readiness Terverifikasi (`GET /ready`)
- **Deskripsi:** Endpoint `/ready` diperkaya untuk menyajikan metadata kapabilitas environment saat status sistem sehat (HTTP 200).
- **Format Respons Sukses (HTTP 200 OK):**
```json
{
  "status": "ready",
  "environment": "development",
  "payment_gateway": "fake",
  "notifier": "log"
}
```
- **Format Respons Kegagalan (HTTP 503 Service Unavailable):**
```json
{
  "status": "unavailable",
  "error": "postgres ping: connection refused"
}
```

### FR-05: Sanitasi & Validasi UUID Endpoint Simulasi Dev (`POST /fake-pay/:ref`)
- **Deskripsi:** Endpoint dev-only `/fake-pay/:ref` memverifikasi validitas format UUID pada query parameter `booking_id` atau path parameter `:ref`.
- **Format Respons Error Validasi (HTTP 400 Bad Request):**
```json
{
  "status": 400,
  "code": "INVALID_BOOKING_ID",
  "error": "booking_id must be a valid UUID",
  "title": "Bad Request",
  "detail": "booking_id must be a valid UUID"
}
```
- **Format Respons Not Found (HTTP 404 Not Found):**
```json
{
  "status": 404,
  "code": "BOOKING_NOT_FOUND",
  "error": "booking tidak ditemukan",
  "title": "Not Found",
  "detail": "booking tidak ditemukan"
}
```
- **Format Respons Hold Expired (HTTP 409 Conflict):**
```json
{
  "status": 409,
  "code": "HOLD_EXPIRED",
  "error": "hold has expired, room availability was released",
  "title": "Conflict",
  "detail": "hold has expired, room availability was released"
}
```
- **Behavior di Production:** Route `/fake-pay/:ref` tidak didaftarkan ke Gin router (`IsDevelopment == false`), sehingga setiap request menerima HTTP 404 standar.
