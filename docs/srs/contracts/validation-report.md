# Validasi paket PRD/SRS per fitur

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Scope: 15 pasangan PRD/SRS + registries + shared contract + OpenAPI target proposal.

- Redocly lint OpenAPI 3.1: PASS, zero errors; satu warning info-license karena lisensi spesifikasi belum ditetapkan owner. Tidak menambahkan lisensi fiktif.
- Prism mock GET /hotel: HTTP200 dengan metadata contoh sesuai schema.
- Prism mock POST /booking-searches payload valid: HTTP200 dengan search result + pagination contoh.
- Prism mock POST /booking-searches missing fields/invalid date: HTTP400 validation-error.
- Prism memakai paths tanpa server prefix /api/v1. Permintaan pertama memakai prefix menghasilkan404 route mock; permintaan ulang menggunakan path yang benar lulus. Ini bukan pengujian route Go.
- Tautan lokal/PRD-SRS pairing, 15 fitur dan 105 FR/AC, serta dependency graph diperiksa sebelum penyalinan.

Perintah:

```bash
npm_config_cache=/tmp/npm-cache-pulang-specs npx --yes @redocly/cli lint booking-roadmap-proposal.openapi.json
npm_config_cache=/tmp/npm-cache-pulang-specs npx --yes @stoplight/prism-cli mock booking-roadmap-proposal.openapi.json --host 127.0.0.1 --port 4010
```

Lint/mock hanya memvalidasi struktur dan contoh. Tidak membuktikan server authorization, OTP delivery, quote arithmetic, real DB locking/rollback, provider signing/payment/refund, device QA atau readiness hotel. Tidak ada request vendor/Go/payment nyata.
Approved existing katalog/Batch C/RBAC/Batch D dipertahankan. Batch D/source backend sedang berubah saat inspeksi, sehingga status implementasi/gap terkini harus diverifikasi saat eksekusi.

---

## Validasi Kontrak Kanonikal OpenAPI 3.1 (5 Oktober 2026)

Tanggal: 5 Oktober 2026 (Asia/Jakarta).  
File: [`docs/srs/contracts/pulang-hotel-booking.openapi.json`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/contracts/pulang-hotel-booking.openapi.json)  
Laporan Lengkap: [`docs/srs/contracts/api-spec-audit-report-2026-10-05.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/contracts/api-spec-audit-report-2026-10-05.md)

### Hasil Validasi
1. **Redocly CLI Lint**:
   ```bash
   npx --yes @redocly/cli lint docs/srs/contracts/pulang-hotel-booking.openapi.json
   ```
   * **Status**: **PASS (0 errors, 0 warnings)**.
   * Format: OpenAPI 3.1.0 resmi dengan skema skalar RFC 7807 ProblemDetails, skema DTO agregat Booking, RoomType, dan FeatureFlag.
2. **Pencocokan Paritas Rute (Router vs OpenAPI)**:
   * **Router Production Golden**: 69 operasi (`internal/api/http/testdata/routes.golden`).
   * **OpenAPI Paths & Operations**: 69 operasi (61 endpoints).
   * **Keselarasan**: **100% Exact Match** (0 rute hilang, 0 rute ekstra).
3. **Cakupan Fitur Terverifikasi**:
   * Health Probes (4)
   * Webhook Xendit & OTA HMAC Webhook (2)
   * Guest Auth & Passwordless OTP (4)
   * Staff Auth & Session (3)
   * Room Catalog & Availability Search (7)
   * Bookings Core & Check-in/Check-out (10)
   * Guest Portal Privat & Refund Status (11)
   * Front Desk Roster, Handover & SSE Stream (7)
   * Mid-Stay Room Move & Extend Stay (3)
   * Housekeeping Status & Out-of-Order (3)
   * Finance Reconciliation & Disputed Cases (4)
   * Dynamic Revenue Rates Calendar & Promos (5)
   * OTA Channel Sync Issues & 1-Click Upgrade (3)
   * Runtime Feature Flags Admin (2)
   * Sandbox Payment Simulator Fake-Pay (1)

