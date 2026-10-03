# PRD — Refactor Transport Router (`internal/api/router.go`)

**Dokumen ID:** `PRD-ROUTER-REFACTOR-2026-10-04` · **Status:** Draft → Approved (scope dikonfirmasi user: fase 1–7, bug diperbaiki, wiring dipindah ke composition root)
**Terkait:** [SRS](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/router-refactor-2026-10-04.md) · [Tech](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/router-refactor-architecture-2026-10-04.md) · [Walkthrough](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/router-refactor-walkthrough-2026-10-04.md)

## 1. Latar Belakang
[`router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go) (1372 baris) mencampur: `Deps` + wiring domain, middleware inline, tabel route (prefix `/api/v1` diulang ±60×), 14 handler, dan logika domain (search, rekonsiliasi webhook). Akibatnya sulit dirawat dan ditemukan beberapa cacat reliabilitas/keamanan (lihat §4). Handler domain lain (finance, stay, housekeeping, dst.) sudah dipisah ke `*_handler.go`; refactor ini menyamakan pola tersebut.

## 2. Tujuan & Non-Tujuan
**Tujuan**
- *Maintainable*: `router.go` ≤ ~100 baris; satu file per domain handler; error mapping berbasis tabel; **`middleware.go` (308 baris, 5 tanggung jawab) dipecah per concern** dan semua middleware inline di router dijadikan fungsi bernama yang bisa dites (scope ditambahkan atas permintaan user, 2026-10-04 00:29).
- *Reliable*: perbaiki cacat §4; setiap route non-publik terbukti punya policy Casbin (test).
- *Efisien*: hilangkan `MaxBytesReader` ganda, eviction rate-limiter, kurangi duplikasi validasi. Tanpa optimasi prematur.

**Non-Tujuan (YAGNI / Ponytail)**: package baru berlapis, DI container, generator route, interface per-handler, optimasi paralel `searchRooms`, perubahan kontrak HTTP publik.

## 3. Persona & Dampak
| Persona | Dampak |
|---|---|
| Guest (publik) | Tidak ada perubahan kontrak; error 404/405 kini RFC 7807 |
| receptionist / housekeeping / revenue_mgr / finance / gm_admin | Tidak ada perubahan perilaku; RBAC tetap path-based Casbin |
| Engineer / AI agent | Menambah route cukup di satu file domain + satu baris register |
| Ops | Request-ID di log, panic tercatat dengan stack, `/ready` tidak membocorkan detail internal |

## 4. Cacat yang Diperbaiki (Acceptance Criteria)
| ID | Cacat | AC |
|---|---|---|
| D-01 | Idempotency key global (tidak di-scope subjek+route) → replay lintas pengguna | Key efektif = `hash(role/subject, METHOD, route, key)`; test: key sama beda subjek → tidak replay |
| D-02 | Flag inline tak membawa role (`featureflag.WithRole`) | Semua pengecekan flag lewat satu helper yang menyuntik role; test flag role-scoped |
| D-03 | Recovery membocorkan nilai panic ke client, tanpa log | Body generik `INTERNAL_ERROR`; `slog.Error` + stack + request-id |
| D-04 | Rate limiter: map tak pernah dibersihkan; trusted proxy default "semua" | Eviction visitor idle; `SetTrustedProxies` dari `Deps.TrustedProxies` (default `nil` = tak percaya header XFF) |
| D-05 | Kebocoran `err.Error()` (webhook, quote, `/ready`) | Pesan klien generik; detail hanya di log; `/ready` publik hanya `status` |
| D-06 | `NoRoute`/`NoMethod` plain-text | RFC 7807 `NOT_FOUND` / `METHOD_NOT_ALLOWED` |
| D-07 | Webhook fail-open bila ledger `GetPaymentAttempts` error | Error ledger → 5xx (Xendit retry), bukan lewati validasi |
| D-08 | Tak ada jaminan route↔policy Casbin | Test: tiap route non-publik cocok ≥1 policy (parse migrasi + seed) |
| D-09 | `gin.SetMode` global di `NewRouter` | Dipindah ke `main`/`TestMain` |
| D-10 | `Complete` idempotency memakai ctx yang bisa sudah timeout | Pakai `context.WithoutCancel` |

## 5. Non-Functional Requirements
- Coverage `internal/api` ≥ 80% (baseline 83,8%); `go vet ./...` bersih; seluruh test existing tetap hijau tanpa mengubah ekspektasi kontrak.
- Tidak ada penurunan latensi p95 (benchmark middleware chain sebelum/sesudah).
- Golden route table identik sebelum/sesudah (kecuali route baru yang tidak ada).

## 6. Risiko
| Risiko | Mitigasi |
|---|---|
| Trusted proxy `nil` mengubah IP klien di balik LB | Konfigurasi `TRUSTED_PROXIES` env; dokumentasi deploy |
| Memindah logika ke service memicu regresi | Fase 7 terpisah, characterization test + E2E regresi |
| Scope idempotency mengubah replay klien existing | Klien memakai key sendiri per sesi; tetap kompatibel; E2E replay |

---

## Addendum A — Refactor Middleware (2026-10-04 00:31)

Atas permintaan user, middleware ikut direfactor (best practice, efisien, maintainable). Keputusan user: tambahkan **access log, security headers, rate limit auth, CORS**; Authorize beralih ke **`c.FullPath()`**.

### Temuan middleware
| ID | Temuan | AC |
|---|---|---|
| D-11 | `middleware.go` mencampur auth, authz, body, error writer, rate limiter; 3 middleware inline di router tak bisa dites | Dipecah: `auth_middleware.go`, `ratelimit.go`, `response.go`, `middleware_core.go`; tiap middleware punya table-driven test |
| D-12 | `IdentifySubject` bersarang (if/else + switch), 3× `context.WithValue` + `c.Set` ganda yang tak pernah dibaca (`c.Get` 0 pemakaian) | Ekstrak `resolveStaff()`; hapus `c.Set` mati; satu `WithContext` per middleware; kunci `RoleKey/SubjectKey/GuestTokenKey` dipertahankan (test menginjeksinya) |
| D-13 | Tiga penulis error (`httpErrorCode`, `httpError` tak terpakai, `writeGuestError` dengan struktur map terpisah) | Satu `ProblemDetails`; hapus `httpError`; `writeGuestError` memakai struct yang sama (content-type tetap `application/json` demi kompatibilitas portal tamu) |
| D-14 | Tak ada access log; request-id hanya header | `accessLog` slog: method, route template (`FullPath`), status, latency, bytes, request_id, role; `/healthz` & `/ready` level debug |
| D-15 | Tidak ada security header | `X-Content-Type-Options: nosniff`, `Cache-Control: no-store` untuk respons non-publik (auth/PII/booking) |
| D-16 | Satu bucket rate limit per IP untuk semua endpoint (login staff 20 rps/IP lemah); tak ada `Retry-After` | `Deps.AuthRateLimiter` terpisah untuk `staff/login`, `guest/challenge`, `guest/verify`; `Retry-After` pada 429 |
| D-17 | Tidak ada CORS | `CORS(allowedOrigins)` opt-in via env `CORS_ALLOWED_ORIGINS`; kosong = nonaktif; preflight `OPTIONS` dijawab 204 sebelum rate limit & auth |
| D-18 | Authorize memakai `URL.Path` mentah | `c.FullPath()` (template route) + test matrix route × role; seluruh policy diverifikasi |
| D-19 | `isMaxBytesError` fallback `strings.Contains` rapuh | Hapus fallback bila test membuktikan `errors.As` cukup (termasuk lewat json v2) |

### Risiko tambahan
| Risiko | Mitigasi |
|---|---|
| `FullPath()` mengubah hasil match Casbin | Test matrix seluruh route × 6 role membandingkan keputusan `URL.Path` vs `FullPath` sebelum switch; tidak boleh ada selisih selain yang disengaja |
| CORS salah konfigurasi membuka origin | Default nonaktif; tanpa wildcard `*` bila `Authorization` dipakai; daftar eksplisit |
| `no-store` pada respons publik katalog | Hanya diterapkan pada route autentikasi/PII; katalog/search tidak |
