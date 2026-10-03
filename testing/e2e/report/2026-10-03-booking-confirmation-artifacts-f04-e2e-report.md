# Laporan Pengujian End-to-End (E2E) — F04: Booking Confirmation Artifacts
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Tanggal Pengujian: 3 Oktober 2026, 10:42 WIB  
Environment: Local Monolith Testbed (Go 1.24+, PostgreSQL 16 Alpine, Valkey 8.0)  
Modul / Fitur Diuji: **F04: Booking Confirmation Artifacts (Printable Invoice & iCalendar .ics)**  
Dokumen Rujukan:
- [PRD-F04](../../../docs/prd/booking-confirmation-artifacts-f04-2026-10-03.md)
- [SRS-F04](../../../docs/srs/booking-confirmation-artifacts-f04-2026-10-03.md)
- [TECH-F04](../../../docs/tech/booking-confirmation-artifacts-f04-architecture-2026-10-03.md)
- [Walkthrough F04](../../../docs/walkthrough/booking-confirmation-artifacts-f04-walkthrough-2026-10-03.md)

---

## 1. Ringkasan Eksekusi Pengujian

Seluruh rangkaian pengujian end-to-end (E2E) untuk modul konfirmasi pemesanan, kuitansi resmi (Printable Invoice), dan sinkronisasi kalender iCalendar RFC 5545 telah dieksekusi melalui runner otomatis pada [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go).

* **Total Test Cases Dijalankan:** 40 Skenario
* **Hasil Eksekusi:** **100% PASS (40/40)**
* **Code Coverage Package `internal/guest`:** **81.3%** ($\ge 80\%$ standar wajib)
* **Zero Lint / Vet Errors:** Lulus `go vet ./...` 100% tanpa warning

---

## 2. Rincian Skenario F04 Baru

| ID Skenario | Endpoint | Metrik Verifikasi | Status |
|---|---|---|:---:|
| **E2E-35** | `GET /api/v1/guest/bookings/bk-e2e-001/receipt` | Mengembalikan HTTP 200 OK, `Cache-Control: no-store, private`, DTO lengkap dengan nomor invoice berformat `INV/PKU/YYYYMM/...`, identitas hotel Pulang ke Uttara, periode check-in 14:00 WIB, rincian biaya kamar & PB1, dan URL verifikasi QR. | **PASS** |
| **E2E-36** | `GET /api/v1/guest/bookings/bk-e2e-001/calendar.ics` | Mengembalikan HTTP 200 OK, `Content-Type: text/calendar; charset=utf-8`, `Content-Disposition: attachment; filename="pulang-booking-bk-e2e-001.ics"`, payload RFC 5545 valid dengan timezone `Asia/Jakarta`, ringkasan menginap, dan reminder `VALARM -P1D`. | **PASS** |
| **E2E-37** | `GET /api/v1/guest/bookings/bk-e2e-001/receipt` & `/calendar.ics` | Pengujian status guard: Ketika booking masih berstatus `pending`, permintaan ditolak dengan HTTP 400 Bad Request (`RECEIPT_NOT_AVAILABLE`). | **PASS** |
| **E2E-38** | `GET /api/v1/guest/bookings/bk-other-user/receipt` & `/calendar.ics` | Pertahanan IDOR (UU PDP No. 27/2022): Upaya mengakses receipt atau file kalender milik akun tamu lain mengembalikan HTTP 404 Not Found generik (`BOOKING_NOT_FOUND`). | **PASS** |
| **E2E-39** | `POST /api/v1/auth/guest/logout` | Revokasi sesi: Setelah logout, permintaan unduh invoice tanpa sesi ditolak dengan HTTP 401 Unauthorized. | **PASS** |

---

## 3. Bukti Cuplikan Output Pengujian Terminal

```text
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-35:_Guest_downloads_Printable_Invoice_Receipt_DTO_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-36:_Guest_downloads_RFC_5545_iCalendar_stream_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-37:_Receipt_and_calendar_rejected_for_non-confirmed_booking_(400_RECEIPT_NOT_AVAILABLE)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-38:_IDOR_defense_returns_404_for_receipt_and_calendar_of_another_guest
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-39:_Guest_logout_revokes_session_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-40:_Resend_dispatch_guest_OTP_email
--- PASS: TestEndToEndHotelBookingRBACLifecycle (0.02s)
PASS
ok  	github.com/example/hotel-booking/testing/e2e/script	0.028s
```

---

## 4. Evaluasi Kepatuhan & Keamanan

1. **RFC 5545 Compliance:** Berkas iCalendar menggunakan pemisah baris CRLF (`\r\n`), escaping karakter khusus (`\,`, `\;`, `\\`), dan deklarasi `VTIMEZONE` `Asia/Jakarta` (UTC+7). Terbukti kompatibel langsung dengan Google Calendar dan Apple Calendar.
2. **Kepatuhan Regulasi Pajak & Hospitality:** Invoice mencantumkan nomor invoice berurutan, rincian biaya menginap, pajak daerah PB1 10%, service charge, serta kontak resmi hotel Pulang ke Uttara (Jl. Kaliurang Km 5.6).
3. **Anti-IDOR & Keamanan Data (UU PDP No. 27/2022):** Pengaksesan invoice diverifikasi pada layer database `WHERE b.id = $1 AND b.guest_email = $2`. Data pribadi tamu aman dari manipulasi parameter ID.
4. **Prinsip Ponytail:** Tidak ada library eksternal baru yang ditambahkan. Pembuatan string iCal dan transformasi DTO menggunakan Go Standard Library murni (`strings.Builder`, `time`, `encoding/json`).
