# Walkthrough Tracker — Batch BE-D: Checkout, Idempotensi & Pemulihan Pembayaran
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **In Progress (Fase 4: TDD Implementation)**
- **Feature Target:** Implementasi Profile Tamu Lengkap, Idempotensi Jaringan IETF, Ledger Pembayaran PCI-DSS SAQ A, dan Otoritas Hold Expiry (`BE-G07`, `BE-G09`, `BE-G11`, `BE-G12`)

---

## Standar Kepatuhan & Rekayasa yang Ditegakkan

1. **Hukum & Privasi Data (UU PDP No. 27/2022 & GDPR):**
   - Pasal 20 & 21 UU PDP: Dasar pemrosesan sah berbasis *explicit consent* (`terms_accepted: true`, `privacy_accepted: true`).
   - Nomor telepon E.164 (`guest_phone`) diperlakukan sebagai PII terlindungi, dimasking di `PublicDTO`, dan tidak dicatat dalam access log plaintext.
2. **Keamanan API (OWASP API Security Top 10 & ISO/IEC 27001):**
   - API4: Pembatasan panjang `special_requests` maksimal 500 karakter dengan sanitasi string.
   - Standar IETF `draft-ietf-httpapi-idempotency-key-header`: Hash SHA-256, pendeteksian payload mismatch (409 Conflict), isolasi key per tamu.
3. **Standar Finansial (PCI-DSS v4.0 SAQ A & PBI Sistem Pembayaran):**
   - Model Hosted Checkout / Redirect URL: Server hotel tidak menerima, memproses, atau menyimpan nomor kartu kredit/CVV.
   - Mata uang wajib Rupiah (IDR) dalam unit integer minor minor unit tanpa float drift.
   - Buku besar `payment_attempts` mencatat riwayat transaksi gateway dan status webhook secara terverifikasi.
4. **Standar Industri Perhotelan (HTNG & Hotel Bintang 4 Yogyakarta):**
   - Penafian resmi: *"Special requests are subject to availability upon check-in and cannot be guaranteed"*.
   - Format waktu kedatangan 24 jam `HH:MM`.
   - Otoritas batas waktu hold 30 menit di server (`expires_at` & `server_time`) guna mencegah overbooking pada 95 kamar fisik.
5. **Anti-Overengineering (Ponytail):**
   - Penggunaan paket standar Go (`crypto/sha256`, `regexp`, `uuid`) tanpa dependensi pihak ketiga baru.

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset Standar Internasional, Nasional & Legal**
  - [x] Riset kepatuhan UU PDP No. 27/2022 Pasal 20-21 (Consent & PII Contact Protection)
  - [x] Riset standar PCI-DSS v4.0 SAQ A (Hosted Checkout model & audit ledger)
  - [x] Riset spesifikasi IETF `draft-ietf-httpapi-idempotency-key-header`
  - [x] Riset standar HTNG untuk special request & format kedatangan hotel bintang 4

- [x] **Fase 2: Dokumen Analisis & Spesifikasi**
  - [x] PRD: [`docs/prd/checkout-idempotency-payment-batch-d-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/checkout-idempotency-payment-batch-d-2026-10-03.md)
  - [x] SRS: [`docs/srs/checkout-idempotency-payment-batch-d-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/checkout-idempotency-payment-batch-d-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/checkout-idempotency-payment-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/checkout-idempotency-payment-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/checkout-idempotency-payment-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/checkout-idempotency-payment-walkthrough-2026-10-03.md)

- [x] **Fase 3: Migrasi Database (`migrations/00007_checkout_idempotency_ledger.sql`)**
  - [x] Tambahkan kolom `guest_phone`, `estimated_arrival_time`, `special_requests`, `expires_at` pada tabel `bookings`
  - [x] Buat tabel `idempotency_keys` dengan index `expires_at`
  - [x] Buat tabel `payment_attempts` dengan index `booking_id` dan `provider_reference`

- [x] **Fase 4: TDD Implementasi Domain & Services (`internal/booking`, `internal/api`)**
  - [x] Validasi data tamu: regex format E.164, format HH:MM, limit 500 chars special requests
  - [x] Update `PublicDTO` untuk memuat `special_requests` & `estimated_arrival_time`, sembunyikan `guest_phone`
  - [x] Implementasi store & middleware/evaluator `IdempotencyStore`
  - [x] Update use case `Create` mengembalikan `ExpiresAt` dan `ServerTime`
  - [x] Update use case `Confirm` memvalidasi hold expiration (`ErrHoldExpired` jika `now > expires_at`)
  - [x] Implementasi pencatatan attempt pada `payment_attempts`
  - [x] Table-driven unit tests ($\ge 80\%$ coverage)

- [x] **Fase 5: End-to-End (E2E) Automation & Verification**
  - [x] Skenario E2E Idempotency Key (replay same key -> 201 identik; same key different body -> 409 Conflict)
  - [x] Skenario E2E Data Tamu Lengkap (E.164, special request 500 chars)
  - [x] Skenario E2E Late Payment Rejection (409 HOLD_EXPIRED saat bayar setelah hold lewat)
  - [x] Skenario E2E Payment Attempt Ledger
  - [x] Laporan E2E di `testing/e2e/report/`

- [x] **Fase 6: Verification & Commit Integrity**
  - [x] `go vet ./...` (0 errors)
  - [x] `go test -v ./...` (100% pass)
  - [x] Verifikasi perubahan sebelum commit
