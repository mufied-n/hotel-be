# Walkthrough Tracker — Integrasi Payment Gateway Xendit & Notifikasi Resend
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **Completed & Verified (Ready for Git Commit)**
- **Feature Target:** Integrasi Payment Gateway Xendit Invoice v2 API, Webhook Token Verification, Pengiriman Notifikasi Outbox via Resend REST API, dan Graceful Fallback.

---

## Standar Rekayasa & Kepatuhan yang Ditegakkan

1. **Keamanan Transaksi & Pembayaran (PCI-DSS v4.0 SAQ A):**
   * Menggunakan Xendit Hosted Checkout (redirect URL), zero cardholder data on hotel premises.
2. **Keamanan API & Webhook (OWASP API Top 10):**
   * Constant-time comparison (`subtle.ConstantTimeCompare`) pada validasi header `x-callback-token`.
   * Penanganan webhook idempoten mencegah konfirmasi ganda pada replay callback.
3. **Perlindungan Data & Durabilitas Notifikasi (UU PDP No. 27/2022 & RFC 7540):**
   * Pengiriman email konfirmasi booking resmi dengan styling Pulang ke Uttara.
   * Header `Idempotency-Key` mencegah duplikasi email saat worker outbox retry.
4. **Anti-Overengineering (Prinsip Ponytail):**
   * Standard library `net/http` murni tanpa SDK pihak ketiga raksasa.
   * Graceful fallback ke `FakeGateway` dan `LogNotifier` jika kredensial belum diisi di development.

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset Mendalam Spesifikasi API & Standar**
  - [x] Riset endpoint Xendit Invoice v2 (`POST /v2/invoices`), Basic Auth, dan payload
  - [x] Riset verifikasi webhook `x-callback-token`
  - [x] Riset Resend REST API (`POST https://api.resend.com/emails`) dan `Idempotency-Key`
  - [x] Riset standar PCI-DSS SAQ A dan UU PDP No. 27/2022

- [x] **Fase 2: Dokumen Analisis & Spesifikasi**
  - [x] PRD: [`docs/prd/xendit-payment-resend-outbox-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/xendit-payment-resend-outbox-2026-10-03.md)
  - [x] SRS: [`docs/srs/xendit-payment-resend-outbox-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/xendit-payment-resend-outbox-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/xendit-payment-resend-outbox-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/xendit-payment-resend-outbox-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/xendit-payment-resend-outbox-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/xendit-payment-resend-outbox-walkthrough-2026-10-03.md)

- [x] **Fase 3: Implementasi Konfigurasi & Adapter (TDD)**
  - [x] Tambah field konfigurasi di `internal/platform/config.go`
  - [x] Tulis table-driven test Xendit Gateway `internal/adapter/payment/xendit_test.go`
  - [x] Implementasikan `internal/adapter/payment/xendit.go`
  - [x] Tulis table-driven test Resend Notifier `internal/adapter/notifier/resend_test.go`
  - [x] Implementasikan `internal/adapter/notifier/resend.go`
  - [x] Daftarkan webhook handler `POST /api/v1/webhooks/xendit` di `internal/api/router.go`
  - [x] Tulis table-driven test Webhook di `internal/api/webhook_test.go`
  - [x] Wire up di `cmd/server/main.go` dengan graceful fallback

- [x] **Fase 4: Verifikasi Unit Test & Coverage (Target $\ge 80\%$)**
  - [x] Verifikasi `go test -v -cover ./...` (Coverage: adapter/payment 87.7%, adapter/notifier 89.9%, api 87.2%, integration 80.5%)
  - [x] Verifikasi `go vet ./...` (0 errors)

- [x] **Fase 5: Pengujian E2E & Pembuatan Laporan**
  - [x] Buat skrip automasi E2E `testing/e2e/script/xendit_resend_e2e.sh`
  - [x] Jalankan skrip E2E dan catat bukti nyata (28 E2E test suites pass)
  - [x] Buat laporan di `testing/e2e/report/2026-10-03-xendit-resend-e2e-report.md`

- [x] **Fase 6: Verifikasi Selektif & Git Commit Integrity**
  - [x] Cek ulang `git status` (hanya stage files yang berkaitan dengan fitur Xendit & Resend)
  - [x] Konfirmasi siap commit dan siap lanjut ke task integrasi frontend berikutnya
