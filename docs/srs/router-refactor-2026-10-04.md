# SRS — Refactor Transport Router

**Dokumen ID:** `SRS-ROUTER-REFACTOR-2026-10-04` · **PRD:** [router-refactor](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/router-refactor-2026-10-04.md)

## 1. Functional Requirements
| ID | Requirement |
|---|---|
| FR-01 | `NewRouter(d Deps) *gin.Engine` tidak boleh memanggil mutator domain (`SetBaseRateSource`, `SetQuoteStore`) maupun `gin.SetMode`; wiring dilakukan di `cmd/server/main.go` dan e2e runner. Default murni transport (`CatalogStore` memory, `IdempotencyStore` memory) boleh tetap di `Deps.withDefaults()`. |
| FR-02 | Seluruh path+method yang terdaftar tetap identik (golden file `testdata/routes.golden`). |
| FR-03 | Route didaftarkan per domain: `registerHealth`, `registerAuth`, `registerGuest`, `registerCatalog`, `registerBooking`, `registerFinance`, … menerima `*gin.RouterGroup` ber-prefix `/api/v1` dan helper `ff(key)`. Casbin tetap membaca `c.Request.URL.Path` penuh. |
| FR-04 | Middleware global (urutan tetap): RequestID → Recovery → Timeout(30s) → RateLimit → BodySizeLimit. |
| FR-05 | RequestID: terima `X-Request-Id`, validasi ≤64 char `[A-Za-z0-9._-]`, selain itu generate; simpan di `gin.Context`, request context, header respons, dan `ProblemDetails.Instance`. |
| FR-06 | Recovery: respons 500 `INTERNAL_ERROR` generik (tanpa nilai panic); log `slog.Error("http.panic", request_id, method, path, panic, stack)`. |
| FR-07 | `NoRoute` → 404 `NOT_FOUND`; `NoMethod` (`HandleMethodNotAllowed=true`) → 405 `METHOD_NOT_ALLOWED` + header `Allow`; keduanya `application/problem+json` dual-shape. |
| FR-08 | `Deps.TrustedProxies []string` → `r.SetTrustedProxies(...)`; `nil` berarti tidak mempercayai header forward. Env `TRUSTED_PROXIES` (koma) di `main`. |
| FR-09 | RateLimiter: eviction visitor idle > 10 menit (lazy sweep tiap N request atau ticker yang berhenti saat `Close()`); `/healthz` & `/ready` dikecualikan. |
| FR-10 | Error mapping berbasis tabel: `type errRule struct{ is error; status int; code, msg string }` + `writeDomainError(c, err, rules, fallback)`. Tabel per handler. Pesan & kode HTTP existing dipertahankan persis; error tak terpetakan → fallback 500 generik (tanpa `err.Error()`), detail ke log. |
| FR-11 | `featureEnabled(c, d, key) bool` menyuntik `featureflag.WithRole` lalu `IsEnabled`; fail-open bila manager nil (perilaku existing). Semua flag inline memakainya. |
| FR-12 | `withIdempotency` helper: scope key = `sha256(subject\|METHOD\|route\|key)`; `Complete` memakai `context.WithoutCancel`; replay mengembalikan status+body+`Idempotency-Replayed: true`. Perilaku Reserve/Complete/Release tidak berubah. |
| FR-13 | Parser guest-count bersama (`parseGuestCounts`) dipakai `searchRooms` & `calculateQuote`; konstanta bernama `maxRooms=8`, `maxChildAge=17`, `maxStayNights=30`, `bookingHorizonDays=365`. Kode error existing dipertahankan. |
| FR-14 | Logika search → `internal/booking` atau `internal/inventory` service method `SearchAvailability(ctx, SearchQuery) ([]SearchResult, error)`; handler hanya parse/serialize. |
| FR-15 | Logika webhook → `booking.Service.ApplyPaymentEvent(ctx, PaymentEvent) (Outcome, error)` (amount/currency/invoice check, confirm, late-payment case, expiry). Handler hanya verify-token → map outcome ke HTTP. Error ledger → error (5xx), bukan lewati. |
| FR-16 | `/ready` publik: `{"status":"ready"}` / `{"status":"unavailable"}` tanpa `err`, env, gateway, notifier. Detail ke log. |
| FR-17 | Tes `TestEveryProtectedRouteHasPolicy`: parse `INSERT` policy di `migrations/*.sql` + seed `casbin_pgx.go`, cocokkan setiap route `apiGroup` dengan `keyMatch2` untuk ≥1 role. Daftar pengecualian eksplisit (mis. `/fake-pay/:ref`). |

## 2. Kontrak HTTP (perubahan yang terlihat klien)
| Endpoint | Sebelum | Sesudah |
|---|---|---|
| route tak dikenal | `404` text/plain | `404` problem+json `{"code":"NOT_FOUND",...}` |
| method salah | `404` | `405` + `Allow`, `METHOD_NOT_ALLOWED` |
| `GET /ready` (error) | `503 {"status","error":"<err>"}` | `503 {"status":"unavailable"}` |
| `GET /ready` (ok) | `{status,environment,payment_gateway,notifier}` | `{status}` |
| panic | `500` detail berisi nilai panic | `500` detail `"internal server error"` |
| webhook ledger error | lanjut confirm | `500 INTERNAL_ERROR` |
| `POST /bookings` key sama beda subjek | replay | proses sebagai request baru |

> Perubahan `/ready` dapat memengaruhi skrip E2E/monitoring yang membaca `environment`; diperiksa pada Tahap 5.

Selain tabel di atas, seluruh kode status, `code`, dan bentuk dual-shape (`code,error,message,detail,title,status`) **tidak berubah**.

## 3. Error Codes Baru
`NOT_FOUND` (404), `METHOD_NOT_ALLOWED` (405). Tidak ada kode lain yang dihapus.

## 4. Acceptance (ringkas)
Golden route identik · semua test existing hijau · test baru per D-01…D-10 · coverage ≥80% · `go vet` bersih · E2E regresi penuh lulus.

---

## Addendum A — Middleware (2026-10-04 00:31) — menggantikan FR-04/FR-05/FR-08/FR-09 bila bertentangan

| ID | Requirement |
|---|---|
| FR-18 | **Urutan global final**: `requestID → accessLog → recoverProblem → secureHeaders → CORS → timeout(30s, `Deps.RequestTimeout`) → globalRateLimit → bodySizeLimit`. `accessLog` di luar `recover` agar 500 hasil panic tercatat; `CORS` sebelum rate-limit/auth agar preflight tidak butuh kredensial. |
| FR-19 | `requestID`: terima `X-Request-Id`/`X-Request-ID`, valid bila ≤64 char `[A-Za-z0-9._-]`, selain itu generate 16-byte hex. Disimpan di request context (`RequestIDFrom(ctx)`) dan header respons. `ProblemDetails` mendapat field **additive** `request_id`; `instance` tetap kosong/path (tidak dipakai klien). |
| FR-20 | `accessLog`: satu baris `slog.Info("http.request", method, route=c.FullPath() (fallback "unmatched"), status, latency_ms, bytes, request_id, role)`; `/healthz`,`/ready` → `slog.Debug`. Tidak mencatat body/header/query (PII). |
| FR-21 | `secureHeaders`: selalu `X-Content-Type-Options: nosniff`. `Cache-Control: no-store` diset oleh grup route autentikasi/PII (`staff`, `guest`, `bookings`, `auth`), bukan global. |
| FR-22 | `CORS(origins []string)`: tanpa origin terdaftar → no-op. Origin cocok → `Access-Control-Allow-Origin: <origin>`, `Vary: Origin`, `Allow-Headers: Authorization, Content-Type, Idempotency-Key, X-Guest-Token, X-Guest-Session, X-Request-Id`, `Allow-Methods` sesuai route, `Max-Age: 600`; `OPTIONS` preflight → 204 + abort. Wildcard `*` ditolak saat startup. Env `CORS_ALLOWED_ORIGINS` (koma). |
| FR-23 | `RateLimiter`: (a) `Retry-After` (detik, dibulatkan ke atas) pada 429; (b) eviction lazy; (c) instance kedua `Deps.AuthRateLimiter` dipasang sebagai route-middleware pada `POST /auth/staff/login`, `/auth/guest/challenge`, `/auth/guest/verify` (default `main`: 5 req/menit, burst 10, key = IP); (d) `/healthz`,`/ready` dikecualikan dari limiter global. |
| FR-24 | `IdentifySubject` dipecah: `resolveStaff(c, verifier) (principal, outcome)` murni + wrapper tipis. Perilaku fail-closed identik (stf_ tanpa verifier → 503; stf_ invalid → 401; infra error → 503; Bearer non-stf → guest). Ctx ditulis dengan satu `WithContext`; `c.Set` dihapus. |
| FR-25 | `Authorize` memakai `c.FullPath()`; bila kosong (tak seharusnya) → 404 `NOT_FOUND` fail-closed. Pesan 403 generik: `"role not authorized for this resource"` (detail role+method+path hanya di log). |
| FR-26 | Error writer tunggal: `writeError(c, status, code, msg)` → `ProblemDetails`; `httpErrorCode` jadi alias tipis; `httpError` dihapus; `writeGuestError` mempertahankan content-type `application/json` dan field existing. |
| FR-27 | Pecah berkas: `middleware_core.go`, `auth_middleware.go`, `ratelimit.go`, `response.go` (ProblemDetails, writers, `decodeJSON`, `BodySizeLimit`, `isMaxBytesError`). `requireStaffSession` pindah ke `auth_middleware.go`; `requireGuestSession` tetap di `guest_auth.go`. |

### Perubahan kontrak tambahan
| Hal | Sebelum | Sesudah |
|---|---|---|
| Body error | tanpa `request_id` | + `request_id` (additive) |
| 403 detail | `role 'x' is not authorized to GET /path` | generik (detail di log) |
| 429 | tanpa `Retry-After` | + `Retry-After` |
| Endpoint auth | limit global 20 rps | + limit ketat 5/menit burst 10 per IP |
| Respons non-publik | tanpa header cache | `Cache-Control: no-store`, `nosniff` |
| Preflight CORS | 404/401 | 204 bila origin terdaftar |

> Pesan 403 yang berubah dapat memecah test yang mencocokkan teks lama; ditangani saat F5 (assert pada `code`, bukan teks).
