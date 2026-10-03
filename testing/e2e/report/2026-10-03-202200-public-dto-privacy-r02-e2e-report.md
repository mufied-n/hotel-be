# E2E Test Report: Public DTO Privacy & Token Protection (BE-R02)

**Fitur:** Public DTO Privacy, Sensitive Free-Text & Token Protection  
**ID Gap:** BE-R02 (P1)  
**Terkait:** BE-G13, BE-R01, F02/F03  
**Tanggal Pengujian:** 3 Oktober 2026, 20:21:20 WIB  
**Hasil:** **34/34 PASS (100% SUKSES)**  
**Environment:** Linux / PostgreSQL 18 Alpine / Valkey 8 / Monolith HTTP Port 28080  

---

## 1. Ringkasan Eksekutif

Pengujian End-to-End ini memverifikasi kepatuhan penuh terhadap **UU Perlindungan Data Pribadi (UU PDP No. 27/2022)** dan isolasi kredensial (*least-privilege token protection*):
1. **Public DTO Minimization:** Struct `PublicDTO` tidak memiliki `special_requests` dan `estimated_arrival_time`. Permintaan GET publik/unauthenticated tidak memaparkan catatan kesehatan/preferensi tamu, waktu kedatangan, kontak, maupun token.
2. **Negative Ownership:** Caller dengan token palsu atau token milik booking lain tidak dapat membaca field privat (otomatis fallback ke `PublicDTO`).
3. **Owner Access:** Tamu pemilik dengan `X-Guest-Token` yang valid dapat membaca seluruh detail pemesanannya, namun `guest_token` tidak diulang dalam payload response GET.
4. **Staff Session Access:** Staf terautentikasi (Front Desk / Receptionist) dapat membaca catatan khusus dan estimasi kedatangan untuk keperluan operasional hotel, namun `guest_token` rahasia tamu di-redact (`guest_token: ""`).
5. **Staff Role Forgery Rejection:** Pengirim header role palsu tanpa sesi diverifikasi server (BE-R01) ditolak dari akses data privat dan hanya menerima `PublicDTO`.

---

## 2. Rincian Eksekusi Test Cases

| No | Kategori Uji | Skenario | Expected | Actual | Status |
|---|---|---|---|---|---|
| 1 | Inisiasi | Kalkulasi quote kamar standard | HTTP 200 | HTTP 200 | **PASS** |
| 2 | Inisiasi | Inisiasi booking hold dengan catatan khusus sensitif & arrival 15:00 | HTTP 201 | HTTP 201 | **PASS** |
| 3 | Public Read | Unauthenticated caller GET booking | HTTP 200 | HTTP 200 | **PASS** |
| 4 | Public Read | Public response memuat ID dan Room Type | Cocok | Cocok | **PASS** |
| 5 | Public Read | Public response TIDAK memuat `special_requests` | Absent | Absent | **PASS** |
| 6 | Public Read | Public response TIDAK memuat marker sensitif | Absent | Absent | **PASS** |
| 7 | Public Read | Public response TIDAK memuat `estimated_arrival_time` | Absent | Absent | **PASS** |
| 8 | Public Read | Public response TIDAK memuat `guest_name` | Absent | Absent | **PASS** |
| 9 | Public Read | Public response TIDAK memuat `guest_email` | Absent | Absent | **PASS** |
| 10 | Public Read | Public response TIDAK memuat `guest_phone` | Absent | Absent | **PASS** |
| 11 | Public Read | Public response TIDAK memuat `guest_token` | Absent | Absent | **PASS** |
| 12 | Public Read | Public response TIDAK memuat rincian harga (`total_price_minor`) | Absent | Absent | **PASS** |
| 13 | Negative Token | GET dengan token acak/palsu (`gst_fake_token_attacker_123`) | HTTP 200 | HTTP 200 | **PASS** |
| 14 | Negative Token | Token palsu tidak membaca `special_requests` | Absent | Absent | **PASS** |
| 15 | Negative Token | Token palsu tidak membaca marker sensitif | Absent | Absent | **PASS** |
| 16 | Negative Token | Token palsu tidak membaca `estimated_arrival_time` | Absent | Absent | **PASS** |
| 17 | Negative Token | Token palsu tidak membaca `guest_token` | Absent | Absent | **PASS** |
| 18 | Negative Other | GET dengan token milik booking lain (`gst_other_booking_token_999`) | HTTP 200 | HTTP 200 | **PASS** |
| 19 | Negative Other | Token lain tidak membaca `special_requests` | Absent | Absent | **PASS** |
| 20 | Negative Other | Token lain tidak membaca marker sensitif | Absent | Absent | **PASS** |
| 21 | Owner Access | GET dengan token valid milik pemesan (`X-Guest-Token`) | HTTP 200 | HTTP 200 | **PASS** |
| 22 | Owner Access | Pemilik membaca `special_requests` lengkap | Cocok | Cocok | **PASS** |
| 23 | Owner Access | Pemilik membaca `estimated_arrival_time` ("15:00") | Cocok | Cocok | **PASS** |
| 24 | Owner Access | Pemilik membaca nama dan email pemesan | Cocok | Cocok | **PASS** |
| 25 | Owner Access | Response GET pemilik TIDAK membocorkan ulang `guest_token` | Absent | Absent | **PASS** |
| 26 | Role Forgery | Caller mengirim `Authorization: Bearer receptionist` tanpa sesi | HTTP 200 | HTTP 200 | **PASS** |
| 27 | Role Forgery | Pemalsu role tidak membaca `special_requests` (PublicDTO) | Absent | Absent | **PASS** |
| 28 | Role Forgery | Pemalsu role tidak membaca `estimated_arrival_time` | Absent | Absent | **PASS** |
| 29 | Staff Session | Staf terverifikasi login via `/api/v1/auth/staff/login` | HTTP 200 | HTTP 200 | **PASS** |
| 30 | Staff Session | Staf dengan token `stf_...` membaca booking | HTTP 200 | HTTP 200 | **PASS** |
| 31 | Staff Session | Staf dapat membaca `special_requests` untuk layanan tamu | Cocok | Cocok | **PASS** |
| 32 | Staff Session | Staf dapat membaca `estimated_arrival_time` untuk operasional | Cocok | Cocok | **PASS** |
| 33 | Staff Session | Staf TIDAK BOLEH menerima `guest_token` rahasia tamu | Absent | Absent | **PASS** |
| 34 | Summary | Verifikasi seluruh assertions | 34 Passed | 34 Passed | **PASS** |

---

## 3. Kesimpulan Verifikasi

Temuan **BE-R02 (P1)** telah tertutup secara tuntas dan terverifikasi di seluruh lapisan unit test, router test, dan E2E test.
- Skema `PublicDTO` bersih dari free-text dan waktu kedatangan.
- Token rahasia tamu terlindungi dari eksposur ke staf maupun pengulangan pada endpoint GET.
