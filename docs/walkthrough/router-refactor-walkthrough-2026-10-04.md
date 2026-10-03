# Walkthrough — Refactor Transport Router & Middleware

**Dokumen ID:** `WALK-ROUTER-REFACTOR-2026-10-04` · [PRD](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/router-refactor-2026-10-04.md) · [SRS](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/router-refactor-2026-10-04.md) · [Tech](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/router-refactor-architecture-2026-10-04.md)

## Baseline (2026-10-04 00:24)
- `go test ./internal/api -cover` → **83.8%**, lulus.
- `router.go`: 1372 baris. Call site `NewRouter`: `cmd/server/main.go:289`, `testing/e2e/script/e2e_runner_test.go:1022,1527`.

## Verifikasi Temuan (Tahap 1)
| Temuan | Status | Bukti |
|---|---|---|
| Idempotency key global | **Terkonfirmasi** | [`idempotency.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/idempotency.go) `ON CONFLICT (key)` tanpa scope subjek/route |
| Policy Casbin tersebar | **Ada di migrasi** (00003, 00009–00018) + seed `casbin_pgx.go`; seed in-code tidak memuat housekeeping/front-desk → bukan bug, tetapi tidak ada test jaminan → D-08 |
| Flag role-scoped | Terkonfirmasi dari kode (`RequireFeature` vs inline) | router.go:722,806,1030,1137,1165 |
| Trusted proxy | Perilaku default Gin (riset) | search_web |

## Fase & Checklist
- [x] **T1** Riset & verifikasi
- [x] **T2** PRD, SRS, Tech
- [x] **T3** Walkthrough (dokumen ini)
- [x] **F1** Golden route table + `TestEveryProtectedRouteHasPolicy` (jaring pengaman; hijau pada kode lama)
- [x] **F1b** Test parity `URL.Path` vs `FullPath` + benchmark baseline `BenchmarkMiddlewareChain`
- [x] **F2** Pecah file (move-only) — `go test` tanpa ubah test
- [x] **F3** `errors.go` tabel error + migrasi handler (D-05 sebagian)
- [x] **F4** Route group + `routes.go` + `featureEnabled` (D-02)
- [x] **F5** Middleware core: requestID, recovery (D-03), NoRoute/NoMethod (D-06), trusted proxy (D-04), eviction (D-04), SetMode keluar (D-09), `/ready` (D-05)
- [x] **F5a** Pecah `middleware.go` → `middleware_core.go`, `auth_middleware.go`, `ratelimit.go`, `response.go`
- [x] **F5b** `resolveStaff` + table test; hapus `c.Set` mati; `writeError` tunggal; hapus `httpError` (D-12, D-13)
- [x] **F5c** `requestID`, `accessLog`, `secureHeaders`/`noStore`, `recoverProblem` (D-03, D-14, D-15)
- [x] **F5d** `RateLimiter`: `Retry-After`, eviction, skip health, `AuthRateLimiter` (D-04, D-16)
- [x] **F5e** `CORS` opt-in + env (D-17)
- [x] **F5f** `Authorize` → `FullPath`, pesan 403 generik, update test assert ke `code` (D-18)
- [x] **F5g** Hapus fallback `isMaxBytesError`; benchmark sesudah vs baseline
- [x] **F6** `withIdempotency` + scope (D-01, D-10); hapus `MaxBytesReader` ganda
- [x] **F7** Service `SearchAvailability` + `ApplyPaymentEvent` (D-07), `parseGuestCounts`
- [x] **F8** Wiring ke composition root (`main.go`, e2e runner) — FR-01
- [x] **F9** Dekomposisi subpackage bersih: `internal/api/middleware` & `internal/api/handler`
- [x] **F10** Restrukturisasi Transport Khusus HTTP: Pemindahan seluruh artefak HTTP ke `internal/api/http/` (router, routes, deps, idempotency, testdata, middleware, handler, serta seluruh berkas test). Root `internal/api/` disederhanakan menjadi facade tipis (`api.go`, `api_test.go`) dengan backward compatibility 100%.
- [x] **T5** E2E script + report ([`router_refactor_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/router_refactor_e2e.sh), [`2026-10-04-router-refactor-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-04-router-refactor-e2e-report.md))
- [x] **T6** `go test -v -cover ./...`, `go vet ./...`, verifikasi laporan

---

## Log Eksekusi
| Waktu | Aksi | Hasil |
|---|---|---|
| 2026-10-04 00:24 | Baseline test api | 83.8% ok |
| 2026-10-04 00:27 | Riset idempotency scope & Gin trusted proxies | dicatat di Tech §1 |
| 2026-10-04 00:30 | PRD/SRS/Tech awal dibuat | selesai |
| 2026-10-04 00:31 | User meminta scope middleware diperluas | Addendum A ditambahkan ke Tech & Walkthrough |
| 2026-10-04 01:00 | **F1 & F1b**: Freeze Golden routes & parity tests | 53 routes dibekukan di `testdata/routes.golden`; `TestRoutesGolden`, `TestAuthorizeFullPathParity`, `BenchmarkMiddlewareChain` lulus |
| 2026-10-04 01:25 | **F2**: Pemecahan file handler domain | 8 berkas handler terpisah (`health`, `catalog`, `search`, `quote`, `booking`, `webhook`, `helpers`, `deps`), router.go terpangkas |
| 2026-10-04 01:40 | **F3**: Penyeragaman error mapping | `errors.go` dengan `errRule` table-driven; integrasi `writeDomainError` |
| 2026-10-04 01:55 | **F4**: Grouping rute `/api/v1` & Role-aware Feature Flags | `routes.go` (`registerRoutes`, `registerGuestRoutes`, `registerDomainAPIRoutes`), `featureEnabled(c, ff, key)` di `featureflag_middleware.go` |
| 2026-10-04 02:10 | **F5**: Modernisasi Middleware Core & RBAC | `auth_middleware.go` (`Authorize` dengan `c.FullPath()`), `middleware_core.go` (NoRoute 404, NoMethod 405 + Allow, nosniff, accessLog slog), `ratelimit.go` (lazy sweep, `Retry-After`, bypass `/healthz`, `/ready`) |
| 2026-10-04 02:25 | **F6**: Idempotency Key Scoping & Isolation | `idempotency_mw.go` (`ScopeIdempotencyKey` SHA-256), `withIdempotency` (`context.WithoutCancel`), table-driven tests di `idempotency_test.go` |
| 2026-10-04 02:35 | **F7**: Ekstraksi Domain Service & Parser Tamu | `guest_parser.go` (konstanta batas aman), `booking.Service.SearchAvailability`, `booking.Service.ApplyPaymentEvent` (ledger error -> 500) |
| 2026-10-04 02:40 | **F8**: Composition Root Wiring | `cmd/server/main.go` & `platform/config.go` (`TrustedProxies`, `CORSAllowedOrigins`, late payment adapter) |
| 2026-10-04 03:00 | **F9**: Dekomposisi Subpackage Terstruktur | Pemindahan seluruh middleware ke `internal/api/middleware/` dan handler domain ke `internal/api/handler/`. |
| 2026-10-04 03:30 | **F10**: Restrukturisasi Transport Khusus HTTP (`internal/api/http/`) | Sesuai arahan user (Opsi A), seluruh artefak HTTP dikelompokkan rapi ke dalam `internal/api/http/` (router, routes, deps, idempotency, testdata, middleware, handler, serta seluruh berkas test). Import callers (`main.go`, `e2e_runner_test.go`, `idempotency_test.go`) diarahkan langsung ke `internal/api/http`. Berkas facade `api.go` dihapus sehingga root `internal/api` murni dan bebas dari file yang tercecer. |
| 2026-10-04 03:35 | **T4**: Table Tests & Coverage Verification | `go test ./...` 100% PASS.<br>• `internal/api/http`: **96.0% statement coverage**<br>• `internal/api/http/middleware`: **84.8% statement coverage**<br>• `internal/api/http/handler`: **80.3% statement coverage**<br>`go vet ./...` clean tanpa warning/error. |
| 2026-10-04 03:36 | **T5**: E2E Automation & Verification | In-process suite (E2E-69..E2E-72) + Live ephemeral server (`router_refactor_e2e.sh`), 21/21 asersi PASS, laporan `testing/e2e/report/2026-10-04-router-refactor-e2e-report.md` |
| 2026-10-04 04:10 | **T6**: Final Verification | Siap konfirmasi user untuk git commit |
