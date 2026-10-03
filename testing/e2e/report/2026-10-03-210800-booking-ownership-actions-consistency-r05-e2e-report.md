# E2E Test Report: Booking Ownership & Allowed Actions Consistency (BE-R05)

**Fitur:** Canonical Email Normalization & Policy-Driven Allowed Actions  
**Gap ID:** BE-R05 (P1)  
**Terkait:** BE-G08, BE-R02, BE-R12, F03 (My Bookings)  
**Tanggal Pengujian:** 3 Oktober 2026, 21:06:29 WIB  
**Hasil:** **19/19 PASS (100% SUKSES)**  
**Environment:** Linux / PostgreSQL 18 Alpine / Valkey 8 / Monolith HTTP Port 28080  

---

## 1. Ringkasan Eksekutif

Pengujian End-to-End ini memverifikasi implementasi perbaikan terhadap konsistensi kepemilikan reservasi (*Booking Ownership*) dan affordance aksi tamu (*Allowed Actions*) pada hotel bintang 4 *Pulang ke Uttara* (Yogyakarta) sesuai standar OWASP API Top 10 (Broken Object Level Authorization), UU PDP No. 27/2022, dan aturan pembatalan hotel:

1. **Canonical Email Normalization on Checkout**: Pada `booking.Service.Create`, data masukan `GuestName` dan `GuestEmail` kini secara ketat dinormalisasi menggunakan `strings.TrimSpace` dan `strings.ToLower`. Di database PostgreSQL (`bookings.guest_email`), seluruh data reservasi baru tersimpan secara kanonikal dalam huruf kecil tanpa whitespace.
2. **Case-Insensitive Ownership Lookup**: Seluruh kueri pembacaan data tamu pada PostgreSQL (`internal/guest/postgres.go`):
   - `CountActiveBookingsByEmail`
   - `ListBookingsByEmail`
   - `GetBookingDetailByEmail`
   - `GetBookingReceiptData`  
   kini menggunakan predikat `WHERE LOWER(TRIM(b.guest_email)) = $1` sehingga tamu yang mendaftar atau login dengan variasi huruf besar/kecil (misal `Guest.Owner@EXAMPLE.com` vs `guest.owner@example.com`) tetap dapat mengakses seluruh reservasi miliknya tanpa hambatan.
3. **Policy-Driven Allowed Actions (`can_cancel`)**:
   - Untuk reservasi `confirmed` bertarif `non_refundable` (misal promo `OCTOBREAK`): `allowed_actions.can_cancel` bernilai `false`.
   - Untuk reservasi `confirmed` bertarif `flexible_48h`: dievaluasi secara dinamis terhadap batas waktu H-2 pukul 14:00 WIB (`booking.FreeCancellationDeadline`). Jika waktu saat ini sebelum batas waktu, `can_cancel` bernilai `true`; jika melewati batas waktu, `can_cancel` bernilai `false`.
4. **Hold Timeout Allowed Actions (`can_pay` & `can_cancel`)**:
   - Untuk reservasi `pending` yang masa berlakunya telah habis (`expires_at < now`): `allowed_actions.can_pay` dan `can_cancel` bernilai `false` (mencegah false affordance bayar atau batal pada hold kedaluwarsa). Tamu tetap diizinkan meminta bantuan staf hotel (`can_request_assistance = true`).
5. **Anti-IDOR Protection**:
   - Tamu yang mencoba mengakses ID reservasi milik tamu lain (`bk-other-user`) secara konsisten menerima respons HTTP 404 Not Found (`BOOKING_NOT_FOUND`), mencegah enumeration dan data leakage.

---

## 2. Rincian Eksekusi Test Cases

| No | Skenario Uji | Parameter / Payload | Expected Result | Actual Result | Status |
|---|---|---|---|---|---|
| 1 | Service Healthcheck | `GET /healthz` | HTTP 200 `{"status":"ok"}` | HTTP 200 `{"status":"ok"}` | **PASS** |
| 2 | Quote Generation | `POST /api/v1/quotes` | Quote ID diterbitkan | Quote ID valid | **PASS** |
| 3 | Checkout with Mixed-Case Email | `POST /api/v1/bookings` with `  Guest.Owner@EXAMPLE.COM  ` | HTTP 201 Created | HTTP 201 Created | **PASS** |
| 4 | DB Stored Email Verification | `SELECT guest_email FROM bookings` | Canonical lowercase `guest.owner...@example.com` | Tersimpan lowercase | **PASS** |
| 5 | Legacy OTP Login | Challenge & Verify OTP with lowercase email | Session token diterbitkan | Token valid | **PASS** |
| 6 | Active Bookings Count | `GET /api/v1/auth/guest/me` | `active_bookings_count: 1` | `active_bookings_count: 1` | **PASS** |
| 7 | List Bookings Case-Insensitivity | `GET /api/v1/guest/bookings` | Booking legacy ditemukan | Booking ID cocok | **PASS** |
| 8 | Booking Detail Case-Insensitivity | `GET /api/v1/guest/bookings/:id` | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 9 | Confirmed Non-Refundable `can_cancel` | Booking detail with `non_refundable` | `can_cancel: false` | `can_cancel: false` | **PASS** |
| 10 | Confirmed Non-Refundable `can_download_receipt` | Booking detail with `non_refundable` | `can_download_receipt: true` | `can_download_receipt: true` | **PASS** |
| 11 | Confirmed Non-Refundable `can_pay` | Booking detail with `non_refundable` | `can_pay: false` | `can_pay: false` | **PASS** |
| 12 | Confirmed Non-Refundable `can_request_assistance` | Booking detail with `non_refundable` | `can_request_assistance: true` | `can_request_assistance: true` | **PASS** |
| 13 | Confirmed Flexible Sebelum Cutoff `can_cancel` | Booking check-in H+7 (`flexible_48h`) | `can_cancel: true` | `can_cancel: true` | **PASS** |
| 14 | Confirmed Flexible Sebelum Cutoff `can_download_receipt` | Booking check-in H+7 (`flexible_48h`) | `can_download_receipt: true` | `can_download_receipt: true` | **PASS** |
| 15 | Confirmed Flexible Melewati Cutoff `can_cancel` | Booking check-in H+1 (melewati batas H-2 14:00 WIB) | `can_cancel: false` | `can_cancel: false` | **PASS** |
| 16 | Expired Hold `can_pay` | Pending booking with `expires_at` lampau | `can_pay: false` | `can_pay: false` | **PASS** |
| 17 | Expired Hold `can_cancel` | Pending booking with `expires_at` lampau | `can_cancel: false` | `can_cancel: false` | **PASS** |
| 18 | Expired Hold `can_request_assistance` | Pending booking with `expires_at` lampau | `can_request_assistance: true` | `can_request_assistance: true` | **PASS** |
| 19 | IDOR Defense Verification | Akses booking ID milik akun lain | HTTP 404 Not Found | HTTP 404 Not Found | **PASS** |

---

## 3. Kesimpulan Verifikasi

Temuan **BE-R05 (P1)** telah terselesaikan dan terverifikasi secara tuntas:
- **Unit & Table Tests**: Seluruh table tests di `internal/guest/service_test.go` dan `internal/booking/service_test.go` lulus 100% dengan total statement coverage **90.1%** pada package `internal/guest`.
- **Zero Lint / Vet Issues**: Lulus `go vet ./...` tanpa peringatan atau kesalahan syntax.
- **E2E Automation**: Skrip E2E (`testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh`) membuktikan bahwa email kanonikal tersimpan dengan andal di DB, reservasi lama dengan variasi casing tetap terdeteksi oleh tamu, dan affordance `allowed_actions` mencerminkan kebijakan tarif serta batas waktu nyata tanpa false affordance.
