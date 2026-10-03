# Product Requirements Document (PRD)
# Modernisasi Transport Layer: Gin Router, JSON v2, & Go Validator v10
**Properti:** Hotel Pulang ke Uttara (Yogyakarta)  
**Dokumen ID:** `PRD-TRANSPORT-MODERNIZATION-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / In Execution  

---

## 1. Latar Belakang & Konteks Bisnis

Sistem Hotel Booking Engine Pulang ke Uttara (95 kamar, Yogyakarta) saat ini menggunakan `go-chi/chi/v5` sebagai router HTTP, `encoding/json` (v1) untuk serialisasi payload, dan validasi imperatif secara manual pada transport layer.

Untuk meningkatkan efisiensi operasional sistem, kehandalan konkurensi, keamanan dari ancaman *JSON smuggling / parameter pollution*, serta kecepatan respon pemesanan kamar saat lonjakan trafik (*peak season / high occupancy*), diputuskan untuk memodernisasi transport layer dengan 3 pilar:
1. **Gin HTTP Web Framework (`github.com/gin-gonic/gin`)**: Menggantikan Chi dengan performa Radix Tree routing yang sangat cepat, grouping yang tangguh, serta keselarasan format routing parameter (`:id`) dengan model aturan Casbin RBAC database.
2. **Standard Go `encoding/json/v2`**: Mengadopsi library standar Go 1.27 terbaru untuk serialisasi/deserialisasi payload berkecepatan tinggi dengan penulisan langsung (`MarshalWrite` / `UnmarshalRead`), validasi UTF-8 yang ketat, dan penolakan *duplicate key* secara otomatis (RFC 8259).
3. **Go Playground Validator v10 (`github.com/go-playground/validator/v10`)**: Menyediakan validasi skema deklaratif pada request DTO, memisahkan secara tegas validasi bentuk/tipe payload (transport) dengan aturan state machine reservasi (domain).

---

## 2. Persona & Pengguna Sistem

| Persona | Peran & Akses | Kebutuhan Transport Layer |
|---|---|---|
| **Tamu Hotel (`guest`)** | Reservasi publik, pencarian multi-malam, OTP challenge & verifikasi sesi, portal reservasi saya. | Respon cepat (< 50ms), validasi input ramah dan akurat, keamanan data PII terjamin. |
| **Resepsionis (`receptionist`)** | Check-in, check-out, no-show, pemindahan kamar (*room move*), perpanjangan menginap (*extend stay*), daily roster. | Latensi minimal saat jam sibuk check-in (14:00 - 16:00), penanganan konkurensi bebas race condition. |
| **Housekeeping (`housekeeping`)** | Status kebersihan dan kesiapan kamar (*Clean*, *Inspected*, *Dirty*, *Out of Order*). | Validasi status transisi cepat pada perangkat mobile staf. |
| **Revenue Manager (`revenue_mgr`)** | Manajemen katalog tipe kamar, kuotasi tarif dinamis, promosi. | Validasi data kamar dan rentang tanggal yang ketat. |
| **Finance (`finance`)** | Rekonsiliasi transaksi Xendit, approval & refund otomatis gateway. | Ekstraksi parameter dan payload aman, integritas tinggi. |
| **General Manager (`gm_admin`)** | Kontrol menyeluruh, manajemen feature flag runtime. | Administrasi sistem tanpa downtime. |

---

## 3. Matriks Fitur & Persyaratan Bisnis

### 3.1 Kontrak RESTful API Tidak Berubah (Zero API Breaking Changes)
* Seluruh endpoint publik, guest portal, dan internal staff tetap mempertahankan method, path URL, query parameters, header autentikasi (`Authorization: Bearer`, `X-Guest-Token`, `X-Guest-Session`), dan HTTP status code yang sudah ada.
* Format respon error tetap mematuhi standar **RFC 7807 Problem Details** (`application/problem+json`) dengan machine-readable error `code` dan `detail`.

### 3.2 Keamanan & Integritas Payload (OWASP API Security)
* Menolak payload JSON yang memiliki kunci ganda (*duplicate keys*) untuk mencegah manipulasi parameter kuotasi kamar atau jumlah tamu.
* Menolak byte UTF-8 cacat pada nama tamu, nomor telepon, dan permintaan khusus (*special requests*).
* Proteksi fail-closed pada otorisasi Casbin RBAC tetap aktif (HTTP 503 jika enforcer uninitialized).

### 3.3 Validasi Skema Deklaratif
* Menggantikan validasi manual di transport layer dengan tag struct standar `validate:"..."` yang diperiksa secara terpusat oleh validator v10.
* Pemetaan error validasi tetap menghasilkan pesan yang mudah dipahami klien serta kode error yang kompatibel dengan test suite yang ada (misal `INVALID_ROOM_COUNT`, `INVALID_GUEST_COUNT`, `INVALID_DATE_FORMAT`).

---

## 4. Acceptance Criteria (Kriteria Keberterimaan)

1. **AC-01 (Router Parity)**: Seluruh 30+ endpoint yang sebelumnya di-mount di Chi dapat diakses dengan sukses melalui Gin engine dengan routing parameter `:id` / `:key`.
2. **AC-02 (JSON v2 Parity)**: Serialisasi dan deserialisasi JSON di seluruh handler [`internal/api`](file:///home/ahmadm/.gemini/antigravity/worktrees/current-booking/migrate_router_to_gin/internal/api) menggunakan `encoding/json/v2` tanpa regresi.
3. **AC-03 (Validator v10 Integration)**: DTO request (Search, Quotes, Bookings, Catalog, Finance, Housekeeping, FrontDesk, Stay) divalidasi menggunakan `validator/v10` dan mengembalikan respon RFC 7807 saat validasi gagal.
4. **AC-04 (Test Suite Coverage)**: Seluruh unit test dan handler test di [`internal/api`](file:///home/ahmadm/.gemini/antigravity/worktrees/current-booking/migrate_router_to_gin/internal/api) lulus 100% dengan statement test coverage $\ge 80\%$ (baseline saat ini 85.1%).
5. **AC-05 (Zero Lint/Vet Error)**: `go vet ./...` lulus tanpa peringatan atau error.
6. **AC-06 (E2E Regression Pass)**: Seluruh automasi E2E di `testing/e2e/script/` berjalan sukses 100%.

---

## 5. Non-Functional Requirements (NFR)

* **Performance**: Overhead routing Gin < 1ms, streaming `json.MarshalWrite` menghemat alokasi memori heap hingga 30%.
* **Reliability**: Graceful shutdown server tetap 10 detik, fail-fast pada error inisialisasi listen.
* **Maintainability**: Kode bersih, modular, dan mematuhi panduan Anti-Overengineering (Ponytail).
