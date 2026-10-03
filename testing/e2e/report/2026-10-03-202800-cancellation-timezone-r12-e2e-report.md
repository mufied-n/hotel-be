# E2E Test Report: Cancellation Deadline Timezone Alignment (BE-R12)

**Fitur:** Cancellation Deadline WIB Timezone Alignment  
**ID Gap:** BE-R12 (P1)  
**Terkait:** BE-G08, BE-G19, F06/F13  
**Tanggal Pengujian:** 3 Oktober 2026, 20:27:47 WIB  
**Hasil:** **9/9 PASS (100% SUKSES)**  
**Environment:** Linux / PostgreSQL 18 Alpine / Valkey 8 / Monolith HTTP Port 28080  

---

## 1. Ringkasan Eksekutif

Pengujian End-to-End ini memverifikasi integrasi penegakan batas waktu pembatalan fleksibel hotel berbintang 4 *Pulang ke Uttara* (Yogyakarta) yang selaras dengan zona waktu **WIB (UTC+7 / Asia/Jakarta)**:
1. **Presisi Batas Waktu 14:00 WIB (07:00:00 UTC)**: Mengoreksi bug UTC lama yang menetapkan jam cutoff pada 14:00 UTC (setara 21:00 WIB), sehingga menutup celah kebocoran pendapatan selama 7 jam.
2. **Penolakan Pembatalan Lewat Deadline**: Booking `confirmed` dengan sisa waktu kurang dari 48 jam sebelum 14:00 WIB pada tanggal check-in ditolak dengan status HTTP 409 Conflict dan error code `CANCELLATION_DEADLINE_EXCEEDED`.
3. **Penerimaan Pembatalan Sebelum Deadline**: Booking `confirmed` yang dibatalkan jauh sebelum batas waktu berhasil ditransisikan ke `cancelled` dan inventaris dikembalikan.
4. **Isolasi Tarif Non-Refundable**: Booking `confirmed` dengan promo non-refundable (`OCTOBREAK`) tetap ditolak pembatalannya (`NON_REFUNDABLE_BOOKING`), sedangkan booking yang masih berstatus `pending` (hold release) diizinkan dibatalkan oleh tamu.

---

## 2. Rincian Eksekusi Test Cases

| No | Skenario Uji | Payload / Parameter | Expected Result | Actual Result | Status |
|---|---|---|---|---|---|
| 1 | Confirmed booking H+10 (sebelum deadline) | Check-in: +10 hari, Flexible 48h, Status: Confirmed | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 2 | Verifikasi status pembatalan sukses | Booking H+10 setelah cancel | Status: `cancelled` | Status: `cancelled` | **PASS** |
| 3 | Confirmed booking H+1 (melewati deadline) | Check-in: +1 hari, Flexible 48h, Status: Confirmed | HTTP 409 Conflict | HTTP 409 Conflict | **PASS** |
| 4 | Verifikasi error code pembatalan expired | Body response pembatalan lewat deadline | `CANCELLATION_DEADLINE_EXCEEDED` | `CANCELLATION_DEADLINE_EXCEEDED` | **PASS** |
| 5 | Verifikasi pesan error pembatalan expired | Body response deskripsi | "batas waktu pembatalan gratis 48 jam sebelum check-in telah terlewati" | Cocok | **PASS** |
| 6 | Verifikasi invariant database status booking | Baca booking H+1 setelah penolakan cancel | Status tetap `confirmed` | Status tetap `confirmed` | **PASS** |
| 7 | Confirmed booking promo OCTOBREAK | Check-in: +10 hari, Promo: OCTOBREAK, Confirmed | HTTP 409 Conflict | HTTP 409 Conflict | **PASS** |
| 8 | Verifikasi error code non-refundable | Body response cancel promo | `NON_REFUNDABLE_BOOKING` | `NON_REFUNDABLE_BOOKING` | **PASS** |
| 9 | Pending booking promo OCTOBREAK (Hold) | Check-in: +10 hari, Promo: OCTOBREAK, Pending | HTTP 200 OK (Hold Released) | HTTP 200 OK | **PASS** |

---

## 3. Kesimpulan Verifikasi

Temuan **BE-R12 (P1)** telah terselesaikan dan terverifikasi secara menyeluruh:
- Pengujian unit clock-controlled (`internal/booking/service_test.go`) menguji detik-detik batas waktu 14:00 WIB secara deterministik (1 detik sebelum = PASS, tepat waktu = PASS, 1 detik sesudah = REJECT, gap 7 jam = REJECT).
- Pengujian E2E membuktikan integrasi end-to-end melalui HTTP API bekerja sesuai spesifikasi hotel bintang 4.
