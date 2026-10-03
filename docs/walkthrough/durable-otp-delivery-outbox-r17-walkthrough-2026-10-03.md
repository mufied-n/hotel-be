# Walkthrough: Pengiriman OTP Andal Berbasis Outbox (BE-R17)

**Nomor Dokumen:** WT-PULANG-BE-R17  
**Tanggal:** 3 Oktober 2026  
**Status:** Completed (Stage 1 s/d Stage 6 Siap Review)  
**Author:** AI Engineering Agent  
**Terkait:** [`BE-R17`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md), BE-G16, F02, F11  

---

## 1. Rencana Pelaksanaan & Checklist

- [x] **Tahap 1: Riset Mendalam**
  - [x] Analisis gap `BE-R17`: OTP delivery tidak memiliki retry durable, silent delivery failures, kolisi idempotency key, dan premature delivery claims.
  - [x] Analisis Transactional Outbox Pattern dan idempotency key per challenge ID.
- [x] **Tahap 2: Analisis & Dokumen Siklus Hidup**
  - [x] PRD: [`docs/prd/durable-otp-delivery-outbox-r17-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/durable-otp-delivery-outbox-r17-2026-10-03.md)
  - [x] SRS: [`docs/srs/durable-otp-delivery-outbox-r17-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/durable-otp-delivery-outbox-r17-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/durable-otp-delivery-outbox-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/durable-otp-delivery-outbox-architecture-2026-10-03.md)
- [x] **Tahap 3: Walkthrough Tracking**
  - [x] Inisialisasi dokumen pelacak progres ini.
- [x] **Tahap 4: Implementasi TDD (Target Coverage $\ge$ 80%)**
  - [x] Tambahkan field `PlainCode` (`json:"-"`) pada `Challenge` di [`internal/guest/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/model.go).
  - [x] Update `CreateChallengeWithCooldown` di [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go) untuk menyisipkan rekaman outbox `guest.otp_dispatch` secara atomik dalam transaksi database yang sama, serta menambahkan metode `HasOutbox`.
  - [x] Update `RequestChallenge` di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go) untuk mendelegasikan pengiriman ke outbox relay dan hanya melakukan inline notifier sebagai fallback test store.
  - [x] Tambahkan variadic `challengeID ...string` pada `SendGuestOTP` di [`internal/guest/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/service.go), [`internal/adapter/notifier/resend.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/adapter/notifier/resend.go), dan [`internal/adapter/notifier/log.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/adapter/notifier/log.go) untuk mengatur header `Idempotency-Key: otp-challenge-<challengeID>`.
  - [x] Buat fungsi handler outbox `NewGuestOTPDispatchHandler` di [`internal/workers/outbox.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/workers/outbox.go) dengan pengecekan kedaluwarsa kode dan status verifikasi.
  - [x] Hubungkan handler `guest.otp_dispatch` pada `OutboxRelay` di [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go).
  - [x] Perbarui respons pesan di [`internal/api/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/guest_auth.go) agar jujur (`delivery_status: accepted` dan pesan tidak menjanjikan pengiriman instan).
  - [x] Table-driven tests di `internal/workers/outbox_test.go`, `internal/adapter/notifier/resend_test.go`, `internal/guest/service_test.go`, dan `internal/api/guest_api_test.go`.
  - [x] Verifikasi `go test -v -cover ./...` dan `go vet ./...` (Zero issues).
- [x] **Tahap 5: End-to-End (E2E) Testing**
  - [x] Buat skrip automasi pengujian: [`testing/e2e/script/durable_otp_delivery_r17_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/durable_otp_delivery_r17_e2e.sh).
  - [x] Uji skenario outbox queuing saat challenge dibuat.
  - [x] Uji penyelesaian pengiriman oleh outbox worker (`status: done`).
  - [x] Uji deduplikasi pengiriman dengan `Idempotency-Key` unik berbasis challenge ID.
  - [x] Uji proteksi kode kedaluwarsa (outbox discard stale OTP).
  - [x] Uji proteksi kode yang sudah diverifikasi sebelumnya.
  - [x] Uji login verifikasi menggunakan kode OTP dari outbox.
  - [x] Seluruh 16 asersi lolos (100% PASS).
  - [x] Dokumentasikan di [`testing/e2e/report/2026-10-04-001500-durable-otp-delivery-r17-e2e-report.md`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-04-001500-durable-otp-delivery-r17-e2e-report.md).
- [ ] **Tahap 6: Review & Final Report**
  - [x] Update manifest audit gap [`docs/gap/07-backend-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/07-backend-reaudit-2026-10-03.md) & [`docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md).
  - [ ] Minta konfirmasi user dan lakukan git commit.

---

## 2. Log Eksekusi & Catatan Modifikasi File

1. **`internal/guest/model.go`**:
   - Menambahkan field `PlainCode string `json:"-"`` pada struct `Challenge` agar kode OTP dapat diteruskan ke outbox tanpa bocor ke JSON/log publik.
2. **`internal/guest/postgres.go`**:
   - Menambahkan penyisipan event ke tabel `outbox` dengan topik `guest.otp_dispatch` di dalam transaksi database yang sama dengan pembuatan tantangan OTP.
   - Mengimplementasikan `HasOutbox() bool` pada `PostgresStore`.
3. **`internal/guest/service.go`**:
   - Memperbarui `RequestChallenge` agar mengandalkan outbox relay jika store mendukung `OutboxStore`, serta fallback ke inline notifier untuk mock testing.
   - Memperbarui interface `OTPNotifier.SendGuestOTP` untuk menerima parameter variadic `challengeID ...string`.
4. **`internal/adapter/notifier/resend.go` & `internal/adapter/notifier/log.go`**:
   - Menggunakan format `Idempotency-Key: otp-challenge-<challengeID>` bila `challengeID` tersedia, mencegah tabrakan pada permintaan paralel dan menjamin deduplikasi saat worker me-retry pesan.
5. **`internal/workers/outbox.go`**:
   - Menambahkan `NewGuestOTPDispatchHandler` yang memproses event outbox, memvalidasi masa berlaku kode OTP (anti-stale delivery), mengecek apakah kode sudah diverifikasi, dan memanggil notifier.
6. **`cmd/server/main.go`**:
   - Mendaftarkan handler `guest.otp_dispatch` pada `OutboxRelay`.
7. **`internal/api/guest_auth.go`**:
   - Memperbarui respons endpoint `POST /api/v1/auth/guest/challenge` dengan `"delivery_status": "accepted"` dan pesan yang jujur netral tanpa mengklaim pengiriman telah selesai seketika.
8. **Pengujian**:
   - Table-driven unit tests di `internal/workers/outbox_test.go`, `internal/adapter/notifier/resend_test.go`, `internal/guest/service_test.go`, dan `internal/api/guest_api_test.go`.
   - E2E testing di [`testing/e2e/script/durable_otp_delivery_r17_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/durable_otp_delivery_r17_e2e.sh) dengan 16 asersi (100% PASS).
