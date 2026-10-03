# Product Requirements Document (PRD) — Guest Session Logout Hardening & Cookie Security (BE-R04)

**Feature:** Guest Session Revocation Integrity, Secure Cookies, and Private Cache Headers  
**Gap ID:** BE-R04 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/api` (`guest_auth.go`), `internal/guest` (`service.go`)  

---

## 1. Business Context & Objective

Pada hotel bintang 4 *Pulang ke Uttara* (Yogyakarta), portal tamu (*Guest Portal*) memungkinkan tamu memeriksa reservasi, status pembayaran, dan faktur resmi. 
Sebelum perbaikan ini, temuan audit **BE-R04** mengungkapkan beberapa kelemahan keamanan sesi:
1. **Palsu Sukses saat Logout (*False Positive Logout*)**: Endpoint `POST /api/v1/auth/guest/logout` mengabaikan error dari `RevokeSession()`, menghapus cookie lokal, dan mengembalikan HTTP 200 OK dengan pesan sukses. Jika pencabutan sesi di database gagal, token sesi tetap aktif di server dan dapat disalahgunakan oleh pihak ketiga melalui header `Authorization: Bearer`.
2. **Ketiadaan Atribut `Secure` pada Cookie**: Cookie `guest_session` dipasang tanpa flag `Secure`, memungkinkan pengiriman token dalam plaintext jika portal diakses melalui HTTP biasa atau reverse-proxy miskonfigurasi.
3. **Ketiadaan Cache-Control Anti-Snoop**: Endpoint autentikasi dan profil tamu (`/api/v1/auth/guest/me`, `/logout`, `/verify`) belum menetapkan header `Cache-Control: no-store, private`, berisiko ter-cache oleh shared proxy atau browser cache publik.
4. **TouchSession Divergence**: `ValidateSession` mengabaikan kegagalan database pada perpanjangan masa aktif sesi (`TouchSession`), menciptakan inkonsistensi antara state in-memory dan database.

Tujuan implementasi ini adalah menegakkan **Session Revocation Honesty**, **Cookie Security Hardening (OWASP ASVS V3)**, dan **Zero Information Leakage** pada siklus hidup sesi tamu.

---

## 2. User Personas & Permissions

| Persona | Hak Akses & Kebutuhan |
|---|---|
| **Tamu Hotel (`guest`)** | Mengakhiri sesi secara aman dari browser/perangkat publik dengan jaminan bahwa token telah hangus di database server. |
| **BFF / Frontend Web** | Menerima status logout jujur; jika logout di server gagal, menampilkan pesan retry dan mempertahankan state sesi agar dapat dicoba kembali. |
| **Security & Compliance Auditor** | Memastikan kepatuhan UU PDP No. 27/2022 dan OWASP ASVS V3 terkait manajemen token sesi dan proteksi cookie. |

---

## 3. Acceptance Criteria

- **AC-01 (Honest Logout)**: Jika `RevokeSession()` gagal (misal koneksi DB error), `handleGuestLogout` WAJIB mengembalikan HTTP 503 Service Unavailable dengan error code `LOGOUT_FAILED`, dan tidak menghapus cookie sehingga klien dapat melakukan retry.
- **AC-02 (Secure Cookie Enforcement)**: Cookie `guest_session` WAJIB memiliki flag `Secure: true` jika request dijalankan di atas TLS (`r.TLS != nil`), header `X-Forwarded-Proto: https`, atau konfigurasi bukan mode development (`!d.IsDevelopment`).
- **AC-03 (Strict Cookie Destruction)**: Saat logout sukses, cookie `guest_session` dihapus dengan `MaxAge: -1`, `Expires: Unix(0,0)`, `Path: "/"`, `HttpOnly: true`, dan `Secure` flag yang konsisten.
- **AC-04 (Private Cache Headers)**: Seluruh response pada `handleGuestVerify`, `handleGuestMe`, dan `handleGuestLogout` WAJIB menyertakan `Cache-Control: no-store, private` dan `Pragma: no-cache`.
- **AC-05 (TouchSession Propagation)**: Jika `TouchSession()` gagal saat `ValidateSession()`, error dicatat pada structured log dan ditangani secara konsisten sehingga sesi tidak mengalami desinkronisasi.
- **AC-06 (Backward Compatibility)**: Klien yang menggunakan bearer token `Authorization: Bearer gst_sess_...` tetap didukung penuh.

---

## 4. Non-Functional Requirements

- **Zero Overhead**: Tidak ada query tambahan di luar operasi ACID yang sudah ada.
- **Fail-Safe**: Mengikuti prinsip *fail-secure* — kegagalan penghapusan token tidak boleh disamarkan sebagai keberhasilan.
