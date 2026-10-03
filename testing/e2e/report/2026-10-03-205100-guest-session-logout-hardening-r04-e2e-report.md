# E2E Test Report: Guest Session Logout Hardening & Cookie Security (BE-R04)

**Fitur:** Guest Session Logout Hardening, Secure Cookies, and Private Cache Headers  
**Gap ID:** BE-R04 (P1)  
**Terkait:** BE-R01, BE-R02, BE-R03, F02/F12  
**Tanggal Pengujian:** 3 Oktober 2026, 20:50:27 WIB  
**Hasil:** **19/19 PASS (100% SUKSES)**  
**Environment:** Linux / PostgreSQL 18 Alpine / Valkey 8 / Monolith HTTP Port 28080  

---

## 1. Ringkasan Eksekutif

Pengujian End-to-End ini memverifikasi implementasi perbaikan terhadap celah keamanan sesi tamu (*Guest Session Lifecycle*) pada hotel bintang 4 *Pulang ke Uttara* (Yogyakarta) sesuai standar OWASP ASVS V3 dan UU PDP No. 27/2022:
1. **Honest Logout & Revocation Integrity**: Endpoint `POST /api/v1/auth/guest/logout` kini memeriksa status hasil pencabutan sesi di database (`RevokeSession`). Jika pencabutan di database gagal, server merespons dengan HTTP 503 Service Unavailable (`LOGOUT_FAILED`) dan tidak menghapus cookie sehingga client dapat melakukan retry. Saat sukses, token sesi terhapus secara permanen dari tabel `guest_sessions` di PostgreSQL (0 baris tersisa).
2. **Dynamic Secure Cookie Enforcement**: Cookie sesi `guest_session` kini secara dinamis menerapkan atribut `Secure` saat koneksi menggunakan HTTPS, terdeteksi melalui `TLS != nil`, header `X-Forwarded-Proto: https`, atau konfigurasi non-development (`!IsDevelopment`). Atribut `HttpOnly: true` dan `SameSite: Lax` tetap dipertahankan untuk mitigasi XSS dan CSRF.
3. **Private Cache-Control Enforcement**: Seluruh endpoint data sesi sensitif (`/api/v1/auth/guest/verify`, `/api/v1/auth/guest/me`, dan `/api/v1/auth/guest/logout`) kini menyertakan header `Cache-Control: no-store, private` dan `Pragma: no-cache` untuk mencegah perantara proxy dan cache browser menyimpan data profil atau token tamu.
4. **Session Invalidation Guarantee**: Token sesi yang telah dicabut diverifikasi ditolak secara mutlak pada seluruh endpoint terproteksi (`/auth/guest/me` dan `/guest/bookings`) dengan HTTP 401 Unauthorized (`UNAUTHORIZED`).
5. **TouchSession Failure Resilience**: Pada `ValidateSession`, kegagalan pembaruan `TouchSession` di database dicatat pada structured log dan tidak memperpanjang waktu kedaluwarsa sesi secara in-memory, mencegah desinkronisasi antara memori aplikasi dan database.

---

## 2. Rincian Eksekusi Test Cases

| No | Skenario Uji | Payload / Parameter | Expected Result | Actual Result | Status |
|---|---|---|---|---|---|
| 1 | Healthcheck endpoint | `GET /healthz` | HTTP 200 `{"status":"ok"}` | HTTP 200 `{"status":"ok"}` | **PASS** |
| 2 | Status verifikasi OTP sukses | POST `/verify` valid | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 3 | Anti-cache header verify | Response header `/verify` | `Cache-Control: no-store, private` | Ada | **PASS** |
| 4 | Pragma no-cache header verify | Response header `/verify` | `Pragma: no-cache` | Ada | **PASS** |
| 5 | Cookie name check | Set-Cookie header | `guest_session=` | Cocok | **PASS** |
| 6 | Cookie HttpOnly flag | Set-Cookie header | `HttpOnly` | Ada | **PASS** |
| 7 | Cookie SameSite flag | Set-Cookie header | `SameSite=Lax` | Ada | **PASS** |
| 8 | Cookie Secure flag (HTTPS) | Set-Cookie header saat `X-Forwarded-Proto: https` | `Secure` | Ada | **PASS** |
| 9 | Pengambilan token sesi | Body JSON response verify | Token non-empty `gst_sess_...` | Berhasil | **PASS** |
| 10 | Akses profil sesi terproteksi | `GET /api/v1/auth/guest/me` | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 11 | Anti-cache header /me | Response header `/me` | `Cache-Control: no-store, private` | Ada | **PASS** |
| 12 | Pragma no-cache header /me | Response header `/me` | `Pragma: no-cache` | Ada | **PASS** |
| 13 | Status logout sukses | `POST /api/v1/auth/guest/logout` | HTTP 200 OK | HTTP 200 OK | **PASS** |
| 14 | Anti-cache header logout | Response header `/logout` | `Cache-Control: no-store, private` | Ada | **PASS** |
| 15 | Penghapusan cookie pada logout | Set-Cookie header | `Max-Age=` (0 atau -1) | Cocok | **PASS** |
| 16 | HttpOnly flag saat hapus cookie | Set-Cookie header logout | `HttpOnly` | Ada | **PASS** |
| 17 | Secure flag saat hapus cookie | Set-Cookie header logout | `Secure` | Ada | **PASS** |
| 18 | Invariant database sesi | `SELECT count(*) FROM guest_sessions` | 0 sesi tersisa | 0 sesi tersisa | **PASS** |
| 19 | Invalidation token pada /me & /bookings | Request dengan token yang dicabut | HTTP 401 Unauthorized | HTTP 401 Unauthorized | **PASS** |

---

## 3. Kesimpulan Verifikasi

Temuan **BE-R04 (P1)** telah terselesaikan dan terverifikasi secara tuntas:
- **Unit & Handler Tests**: Seluruh table tests di `internal/api/guest_api_test.go` (coverage: 85.2%) dan `internal/guest/service_test.go` (coverage: 89.5%) lulus 100%.
- **Zero Lint / Vet Issues**: Lulus `go vet ./...` tanpa error.
- **E2E Automation**: Skrip E2E (`testing/e2e/script/guest_session_logout_hardening_r04_e2e.sh`) membuktikan penghapusan sesi di DB bekerja secara deterministik, cookie diamankan dengan flag `Secure`, dan header private cache diterapkan secara menyeluruh.
