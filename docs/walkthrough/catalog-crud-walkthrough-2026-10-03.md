# Walkthrough Tracker — Room Variant Catalog CRUD Management
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** Completed (100% Pass, Coverage $\ge 80\%$)
- **Feature Target:** Penambahan Operasi CRUD Varian Katalog Kamar untuk Hotel Management (`revenue_mgr`, `gm_admin`)

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset & Analisis Kebutuhan**
  - [x] Riset RESTful resource design & best practices perhotelan
  - [x] Analisis persona hotel: `revenue_mgr` dan `gm_admin`

- [x] **Fase 2: Dokumen Spesifikasi**
  - [x] PRD: [`docs/prd/catalog-crud-management-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/catalog-crud-management-2026-10-03.md)
  - [x] SRS: [`docs/srs/catalog-crud-management-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/catalog-crud-management-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/catalog-crud-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/catalog-crud-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/catalog-crud-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/catalog-crud-walkthrough-2026-10-03.md)

- [x] **Fase 3: Implementasi TDD (Target Coverage $\ge 80\%$)**
  - [x] Perluas interface `catalog.Store` dengan `CreateVariant`, `UpdateVariant`, `DeleteVariant`
  - [x] Implementasikan di `catalog.MemoryStore` dengan lock thread-safe dan stdlib UUID generator
  - [x] Implementasikan di `catalog.PostgresStore` dengan penanganan constraint 23505 dan 23503
  - [x] Table-driven unit test `internal/catalog/catalog_test.go` (**93.7% coverage**)
  - [x] Tambahkan endpoint di `internal/api/router.go`:
    - `GET /api/v1/catalog/rooms/{id}`
    - `POST /api/v1/catalog/rooms`
    - `PUT /api/v1/catalog/rooms/{id}`
    - `DELETE /api/v1/catalog/rooms/{id}`
  - [x] Tambahkan Casbin RBAC rules untuk `revenue_mgr` dan `gm_admin`
  - [x] Table-driven tests di `internal/api/router_test.go` (**88.2% coverage**)

- [x] **Fase 4: End-to-End (E2E) Automation & Verification**
  - [x] Tambahkan skenario CRUD kamar pada `testing/e2e/script/e2e_runner_test.go` (E2E-02E s/d E2E-02I)
  - [x] Tambahkan skenario CRUD kamar pada `testing/e2e/script/hotel_booking_rbac_e2e.sh`
  - [x] Buat laporan E2E di [`testing/e2e/report/2026-10-03-022000-catalog-crud-batch-b-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-03-022000-catalog-crud-batch-b-e2e-report.md)

- [x] **Fase 5: Verification & Commit Review**
  - [x] Jalankan `go vet ./...` (0 errors)
  - [x] Jalankan `go test -v ./...` (100% pass)
  - [x] Pelaporan ke user dan permintaan konfirmasi commit
