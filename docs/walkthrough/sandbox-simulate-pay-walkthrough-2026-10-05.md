# Walkthrough — Simulasi Bayar Sandbox (Post-Checkout)

**Tanggal:** 2026-10-05 · **Terkait:** [PRD](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/sandbox-simulate-pay-2026-10-05.md), [SRS](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/sandbox-simulate-pay-2026-10-05.md), [Tech Architecture](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/sandbox-simulate-pay-architecture-2026-10-05.md)

---

## 1. Rangkuman Eksekusi

### Permasalahan Sebelumnya
* Setelah tamu menyelesaikan alur checkout (`review.vue`), sistem membuat booking dengan status `PENDING` (hold 30 menit).
* Di tahap sandbox/staging tanpa payment gateway live, payment URL lokal dummy (`http://localhost:8080/fake-pay/...`) disaring oleh BFF Nuxt demi keamanan di domain publik.
* Akibatnya, tombol bayar tidak muncul di `/booking/status/[id]`, tamu tidak menerima email, dan booking tersangkut sampai waktu hold kedaluwarsa.

### Solusi yang Diimplementasikan (Opsi B: Tombol Simulasi Bayar di Status Page)
1. **BFF Endpoint**: `POST /api/bff/bookings/[id]/simulate-pay`
   * Memvalidasi CSRF (`X-Pulang-CSRF`).
   * Memvalidasi kepemilikan sesi booking (`bookingAccess[id].token`).
   * Memvalidasi flag `public.sandboxPay`.
   * Meneruskan ke backend `POST /fake-pay/:id?booking_id=:id` untuk memicu konfirmasi idempotently.
2. **Frontend UI & State**:
   * Menambahkan tombol **"Simulasi Bayar Sekarang (Sandbox)"** pada [BookingStatusPanel.vue](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/components/booking/BookingStatusPanel.vue) saat booking berstatus `pending` atau `pending_payment`.
   * Menghubungkan event `@simulate="simulatePay"` pada [status/[id].vue](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/pages/booking/status/[id].vue) untuk memanggil API BFF dan merefresh data status secara instan.
   * Menambahkan method `simulatePay` pada [api-booking-client.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/services/api-booking-client.ts) dan kontrak [booking-client.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/services/booking-client.ts).
3. **Backend Integration**:
   * Memanfaatkan endpoint eksisting `/fake-pay/:ref` yang memanggil `bkSvc.Confirm()`.
   * Begitu booking bertransisi ke `CONFIRMED`, outbox relay mengeksekusi `SendBookingConfirmed` yang mengirim voucher HTML resmi ke email tamu melalui Resend (`reservations@hotel.fied.space`).
   * Event `booking.confirmed` otomatis disiarkan ke Front Desk SSE.

---

## 2. File yang Dibuat / Dimodifikasi

### Frontend (`mimiking-booking-secure`)
* [server/api/bff/bookings/[id]/simulate-pay.post.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/server/api/bff/bookings/[id]/simulate-pay.post.ts) (New)
* [nuxt.config.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/nuxt.config.ts) (Modified: added `sandboxPay` public config)
* [app/services/api-booking-client.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/services/api-booking-client.ts) (Modified: added `simulatePay`)
* [app/services/booking-client.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/services/booking-client.ts) (Modified: optional `simulatePay` in interface)
* [app/components/booking/BookingStatusPanel.vue](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/components/booking/BookingStatusPanel.vue) (Modified: added simulate payment button and emit)
* [app/pages/booking/status/[id].vue](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/app/pages/booking/status/[id].vue) (Modified: wired simulate handler)
* [tests/unit/simulate-pay.test.ts](file:///mnt/code/projects/jobs/pulang/mimiking-booking-secure/tests/unit/simulate-pay.test.ts) (New: table-driven unit tests)

### Backend & Dokumentasi (`current-booking`)
* [docs/prd/sandbox-simulate-pay-2026-10-05.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/sandbox-simulate-pay-2026-10-05.md) (New)
* [docs/srs/sandbox-simulate-pay-2026-10-05.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/sandbox-simulate-pay-2026-10-05.md) (New)
* [docs/tech/sandbox-simulate-pay-architecture-2026-10-05.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/sandbox-simulate-pay-architecture-2026-10-05.md) (New)
* [docs/walkthrough/sandbox-simulate-pay-walkthrough-2026-10-05.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/sandbox-simulate-pay-walkthrough-2026-10-05.md) (New)
* [testing/e2e/script/sandbox_simulate_pay_e2e.sh](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/sandbox_simulate_pay_e2e.sh) (New)
* [testing/e2e/report/2026-10-05-113700-sandbox-simulate-pay-e2e-report.md](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/report/2026-10-05-113700-sandbox-simulate-pay-e2e-report.md) (New)

---

## 3. Log Pengujian & Validasi

1. **Frontend Lint & Typecheck**:
   ```bash
   npx eslint server/api/bff/bookings app/components/booking/BookingStatusPanel.vue app/pages/booking/status/[id].vue app/services nuxt.config.ts
   npx nuxt typecheck
   # Output: Exit code 0 (Pass)
   ```
2. **Frontend Vitest Table-Driven Tests**:
   ```bash
   npx vitest run tests/unit/simulate-pay.test.ts
   # Output: 3 passed (100%)
   ```
3. **Backend Go Vet & Tests**:
   ```bash
   go vet ./...
   go test -v -cover ./...
   # Output: 82/82 test suites passed, 0 lint errors
   ```
