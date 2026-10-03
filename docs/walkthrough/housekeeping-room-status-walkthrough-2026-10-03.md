# Walkthrough — Housekeeping Room Status & Readiness Lifecycle
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **PRD:** [PRD-Housekeeping-Readiness](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/housekeeping-room-status-and-readiness-2026-10-03.md)
- **SRS:** [SRS-Housekeeping-Readiness](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/housekeeping-room-status-and-readiness-2026-10-03.md)
- **Tech Architecture:** [TECH-Housekeeping-Readiness](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/housekeeping-room-status-architecture-2026-10-03.md)

---

## Progress Tracking Matrix

| Tahap | Aktivitas | Status | Target Evidence |
|---|---|:---:|---|
| **Tahap 1** | Riset Industri (AHLA, Cloudbeds, Opera PMS) & Analisis Kritis Kesiapan Kamar | **DONE** | PRD, SRS, Tech Architecture |
| **Tahap 2** | Pembuatan Dokumen Siklus Hidup PRD, SRS, dan Desain Arsitektur Teknis | **DONE** | `docs/prd/`, `docs/srs/`, `docs/tech/` |
| **Tahap 3** | Walkthrough Tracking & Milestone Log | **DONE** | `docs/walkthrough/` |
| **Tahap 4** | Implementasi TDD (Table-Driven Tests $\ge 80\%$ Coverage) | **DONE** | `internal/housekeeping/` (80.3% coverage), `go vet` clean |
| **Tahap 5** | Automasi Pengujian E2E & Pembuatan Laporan | **DONE** | E2E-46 s/d E2E-51 PASS (51/51), `testing/e2e/report/` |
| **Tahap 6** | Verifikasi Akhir & Konfirmasi Git Commit | **READY** | Selective staging tanpa menyentuh file untracked user |

---

## File Modification Plan

1. **`migrations/00010_housekeeping_room_readiness.sql`**:
   - Menambahkan kolom `cleanliness_status`, `maintenance_notes`, `updated_by`, `updated_at` pada tabel `rooms`.
   - Mengatur nilai default 'inspected' untuk 95 kamar eksisting agar sistem tetap siap huni.
   - Menambahkan aturan Casbin RBAC untuk role `housekeeping`.
2. **`internal/housekeeping/` (Package Domain Baru)**:
   - `model.go`: Definisi CleanlinessStatus enum, RoomOperationalView, UpdateStatusInput.
   - `store.go`: Interface `Store` dan custom domain errors (`ErrRoomNotFound`, `ErrInvalidTransition`, `ErrRoomNotReady`).
   - `postgres.go`: Implementasi Postgres store query 95 kamar dan update status.
   - `postgres_test.go`: Pengujian store langsung ke database test.
   - `service.go`: Business use case `GetRoomBoard`, `UpdateStatus`, `ValidateRoomForCheckIn`, `MarkRoomDirtyOnCheckOut`.
   - `service_test.go`: Table-driven unit test state machine transitions & guards.
3. **`internal/api/housekeeping_handler.go` & `internal/api/housekeeping_api_test.go`**:
   - Handler untuk `GET /api/v1/housekeeping/rooms`, `PUT /api/v1/housekeeping/rooms/{room_number}/status`, `POST /api/v1/housekeeping/rooms/{room_number}/out-of-order`.
   - Table-driven HTTP handler tests.
4. **`internal/api/router.go` & `cmd/server/main.go`**:
   - Registrasi rute Housekeeping.
   - Pasang guard kesiapan kamar fisik pada `checkIn` handler (`booking.ErrRoomNotReady` -> HTTP 409 `ROOM_NOT_READY`).
   - Pasang auto-dirty pada `checkOut` handler (`UpdateStatus` -> `vacant_dirty`).
   - Wiring di `main.go`.
5. **`testing/e2e/`**:
   - Tambahkan skenario E2E-46 s/d E2E-51 di `testing/e2e/script/e2e_runner_test.go`.
   - Skrip executable bash di `testing/e2e/script/housekeeping_room_readiness_e2e.sh`.
   - Laporan E2E di `testing/e2e/report/2026-10-03-housekeeping-readiness-e2e-report.md`.

---

## Execution & Verification Log

### 1. Database Migration (Goose V10)
```bash
GOOSE_DRIVER=postgres GOOSE_DBSTRING="postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable" goose -dir migrations up
# Output: OK 00010_housekeeping_room_readiness.sql
```

### 2. Unit & Integration Testing (Target $\ge$ 80% Coverage)
```bash
go test -v -cover ./internal/housekeeping/...
# Result: PASS (coverage: 80.3% of statements)

go test -v -run TestHousekeepingAPI ./internal/api
# Result: PASS (18 table-driven test cases)
```

### 3. Lint / Vet Verification
```bash
go vet ./...
# Result: clean (0 warnings, 0 errors)
```

### 4. End-to-End Suite Execution
```bash
go test -v ./testing/e2e/script -run TestEndToEndHotelBookingRBACLifecycle
# Result: PASS 100% (51/51 scenarios, including E2E-46 through E2E-51)
```

