# Walkthrough — Stay Modification: Room Move & Stay Extension (Proposed 03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **PRD:** [PRD Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/stay-modification-room-move-and-extension-2026-10-03.md)
- **SRS:** [SRS Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/stay-modification-room-move-and-extension-2026-10-03.md)
- **Tech Architecture:** [Tech Architecture Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/stay-modification-room-move-architecture-2026-10-03.md)
- **E2E Test Report:** [E2E Test Report Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-stay-modification-room-move-e2e-report.md)

---

## Progress Tracking Matrix

| Tahap | Aktivitas | Status | Target Evidence |
|---|---|:---:|---|
| **Tahap 1** | Riset Industri (OPERA Cloud, Cloudbeds, Mews) & Analisis GiST Room Move | **DONE** | PRD, SRS, Tech Architecture |
| **Tahap 2** | Pembuatan Dokumen Siklus Hidup PRD, SRS, dan Desain Arsitektur Teknis | **DONE** | `docs/prd/`, `docs/srs/`, `docs/tech/` |
| **Tahap 3** | Walkthrough Tracking & Milestone Log | **DONE** | `docs/walkthrough/` |
| **Tahap 4** | Implementasi TDD (Table-Driven Tests $\ge 80\%$ Coverage) | **DONE** | `internal/stay/` (82.1%), `internal/api/` (84.9%), `go vet` clean |
| **Tahap 5** | Automasi Pengujian E2E & Pembuatan Laporan | **DONE** | E2E-56 s/d E2E-60 PASS (60/60), Bash script, E2E Report |
| **Tahap 6** | Verifikasi Akhir & Konfirmasi Git Commit | **DONE** | Zero vet errors, selective staging siap |

---

## File Modification & Implementation Summary

1. **`migrations/00012_stay_modification_and_room_move.sql`**:
   - Pembuatan tabel `room_move_logs` dan index `idx_room_move_booking`, `idx_room_move_created_at`.
   - Aturan Casbin RBAC untuk `receptionist` dan `finance`.
2. **`internal/stay/` (Package Domain Baru)**:
   - `model.go`: Definisi tipe data `RoomMoveInput`, `RoomMoveResult`, `ExtendStayInput`, `ExtendStayResult`, `RoomMoveLog`, `BookingDetails`.
   - `store.go`: Interface `Store` & domain errors (`ErrTargetRoomNotReady`, `ErrInvalidBookingStatus`, `ErrNoAvailabilityForExtension`, `ErrRoomPhysicalOverlap`).
   - `postgres.go`: Transaksi PostgreSQL atomik untuk memotong daterange kamar lama, insert penugasan kamar baru, transisi kebersihan kamar, dan perpanjangan inventaris & reservasi.
   - `postgres_test.go`: Pengujian integrasi Postgres pada tabel nyata database test.
   - `service.go`: Business use case `MoveRoom`, `ExtendStay`, `ListRoomMoves` terintegrasi dengan `rates.Engine`.
   - `service_test.go`: Table-driven tests unit logic & kalkulasi tarif perpanjangan dinamis.
   - **Coverage Package:** **82.1% of statements**.
3. **`internal/api/` (Transport Layer)**:
   - `stay_handler.go`: HTTP Handlers `handleRoomMove`, `handleExtendStay`, `handleListRoomMoves`.
   - `router.go`: Registrasi rute di dalam grup proteksi Casbin RBAC.
   - `stay_api_test.go`: 14 table-driven tests HTTP request/response & negative cases.
   - **Coverage API:** **84.9% of statements**.
4. **`cmd/server/main.go`**:
   - Inisialisasi `stayStore` dan `staySvc`, diinjeksi ke `api.Deps.StaySvc`.
5. **`testing/e2e/`**:
   - `testing/e2e/script/e2e_runner_test.go`: Penambahan skenario `E2E-56` s/d `E2E-60` (60/60 PASS 100%).
   - `testing/e2e/script/stay_modification_room_move_e2e.sh`: Skrip pengujian otomatis bash executable.
   - `testing/e2e/report/2026-10-03-stay-modification-room-move-e2e-report.md`: Laporan komprehensif hasil pengujian E2E.
