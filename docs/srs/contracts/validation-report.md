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
