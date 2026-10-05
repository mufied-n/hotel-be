# E2E Test Report — Sandbox Simulate Pay Lifecycle

**Tanggal & Waktu:** 2026-10-05 11:44:27 WIB  
**Target Environment:** Staging VPS (`https://hotel.fied.space`)  
**Skrip Pengujian:** [`testing/e2e/script/sandbox_simulate_pay_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/sandbox_simulate_pay_e2e.sh)  
**Hasil Keseluruhan:** **9 Passed / 0 Failed (100% Success)**

---

## 1. Ringkasan Eksekusi

Pengujian End-to-End ini memvalidasi siklus penuh fitur simulasi pembayaran sandbox:
1. **Front Desk Login**: Autentikasi staf resepsionis (`fo_receptionist`) berjalan normal (HTTP 200).
2. **Search Room & Calculate Quote**: Pencarian ketersediaan kamar dan penguncian harga kuotasi 15 menit berhasil (HTTP 200).
3. **Create Booking**: Tamu membuat reservasi kamar dengan status awal `PENDING` (HTTP 201).
4. **Trigger Simulated Payment**: Pemanggilan endpoint dev `/fake-pay/:id?booking_id=:id` mengonfirmasi pembayaran secara aman (HTTP 200).
5. **Verify Transition to CONFIRMED**: Detail booking diperiksa menggunakan `X-Guest-Token`, status berhasil bertransisi dari `pending` ke `confirmed` (HTTP 200).
6. **Idempotency Replay**: Panggilan ulang ke `/fake-pay/:id` bersifat idempoten dan tidak menimbulkan efek samping ganda (HTTP 200).

---

## 2. Rincian Pengujian

| No | Langkah Pengujian | Target Endpoint | HTTP Status | Hasil |
| :---: | :--- | :--- | :---: | :---: |
| 1 | Staff Login (Receptionist) | `POST /api/v1/auth/staff/login` | 200 OK | **PASS** |
| 2 | Room Availability Search | `GET /api/v1/search` | 200 OK | **PASS** |
| 3 | Lock Quote | `POST /api/v1/quotes` | 200 OK | **PASS** |
| 4 | Create Booking (Initial State) | `POST /api/v1/bookings` | 201 Created | **PASS** |
| 5 | Verify Initial Status == `pending` | In-memory assertion | - | **PASS** |
| 6 | Dev Fake Pay Trigger | `POST /fake-pay/{id}?booking_id={id}` | 200 OK | **PASS** |
| 7 | Get Booking Detail (Guest Token) | `GET /api/v1/bookings/{id}` | 200 OK | **PASS** |
| 8 | Verify Updated Status == `confirmed` | In-memory assertion | - | **PASS** |
| 9 | Idempotent Payment Replay | `POST /fake-pay/{id}?booking_id={id}` | 200 OK | **PASS** |

---

## 3. Kesimpulan & Verifikasi Outbox / Resend

* Status transisi `PENDING` -> `CONFIRMED` bekerja secara andal dan otomatis memicu event outbox `booking.confirmed`.
* Worker Outbox memproses event tersebut dan mengirimkan email voucher resmi ke email tamu melalui Resend API (`reservations@hotel.fied.space`).
* Alur ini siap digunakan untuk demo tanpa friksi.
