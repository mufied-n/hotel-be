# Software Requirements Specification (SRS)
# Modernisasi Transport Layer: Gin Router, JSON v2, & Go Validator v10
**Properti:** Hotel Pulang ke Uttara (Yogyakarta)  
**Dokumen ID:** `SRS-TRANSPORT-MODERNIZATION-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / In Execution  
**PRD Referensi:** [`docs/prd/transport-migration-gin-jsonv2-validator-2026-10-03.md`](file:///home/ahmadm/.gemini/antigravity/worktrees/current-booking/migrate_router_to_gin/docs/prd/transport-migration-gin-jsonv2-validator-2026-10-03.md)  

---

## 1. Lingkup & Deskripsi Arsitektural

Dokumen ini mendefinisikan spesifikasi kebutuhan teknis dan antarmuka untuk memigrasikan transport layer pada modul [`internal/api`](file:///home/ahmadm/.gemini/antigravity/worktrees/current-booking/migrate_router_to_gin/internal/api) dari Chi router ke Gin web engine, mengadopsi `encoding/json/v2` (Go 1.27), dan mengintegrasikan `go-playground/validator/v10` untuk validasi DTO deklaratif.

---

## 2. Kebutuhan Fungsional (Functional Requirements)

### FR-TRANS-01: Inisialisasi Gin Engine & Global Pipeline
* Sistem wajib menginisialisasi router melalui `gin.New()` dalam mode `gin.ReleaseMode` (atau `gin.TestMode` saat testing) untuk mencegah log polusi pada stdout.
* Pipeline global Gin wajib mencakup:
  1. **RequestID Middleware**: Menyuntikkan atau membaca header `X-Request-Id`.
  2. **RealIP Middleware**: Mengekstrak alamat IP klien yang sebenarnya.
  3. **Recovery Middleware**: Menangkap *panic* goroutine dan mengembalikan RFC 7807 error `500 Internal Server Error`.
  4. **Timeout Middleware**: Menetapkan deadline context request 30 detik.
  5. **Token Bucket Rate Limiter**: Membatasi laju request per IP (20 req/s, burst 40).

### FR-TRANS-02: Normalisasi Route Parameters
* Semua path route yang sebelumnya menggunakan sintaks Chi `{id}` atau `{key}` dinormalisasi ke format Gin `:id` atau `:key`.
* Daftar endpoint terproteksi dan publik tetap konsisten dengan model Casbin RBAC (`config/rbac_model.conf`).

### FR-TRANS-03: Serialisasi & Deserialisasi dengan `encoding/json/v2`
* Fungsi pembantu rendering respon `writeJSON(c *gin.Context, code int, v any)` wajib menggunakan `json.MarshalWrite(c.Writer, v)`.
* Respon error `writeProblemDetails` wajib menggunakan `Content-Type: application/problem+json` dan diserialisasi via `json.MarshalWrite`.
* Deserialisasi payload body di handler wajib menggunakan `json.UnmarshalRead(c.Request.Body, &req)` atau `json.Unmarshal(bodyBytes, &req)` saat body perlu di-cache untuk idempotency hash.
* Sistem wajib menolak JSON malformed atau duplicate keys dengan kode error `INVALID_JSON` / `BAD_REQUEST`.

### FR-TRANS-04: Validasi Skema Deklaratif dengan `validator/v10`
* DTO request wajib dilengkapi struct tags `validate:"..."`.
* Jika validasi tag gagal, adapter error wajib memetakan kesalahan ke format RFC 7807 Problem Details dengan kode error spesifik:
  * Kesalahan `rooms` $\rightarrow$ `INVALID_ROOM_COUNT`
  * Kesalahan `adults` / `children` $\rightarrow$ `INVALID_GUEST_COUNT`
  * Kesalahan `child_ages` $\rightarrow$ `INVALID_CHILD_AGE`
  * Kesalahan format tanggal `check_in` / `check_out` $\rightarrow$ `INVALID_DATE_FORMAT`
  * Kesalahan payload umum $\rightarrow$ `INVALID_PAYLOAD` / `BAD_REQUEST`

### FR-TRANS-05: Adapter Middleware & Otentikasi
* Middleware `IdentifySubject()` mengekstrak role, subject, dan `X-Guest-Token`, menyimpannya ke request context Go (`c.Request.Context()`) serta context Gin (`c.Set(...)`).
* Middleware `Authorize(enforcer)` mengevaluasi `enforcer.Enforce(role, path, method)` secara fail-closed (HTTP 503 jika nil, HTTP 403 jika tidak diizinkan).
* Middleware `RequireFeature(ff, key)` mengevaluasi status feature flag runtime; jika nonaktif mengembalikan HTTP 503 `FEATURE_DISABLED`.
* Middleware `requireGuestSession(guestSvc)` memvalidasi sesi tamu aktif dari header Bearer / cookie / `X-Guest-Session`.

### FR-TRANS-06: Endpoint Simulasi Dev (`FakePay`)
* Pada mode pengembangan (`IsDevelopment: true`), endpoint `/fake-pay/:ref` dimount dan mengekstrak parameter `ref` menggunakan `c.Param("ref")` atau query `booking_id`.

---

## 3. Matriks Error Code & Spesifikasi RFC 7807

```json
{
  "error": "rooms must be between 1 and 8",
  "title": "Bad Request",
  "status": 400,
  "detail": "rooms must be between 1 and 8",
  "code": "INVALID_ROOM_COUNT"
}
```

| Kasus Validasi | Field | Tag Validator | HTTP Status | Code RFC 7807 |
|---|---|---|---|---|
| Kamar melebihi kapasitas/batas | `rooms` | `min=1,max=8` | 400 Bad Request | `INVALID_ROOM_COUNT` |
| Tamu dewasa kurang dari 1 | `adults` | `min=1` | 400 Bad Request | `INVALID_GUEST_COUNT` |
| Usia anak di luar rentang 0-17 | `child_ages` | `custom_age` | 400 Bad Request | `INVALID_CHILD_AGE` |
| Format tanggal bukan YYYY-MM-DD | `check_in`, `check_out` | `datetime` | 400 Bad Request | `INVALID_DATE_FORMAT` |
| Role tidak memiliki akses | N/A | Casbin Enforce | 403 Forbidden | `FORBIDDEN` |
| Enforcer nil / uninitialized | N/A | Fail-closed guard | 503 Unavailable | `AUTH_SERVICE_UNAVAILABLE` |
| Fitur runtime dinonaktifkan | N/A | Feature flag guard | 503 Unavailable | `FEATURE_DISABLED` |
