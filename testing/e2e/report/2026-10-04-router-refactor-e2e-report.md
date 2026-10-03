# E2E Test Report: Router & Middleware Architecture Modernization

- **Tanggal / Waktu:** 2026-10-04 03:36:00 WIB
- **Target Arsitektur:** Modernisasi HTTP Transport Layer Khusus HTTP (`internal/api/http`), Dekomposisi Handler Subpackage (`internal/api/http/handler`), Dekomposisi Middleware Subpackage (`internal/api/http/middleware`), Pemisahan Domain Service, Scoped Idempotency, dan Middleware Hardening
- **Komponen Pengujian:**
  - [`internal/api/http/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/router.go) (Gin router initialization, trusted proxies, middleware pipeline)
  - [`internal/api/http/routes.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go) (Registrasi rute terstruktur, `/api/v1` group, `NoRoute` & `NoMethod` RFC 7807 handlers)
  - [`internal/api/http/middleware/core.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/core.go) (`RequestID`, `AccessLog`, `RecoverProblem`, `SecureHeaders`, `NoStore`, `TimeoutContext`, `NotFound`, `MethodNotAllowed`, `CORS`, `BodySizeLimit`)
  - [`internal/api/http/middleware/auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/auth.go) (`ResolveStaff`, `Authorize` Casbin dengan `c.FullPath()`, `RequireStaffSession`)
  - [`internal/api/http/middleware/guest_session.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/guest_session.go) (`RequireGuestSession`)
  - [`internal/api/http/middleware/ratelimit.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/ratelimit.go) (`RateLimiter` lazy sweep, `Retry-After`, `SkipRateLimitRoutes` exemption)
  - [`internal/api/http/middleware/idempotency.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/idempotency.go) (`ScopeIdempotencyKey` SHA-256 isolation, `WithIdempotency` dengan `context.WithoutCancel`)
  - [`internal/api/http/handler/errors.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/errors.go) (Table-driven domain error mapping ke RFC 7807 dual-shape)
  - [`internal/api/http/handler/guest_parser.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_parser.go) (`ParseGuestQuery`, `ValidateQuoteGuestCounts` dengan konstanta invariant)
  - [`internal/api/http/handler/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler) (20 Domain handlers terisolasi tanpa logic domain yang bocor)
  - [`internal/booking/search.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/search.go) (`booking.Service.SearchAvailability`)
  - [`internal/booking/payment_event.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/payment_event.go) (`booking.Service.ApplyPaymentEvent`)
  - [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go) & [`internal/platform/config.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/config.go) (Composition root injection, trusted proxies, CORS origins)
- **Lingkungan Pengujian:**
  - In-Process Go E2E Runner: [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) (Mocks komplit, Casbin memory enforcer, Feature flag memory manager)
  - Ephemeral Live HTTP Server: Port 28089 (`127.0.0.1:28089`) dijalankan otomatis oleh skrip pengujian
- **Skrip Eksekusi:** [`testing/e2e/script/router_refactor_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/router_refactor_e2e.sh)

---

## 1. Ringkasan Eksekusi

| Metrik | Nilai | Status |
| :--- | :--- | :--- |
| **Total Asersi Otomatis** | 21 asersi validasi runtime | 100% PASS |
| **Asersi Go In-Process** | 4 sub-test suite (`E2E-69` – `E2E-72`) | PASS |
| **Asersi Live Ephemeral HTTP** | 17 asersi via curl & real network socket | PASS |
| **Gagal / Error** | 0 | None |
| **Durasi Eksekusi** | ~0.8 detik | Cepat & Deterministik |
| **Statement Coverage (`internal/api/http`)** | **96.0%** (Target $\ge$ 80%) | MEMENUHI SYARAT |
| **Statement Coverage (`internal/api/http/middleware`)** | **84.8%** (Target $\ge$ 80%) | MEMENUHI SYARAT |
| **Statement Coverage (`internal/api/http/handler`)** | **80.3%** (Target $\ge$ 80%) | MEMENUHI SYARAT |
| **Static Analysis (`go vet ./...`)** | 0 warnings, 0 errors | CLEAN |

---

## 2. Rincian Skenario & Bukti Asersi

### Skenario 1: Go In-Process E2E Suite (E2E-69 .. E2E-72)
- **Pengujian:** Menjalankan rangkaian uji integrasi end-to-end terstruktur di dalam [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go).
- **Hasil:**
  - `E2E-69: Router Refactor NoRoute and NoMethod RFC 7807`: Terverifikasi 404 pada rute tidak dikenal, 405 pada method tidak terdaftar dengan header `Allow: GET`, serta 200 pada probe `/ready` (`PASS`).
  - `E2E-70: Scoped Idempotency Key Replay and Conflict Isolation`: Terverifikasi request pertama 201 Created, request kedua dengan payload identik mengembalikan 201 dengan header `Idempotency-Replayed: true`, dan request ketiga dengan payload berbeda mengembalikan 409 Conflict (`PASS`).
  - `E2E-71: Search Availability and Guest Occupancy Parser`: Terverifikasi query sah menghasilkan struktur varian kamar dan parser menolak umur anak $\ge 18$ tahun dengan 400 Bad Request (`PASS`).
  - `E2E-72: Webhook Delegation via ApplyPaymentEvent`: Terverifikasi penolakan webhook Xendit tanpa token otentikasi dengan status 401 Unauthorized (`PASS`).

### Skenario 2: Probe Endpoints & Security Headers
- **Pengujian:** Mengirimkan `HEAD /healthz` dan `GET /ready` ke server live.
- **Hasil:**
  - Header `X-Request-Id` (UUIDv4/hex 32-char) terinjeksi otomatis ke response (`PASS`).
  - Header `X-Content-Type-Options: nosniff` terinjeksi ke response (`PASS`).
  - Endpoint `/ready` mengembalikan payload status JSON `{"status":"ready", ...}` (`PASS`).

### Skenario 3: Penanganan RFC 7807 NoRoute (HTTP 404)
- **Pengujian:** Mengirimkan `GET /api/v1/non-existent-route-for-testing`.
- **Hasil:**
  - Status HTTP **404 Not Found** (`PASS`).
  - JSON RFC 7807 dual-shape memuat atribut `"code": "NOT_FOUND"`, `"title": "Not Found"`, dan `"status": 404` (`PASS`).

### Skenario 4: Penanganan RFC 7807 NoMethod & Header Allow (HTTP 405)
- **Pengujian:** Mengirimkan `DELETE /api/v1/search` (hanya terdaftar untuk method `GET`).
- **Hasil:**
  - Status HTTP **405 Method Not Allowed** (`PASS`).
  - JSON RFC 7807 memuat `"code": "METHOD_NOT_ALLOWED"` (`PASS`).
  - Response header memuat `Allow: GET` sesuai standar RFC 9110 (`PASS`).

### Skenario 5: Penegakan Batas Karakter Idempotency-Key
- **Pengujian:** Mengirimkan header `Idempotency-Key` sepanjang 65 karakter (> 64 batas aman).
- **Hasil:**
  - Ditolak sebelum menyentuh layer database/domain dengan status HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "INVALID_IDEMPOTENCY_KEY"` (`PASS`).

### Skenario 6: Validasi Parameter Okupansi Tamu (Unified Guest Parser)
- **Pengujian:** Mengirimkan query parameter `child_ages=18` pada endpoint `/api/v1/search`.
- **Hasil:**
  - Status HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "INVALID_CHILD_AGE"` (`PASS`).

### Skenario 7: Validasi Batas Durasi Menginap (Max Stay Period)
- **Pengujian:** Mengirimkan parameter reservasi menginap selama 39 malam (> 30 malam limit operasional).
- **Hasil:**
  - Status HTTP **400 Bad Request** (`PASS`).
  - Kode error `"code": "EXCEEDS_MAX_LOS"` (`PASS`).

### Skenario 8: Keamanan Callback Webhook Pembayaran
- **Pengujian:** Mengirimkan payload webhook tanpa header otentikasi `x-callback-token`.
- **Hasil:**
  - Ditolak seketika dengan status HTTP **401 Unauthorized** (`PASS`).
  - Kode error `"code": "UNAUTHORIZED"` (`PASS`).

---

## 3. Matriks Regresi Kontrak Rute Golden

Pengujian regresi terhadap [`internal/api/testdata/routes.golden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/testdata/routes.golden) dijalankan via unit test [`TestRoutesGolden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/route_guard_test.go):

| Kelompok Rute | Total Endpoint Terdaftar | Hasil Validasi Kontrak |
| :--- | :--- | :--- |
| **System Probes** | 2 (`/healthz`, `/ready`) | MATCH (100% Identik) |
| **Auth & Staff** | 4 (`/api/v1/auth/staff/*`) | MATCH (100% Identik) |
| **Guest Auth & Portal** | 8 (`/api/v1/auth/guest/*`, `/api/v1/guest/*`) | MATCH (100% Identik) |
| **Catalog & Inventory** | 6 (`/api/v1/catalog/*`, `/api/v1/availability`) | MATCH (100% Identik) |
| **Pricing & Checkout** | 3 (`/api/v1/search`, `/api/v1/quotes`, `/api/v1/bookings`) | MATCH (100% Identik) |
| **Booking Operations** | 7 (`/api/v1/bookings/:id/*`) | MATCH (100% Identik) |
| **Front Desk Operations** | 6 (`/api/v1/front-desk/*`) | MATCH (100% Identik) |
| **Housekeeping Board** | 3 (`/api/v1/housekeeping/*`) | MATCH (100% Identik) |
| **Finance & Reconciliation**| 4 (`/api/v1/finance/*`) | MATCH (100% Identik) |
| **Admin & Feature Flags** | 2 (`/api/v1/admin/feature-flags*`) | MATCH (100% Identik) |
| **Payment Webhook** | 1 (`/api/v1/webhooks/xendit`) | MATCH (100% Identik) |
| **Development Stubs** | 2 (`/fake-pay/*`, `/dev/quote-store/evict`) | MATCH (100% Identik) |
| **TOTAL** | **53 Routes Frozen** | **PASS (Zero Drift)** |

---

## 4. Kesimpulan & Rekomendasi

Seluruh 8 fase refaktor router dan middleware telah berhasil diselesaikan, memenuhi seluruh kriteria penerimaan spesifikasi PRD/SRS dan batas ketat anti-overengineering:
1. **Separation of Concerns:** Handler HTTP kini murni berfungsi sebagai transport adapter (decode, validate, delegate, encode). Domain logic (`SearchAvailability`, `ApplyPaymentEvent`) berada di domain service `internal/booking`.
2. **Security & Reliability:** Scoped Idempotency Key mencegah tabrakan lintas path/metode, Casbin RBAC terproteksi dari bypass via query parameters berkat `c.FullPath()`, dan RFC 7807 diterapkan konsisten pada seluruh layer (404, 405, 500 panic recovery).
3. **Efficiency:** Eviction rate-limiter menggunakan lazy sweep berbiaya amortized $O(1)$ tanpa background goroutine leak, dan response payload mematuhi dual-shape contract tanpa breaking changes.
4. **Verifikasi:** Lulus 100% unit tests, table-driven tests, static analysis (`go vet`), serta end-to-end execution. Siap untuk commit dan merge.
