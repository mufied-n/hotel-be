# Product Requirements Document (PRD) — Booking Ownership & Allowed Actions Consistency (BE-R05)

**Feature:** Canonical Email Normalization & Policy-Driven Allowed Actions  
**Gap ID:** BE-R05 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/booking`, `internal/guest`  

---

## 1. Business Context & Objective

Portal "Booking Saya" (*My Bookings*) pada sistem *Pulang ke Uttara* memungkinkan tamu yang telah memverifikasi identitasnya via OTP untuk mengakses riwayat reservasi, melunasi pemesanan, atau membatalkan reservasi.
Sebelum perbaikan ini, temuan audit **BE-R05** mengidentifikasi dua celah integritas penting:
1. **Inkonsistensi Normalisasi Email (Hilangnya Booking Milik Tamu)**:
   - Pada pembuatan pesanan (`booking.Service`), email disimpan apa adanya sesuai input tamu (misal `Guest@Example.com` atau dengan spasi).
   - Pada autentikasi OTP (`guest.Service`), email dinormalisasi menjadi lowercase (`guest@example.com`).
   - Query pencarian pemesanan memakai kesamaan teks persis case-sensitive (`b.guest_email = $1`). Akibatnya, tamu yang mengetikkan huruf kapital saat reservasi tidak dapat menemukan pesanannya saat login ke portal tamu.
2. **False Affordance pada `allowed_actions`**:
   - Fungsi `computeAllowedActions` hanya memeriksa status teks `pending` atau `confirmed`.
   - Untuk pesanan `confirmed`, sistem selalu mengembalikan `can_cancel: true`, padahal pemesanan dengan tarif non-refundable (`OCTOBREAK` / `non_refundable`) atau pemesanan yang telah melewati batas waktu 48 jam sebelum 14:00 WIB pasti akan ditolak dengan HTTP 409 saat tombol cancel ditekan.
   - Untuk pesanan `pending` yang masa hold-nya telah kedaluwarsa (`expires_at < now`), sistem tetap mengembalikan `can_pay: true` dan `can_cancel: true`.

Tujuan dari perbaikan ini adalah memastikan **Canonical Email Equality** di seluruh lapisan (checkout, autentikasi, dan lookup) serta **Action Truthfulness** pada `allowed_actions` yang diturunkan langsung dari kebijakan tarif dan batas waktu resmi hotel.

---

## 2. User Personas & Permissions

| Persona | Kebutuhan & Ekspektasi |
|---|---|
| **Tamu Hotel (`guest`)** | Menemukan seluruh reservasi miliknya tanpa terpengaruh perbedaan huruf besar/kecil email, dan hanya melihat tombol aksi yang valid dan dapat dieksekusi. |
| **Frontend Engineer / Mobile Dev** | Mengandalkan payload `allowed_actions` untuk mengaktifkan/menonaktifkan tombol (Pay, Cancel, Download Receipt) tanpa tebak-tebakan status. |
| **Hotel Front Desk & Revenue Manager** | Mencegah kebingungan tamu yang komplain karena melihat tombol pembatalan aktif pada reservasi promo non-refundable. |

---

## 3. Acceptance Criteria

- **AC-01 (Canonical Email at Booking Creation)**: `CreateBooking` dan `CreateBookingQuoted` WAJIB menormalisasi `GuestEmail` menjadi lowercase dan trimmed (`strings.ToLower(strings.TrimSpace(email))`).
- **AC-02 (Case-Insensitive Ownership Lookup)**: Seluruh query lookup pada `guest.PostgresStore` (`ListBookingsByEmail`, `GetBookingDetailByEmail`, `CountActiveBookingsByEmail`, `GetBookingReceiptData`) WAJIB menggunakan evaluasi `LOWER(TRIM(b.guest_email)) = $1` dengan argumen yang ternormalisasi.
- **AC-03 (Policy-Driven `can_cancel` for Confirmed Bookings)**:
  - Jika reservasi berstatus `confirmed` dan memiliki kebijakan `non_refundable`, `can_cancel` WAJIB bernilai `false`.
  - Jika reservasi berstatus `confirmed` dan memiliki kebijakan `flexible_48h`, sistem WAJIB menghitung deadline via `booking.FreeCancellationDeadline(checkIn, policy)`. Jika `now > deadline`, `can_cancel` WAJIB `false`; jika `now <= deadline`, `can_cancel` WAJIB `true`.
- **AC-04 (Expiration-Aware `can_pay` for Pending Bookings)**:
  - Jika reservasi berstatus `pending` dan `ExpiresAt` telah terlewati (`now > ExpiresAt`), maka `can_pay` dan `can_cancel` WAJIB bernilai `false`.
- **AC-05 (Zero Regressions on IDOR & Privacy)**: Tamu tidak boleh dapat melihat pesanan tamu lain (tetap HTTP 404 pada detail dan kuitansi tak berhak).

---

## 4. Non-Functional Requirements

- **Timezone Awareness**: Evaluasi deadline pembatalan tetap terikat pada zona waktu resmi hotel Yogyakarta (`LocationWIB` / UTC+7).
- **Test Coverage**: Minimal 80% coverage pada modul yang terdampak.
