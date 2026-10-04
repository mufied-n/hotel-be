# E2E Test Report: Candidate A — Dynamic Rates, Room Allotment & Stop-Sell Engine

- **Tanggal / Waktu:** 2026-10-04 05:10:00 WIB
- **Target Fitur:** Candidate A — Dynamic Rates, Room Allotment & Stop-Sell Engine (F08/F09/F10)
- **Komponen Pengujian:**
  - [`migrations/00019_rate_calendar_and_promos.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00019_rate_calendar_and_promos.sql) (Skema tabel `rate_calendar_overrides`, `promo_campaigns`, seeding promo default `OCTOBREAK`, dan Casbin RBAC rules untuk `revenue_mgr`, `gm_admin`, `receptionist`)
  - [`internal/rates/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/model.go) (Model domain `CalendarOverride`, `PromoCampaign`, request `BulkCalendarUpdateRequest`, error constants `ErrStopSellApplied`, `ErrClosedToArrival`, `ErrMinLengthOfStay`, dll.)
  - [`internal/rates/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/service.go) (`Service`/`Engine` kalkulasi harga terkunci, restriksi kalender harian, dynamic promo quota atomic check & stay constraint enforcement)
  - [`internal/rates/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/postgres.go) (PostgreSQL store untuk kalender tarif rekonsiliasi hari renggang via `generate_series`, `pgx.Batch` bulk upsert, dan reservasi kuota promo atomic)
  - [`internal/rates/store.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/store.go) (Thread-safe in-memory store untuk eksekusi testing mandiri)
  - [`internal/rates/valkey.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/valkey.go) (Adapter Redis/Valkey untuk penyimpanan kuotasi terkunci 15 menit)
  - [`internal/api/http/handler/revenue.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go) (5 HTTP endpoint manajemen revenue: `GetRevenueCalendar`, `BulkUpdateCalendar`, `ListPromos`, `CreatePromo`, `UpdatePromo`)
  - [`internal/api/http/routes.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go) (Pendaftaran rute API `/revenue/*` di bawah proteksi Casbin RBAC)
  - [`internal/api/http/handler/errors.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/errors.go) (Pemetaan kode domain error ke RFC 7807: `ROOM_STOP_SELL`, `CLOSED_TO_ARRIVAL`, `MIN_LENGTH_OF_STAY_VIOLATED`, `PROMO_MIN_STAY_VIOLATED`, dll.)
  - [`internal/booking/search.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/search.go) (`SearchAvailability` mapping alasan unavailable kamar ke `STOP_SELL`, `CLOSED_TO_ARRIVAL`, `MIN_LENGTH_OF_STAY_VIOLATED`)
  - [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go) (Wiring dependency injection `CalendarStore` dan `PromoStore` ke `rateEngine` dan `apihttp.Deps`)
- **Lingkungan Pengujian:**
  - In-Process Go E2E Runner: [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) (`E2E-73` dan `E2E-74`)
  - Ephemeral Live HTTP Server: Port 28091 (`127.0.0.1:28091`) dijalankan via background Go test server
  - Database: PostgreSQL 18 container (`current-booking-postgres-1`) dengan Goose migration versi 19
- **Skrip Eksekusi:** [`testing/e2e/script/dynamic_rates_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/dynamic_rates_e2e.sh)

---

## 1. Ringkasan Eksekusi

| Metrik | Target | Nilai Realisasi | Status |
| :--- | :--- | :--- | :--- |
| **Total Asersi Otomatis** | - | 26 asersi | 100% PASS |
| **Asersi Go In-Process** | - | 2 sub-test (`E2E-73` & `E2E-74`) | PASS |
| **Asersi Live HTTP Ephemeral** | - | 24 asersi via curl & real socket | PASS |
| **Gagal / Error** | 0 | 0 | CLEAN |
| **Statement Coverage (`internal/rates`)** | $\ge 80\%$ | **93.8%** | MEMENUHI SYARAT |
| **Statement Coverage (`internal/api/http`)** | $\ge 80\%$ | **96.5%** | MEMENUHI SYARAT |
| **Statement Coverage (`internal/api/http/handler`)** | $\ge 80\%$ | **81.8%** | MEMENUHI SYARAT |
| **Statement Coverage (`internal/api/http/middleware`)** | $\ge 80\%$ | **84.8%** | MEMENUHI SYARAT |
| **Data Race Detection (`-race`)** | 0 races | 0 data races | CLEAN |
| **Static Analysis (`go vet ./...`)** | 0 issues | 0 issues | CLEAN |

---

## 2. Rincian Skenario Pengujian

### Skenario 1: Go In-Process E2E Suite (E2E-73 & E2E-74)
- **Komponen Diuji:** [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go)
- **Eksekusi:**
  - `E2E-73`: Revenue Management - Stop-Sell & MinLOS Restrictions (`PASS`)
  - `E2E-74`: Dynamic Promo Campaigns Lifecycle & Validation (`PASS`)

### Skenario 2: Query Kalender Tarif & RBAC Authorization (FR-08)
- **Pengujian:**
  - `GET /api/v1/revenue/calendar` tanpa parameter tanggal $\rightarrow$ HTTP 400 Bad Request (`MISSING_DATE_RANGE`).
  - `GET /api/v1/revenue/calendar?start_date=2026-10-10&end_date=2026-10-15` oleh `revenue_mgr` $\rightarrow$ HTTP 200 OK dengan daftar kalender harian.
  - `GET /api/v1/revenue/calendar` oleh `receptionist` $\rightarrow$ HTTP 200 OK (Staf Front Desk diizinkan melihat kalender tarif).
  - `GET /api/v1/revenue/calendar` oleh publik/guest tanpa bearer token $\rightarrow$ HTTP 403 Forbidden (Casbin RBAC fail-closed).

### Skenario 3: Bulk Update Stop-Sell & Proteksi Pencarian Publik (FR-01)
- **Pengujian:**
  - Revenue Manager mengirim `PUT /api/v1/revenue/calendar/bulk` dengan payload:
    ```json
    {
      "room_type_ids": ["01900000-0000-7000-8000-000000000001"],
      "start_date": "2026-10-20",
      "end_date": "2026-10-20",
      "rate_plan_code": "RO",
      "is_stop_sell": true
    }
    ```
    Respons: HTTP 200 OK, `{"status":"success","affected_records":1}`.
  - Guest mencari kamar melalui `GET /api/v1/search?check_in=2026-10-20&check_out=2026-10-21&rooms=1&adults=1`:
    Respons: HTTP 200 OK, kamar Superior King ditandai `available: false` dan `unavailable_reason: "STOP_SELL"`.
  - Guest mencoba mengunci penawaran harga via `POST /api/v1/quotes` pada tanggal 2026-10-20:
    Respons: HTTP 400 Bad Request, `{"code":"ROOM_STOP_SELL","message":"penjualan kamar ditutup sementara untuk tanggal yang dipilih"}`.

### Skenario 4: Penegakan Batas Minimum Length of Stay (MinLOS) (FR-08)
- **Pengujian:**
  - Revenue Manager mengatur `min_los: 3` pada rentang tanggal 2026-10-25 s.d 2026-10-27.
  - Guest mencoba melakukan quote untuk durasi 1 malam (`check_in: 2026-10-25`, `check_out: 2026-10-26`).
  - Respons: HTTP 400 Bad Request, `{"code":"MIN_LENGTH_OF_STAY_VIOLATED","message":"durasi menginap kurang dari batas minimum yang ditentukan"}`.

### Skenario 5: Lifecycle Kampanye Promo Dinamis (FR-09)
- **Pengujian:**
  - Revenue Manager membuat promo baru `E2ESPECIAL20` (diskon 20%, max diskon Rp 400.000, min stay 2 malam, kuota 10) via `POST /api/v1/revenue/promos`:
    Respons: HTTP 201 Created.
  - Guest quote 1 malam dengan kode `E2ESPECIAL20`:
    Respons: HTTP 400 Bad Request, `{"code":"PROMO_MIN_STAY_VIOLATED"}`.
  - Guest quote 2 malam dengan kode `E2ESPECIAL20`:
    Respons: HTTP 200 OK dengan pemotongan diskon yang terkunci di snapshot harga (`discount_minor`).
  - Revenue Manager menonaktifkan kode promo via `PUT /api/v1/revenue/promos/:id`:
    Respons: HTTP 200 OK (`is_active: false`).
  - Guest mencoba quote kembali dengan kode promo yang dinonaktifkan:
    Respons: HTTP 400 Bad Request, `{"code":"PROMO_EXPIRED"}`.

---

## 3. Kesimpulan Verifikasi

Seluruh kriteria penerimaan (*Acceptance Criteria*) Candidate A yang ditentukan pada dokumen PRD ([`docs/prd/dynamic-rates-and-stop-sell-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/dynamic-rates-and-stop-sell-2026-10-04.md)) dan SRS ([`docs/srs/dynamic-rates-and-stop-sell-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/dynamic-rates-and-stop-sell-2026-10-04.md)) telah terverifikasi secara tuntas, aman dari data race, lulus uji statis tanpa warning, dan mencapai tingkat *code coverage* $\ge 80\%$ pada seluruh modul terkait.
