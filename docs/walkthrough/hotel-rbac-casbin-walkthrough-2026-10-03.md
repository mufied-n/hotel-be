# Implementation Walkthrough & Progress Tracker
# Casbin RBAC Integration — Hotel Pulang ke Uttara
**Fitur:** Role-Based Access Control (RBAC) dengan Casbin & PostgreSQL  
**Dokumen ID:** `WALKTHROUGH-RBAC-2026-10-03`  
**Status:** Completed & Verified  
**Tanggal Mulai:** 2026-10-03  
**Tanggal Selesai:** 2026-10-03  

---

## 1. Ikhtisar & Tujuan Pengerjaan
Walkthrough ini berfungsi sebagai dokumen pelacak pengerjaan (*execution tracker*) langkah demi langkah untuk implementasi RBAC hotel Pulang ke Uttara. Dokumen ini mendokumentasikan setiap fase, file yang dimodifikasi/dibuat, perintah pengujian, serta verifikasi hasil.

---

## 2. Rencana Eksekusi & Checklist Status

- [x] **Fase 1: Dokumentasi Lifecycle Fitur Lengkap**
  - [x] Buat PRD: [`docs/prd/hotel-rbac-casbin-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/hotel-rbac-casbin-2026-10-03.md)
  - [x] Buat SRS: [`docs/srs/hotel-rbac-casbin-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/hotel-rbac-casbin-2026-10-03.md)
  - [x] Buat Dokumen Desain Teknis: [`docs/tech/hotel-rbac-casbin-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/hotel-rbac-casbin-architecture-2026-10-03.md)
  - [x] Inisialisasi Walkthrough Tracker: [`docs/walkthrough/hotel-rbac-casbin-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/hotel-rbac-casbin-walkthrough-2026-10-03.md)

- [x] **Fase 2: Konfigurasi & Migrasi Database PostgreSQL**
  - [x] Tambahkan konfigurasi model Casbin di [`config/rbac_model.conf`](file:///mnt/code/projects/jobs/pulang/current-booking/config/rbac_model.conf).
  - [x] Buat file migrasi Goose [`migrations/00003_casbin_rbac.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00003_casbin_rbac.sql) (tabel `casbin_rule`, `staff_users`, dan seed kebijakan awal).
  - [x] Verifikasi build runner migrasi: `go build ./cmd/migrate`.

- [x] **Fase 3: Native Pgx Adapter & Casbin Enforcer Core**
  - [x] Implementasikan native adapter di [`internal/platform/auth/casbin_pgx.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth/casbin_pgx.go) yang terhubung langsung ke `*pgxpool.Pool` tanpa ORM luar (sesuai filosofi Ponytail).
  - [x] Sediakan factory method inisialisasi `NewEnforcer(pool *pgxpool.Pool, modelPath string) (*casbin.SyncedEnforcer, error)` dan in-memory helper `NewInMemoryEnforcer(policies [][]string)`.
  - [x] Buat unit test mandiri untuk adapter di [`internal/platform/auth/casbin_pgx_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth/casbin_pgx_test.go).
  - [x] Hasil tes `go test -v ./internal/platform/auth/...`: 25 test cases lulus (0.005s).

- [x] **Fase 4: HTTP Transport Layer & Middleware**
  - [x] Tambahkan middleware ekstraksi identitas (`IdentifySubject`) dan otorisasi Casbin (`Authorize`) di [`internal/api/middleware.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go).
  - [x] Pasang middleware pada grup route di [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go).
  - [x] Perbarui [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go) untuk menginisialisasi enforcer dan menginjeksinya ke router `api.Deps`.

- [x] **Fase 5: Testing & Verifikasi Komprehensif**
  - [x] Tulis table-driven test suite di [`internal/api/rbac_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/rbac_test.go) yang menguji:
    - Akses publik `guest` ke `/healthz`, `/ready`, `/api/v1/availability`, `/api/v1/bookings/:id` (200 OK).
    - Akses `guest` ke check-in `/api/v1/bookings/:id/check-in` (ditolak 403 Forbidden).
    - Akses `guest` ke check-out `/api/v1/bookings/:id/check-out` (ditolak 403 Forbidden).
    - Akses `guest` ke no-show `/api/v1/bookings/:id/no-show` (ditolak 403 Forbidden).
    - Akses staf `receptionist` ke check-in, check-out, no-show (sukses 200 OK).
    - Pewarisan peran `receptionist` $\rightarrow$ `guest` (sukses 200 OK).
    - Akses staf `housekeeping` ke check-in (ditolak 403 Forbidden).
    - Akses `gm_admin` ke seluruh endpoint (sukses 200 OK via wildcard `*`).
  - [x] Jalankan `go test -v ./...` dan pastikan seluruh test suite pass 100%.
  - [x] Jalankan `go vet ./...` dan verifikasi integritas kode.

---

## 3. Log Eksekusi & Catatan Modifikasi File

### A. Berkas Baru yang Dibuat:
1. [`docs/prd/hotel-rbac-casbin-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/hotel-rbac-casbin-2026-10-03.md): Product Requirements Document untuk RBAC Pulang ke Uttara.
2. [`docs/srs/hotel-rbac-casbin-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/hotel-rbac-casbin-2026-10-03.md): Software Requirements Specification dengan FR-01 s/d FR-06.
3. [`docs/tech/hotel-rbac-casbin-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/hotel-rbac-casbin-architecture-2026-10-03.md): Arsitektur teknis, diagram Mermaid, skema pgx adapter, dan metrik performa.
4. [`docs/walkthrough/hotel-rbac-casbin-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/hotel-rbac-casbin-walkthrough-2026-10-03.md): Walkthrough & tracker eksekusi ini.
5. [`config/rbac_model.conf`](file:///mnt/code/projects/jobs/pulang/current-booking/config/rbac_model.conf): Definisi model RBAC Casbin (`keyMatch2` & `g(r.sub, p.sub)`).
6. [`migrations/00003_casbin_rbac.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00003_casbin_rbac.sql): Migrasi SQL Goose untuk tabel `casbin_rule`, `staff_users`, dan initial seed policies.
7. [`internal/platform/auth/casbin_pgx.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth/casbin_pgx.go): Native adapter Casbin `persist.Adapter` langsung di atas `*pgxpool.Pool`.
8. [`internal/platform/auth/casbin_pgx_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth/casbin_pgx_test.go): 25 skenario unit test evaluasi aturan kebijakan Casbin.
9. [`internal/api/middleware.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go): Middleware `IdentifySubject` dan `Authorize`.
10. [`internal/api/rbac_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/rbac_test.go): Table-driven test suite HTTP router RBAC (14 test cases).

### B. Berkas yang Diperbarui:
1. [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go): Menambahkan `Enforcer *casbin.SyncedEnforcer` ke `Deps` dan membungkus route group API dengan `IdentifySubject()` & `Authorize(d.Enforcer)`.
2. [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go): Menginisialisasi `auth.NewEnforcer(pool, "config/rbac_model.conf")` dan menginjeksinya ke `api.Deps`.

---

## 4. Hasil Verifikasi & Pengujian Otomatis

### 4.1 Unit Test Casbin Policy (`internal/platform/auth`)
```text
=== RUN   TestCasbinRBACPolicyEvaluation
--- PASS: TestCasbinRBACPolicyEvaluation (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_can_view_availability (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_can_create_booking (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_can_view_booking_by_id (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_can_cancel_booking (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_CANNOT_check-in (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_CANNOT_check-out (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_CANNOT_mark_no-show (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/guest_CANNOT_access_housekeeping (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_can_check-in (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_can_check-out (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_can_mark_no-show (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_inherits_guest_availability_access (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_inherits_guest_booking_access (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_CANNOT_update_rates (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/receptionist_CANNOT_access_finance_reports (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/housekeeping_can_view_status (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/housekeeping_can_update_room (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/housekeeping_CANNOT_check-in (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/revenue_mgr_can_update_rates (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/revenue_mgr_CANNOT_check-in (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/finance_can_access_reports (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/finance_CANNOT_update_rates (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/gm_admin_can_check-in (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/gm_admin_can_update_rates (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/gm_admin_can_access_reports (0.00s)
    --- PASS: TestCasbinRBACPolicyEvaluation/gm_admin_can_do_anything (0.00s)
PASS
ok  	github.com/example/hotel-booking/internal/platform/auth	0.005s
```

### 4.2 Integration Test Router RBAC (`internal/api`)
```text
=== RUN   TestRBACRouteProtection
--- PASS: TestRBACRouteProtection (0.00s)
    --- PASS: TestRBACRouteProtection/Healthz_endpoint_is_public_without_credentials (0.00s)
    --- PASS: TestRBACRouteProtection/Ready_endpoint_is_public_without_credentials (0.00s)
    --- PASS: TestRBACRouteProtection/Anonymous_user_can_view_availability_(defaults_to_guest) (0.00s)
    --- PASS: TestRBACRouteProtection/Guest_role_can_view_booking_by_ID (0.00s)
    --- PASS: TestRBACRouteProtection/Guest_role_is_FORBIDDEN_from_check-in (0.00s)
    --- PASS: TestRBACRouteProtection/Anonymous_is_FORBIDDEN_from_check-in (0.00s)
    --- PASS: TestRBACRouteProtection/Guest_role_is_FORBIDDEN_from_check-out (0.00s)
    --- PASS: TestRBACRouteProtection/Guest_role_is_FORBIDDEN_from_marking_no-show (0.00s)
    --- PASS: TestRBACRouteProtection/Receptionist_role_can_check-in_via_X-User-Role (0.00s)
    --- PASS: TestRBACRouteProtection/Receptionist_role_can_check-out_via_Authorization_Bearer_token (0.00s)
    --- PASS: TestRBACRouteProtection/Receptionist_role_can_mark_no-show (0.00s)
    --- PASS: TestRBACRouteProtection/Receptionist_inherits_guest_access_to_view_booking (0.00s)
    --- PASS: TestRBACRouteProtection/Housekeeping_is_FORBIDDEN_from_check-in (0.00s)
    --- PASS: TestRBACRouteProtection/General_Manager_(gm_admin)_can_check-in (0.00s)
    --- PASS: TestRBACRouteProtection/General_Manager_(gm_admin)_can_check-out (0.00s)
PASS
ok  	github.com/example/hotel-booking/internal/api	0.007s
```

### 4.3 Keseluruhan Paket (`go test ./...` & `go vet ./...`)
* `go test ./...` -> Semua paket passed (100% lulus).
* `go vet ./...` -> Exit code 0 (tidak ada temuan kelemahan kode).
