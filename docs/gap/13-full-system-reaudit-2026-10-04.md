# Audit Ulang Komprehensif Sistem Backend — 4 Oktober 2026

**Tanggal:** 4 Oktober 2026 (Asia/Jakarta)  
**Status Audit:** FINAL / SOURCE VERIFIED  
**Cakupan:** Seluruh codebase backend Hotel Booking Engine (*Pulang ke Uttara*), mencakup refactoring arsitektur transport HTTP, penutupan register celah BE-R01–BE-R18, evaluasi roadmap F01–F15, audit anti-overengineering (Ponytail), analisis keamanan & sesi, serta verifikasi pengujian konkurensi (race detector) dan coverage.

---

## 1. Eksekutif Ringkasan (Executive Summary)

Audit ulang ini dilakukan untuk mengevaluasi kondisi menyeluruh repositori setelah pelaksanaan refactoring modular HTTP transport layer, penyelesaian perbaikan gap BE-R17 & BE-R18, pembersihan file orphan/tercecer, serta pengujian end-to-end terintegrasi.

### Hasil Kunci:
1. **Arsitektur Transport Bersih & Modular:** Monolit flat di `internal/api/` telah sepenuhnya ditata ulang menjadi arsitektur namespace protokol di [`internal/api/http/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http) dengan subpackage [`middleware/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware) dan [`handler/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler). Kompatibilitas facade (`api.go`) dan 9 file test orphan telah dihapus 100%. Caller repositori terhubung langsung ke package resmi.
2. **Penutupan Register Celah (18/18 BE-R RESOLVED):** Seluruh 18 temuan re-audit aktif (BE-R01 hingga BE-R18) telah ditutup dengan implementasi nyata pada kode sumber dan dilindungi oleh unit/E2E test. Tidak ada temuan P0 atau P1 yang berstatus OPEN.
3. **Zero Lint / Vet Issues:** Eksekusi `go vet ./...` lulus bersih (exit code 0).
4. **Verifikasi Bebas Race Condition:** Eksekusi `go test -race ./...` lulus 100% pada konkurensi default tanpa tabrakan race condition lintas paket.
5. **Coverage Transport $\ge 80\%$:**
   - `internal/api/http`: **96.0%**
   - `internal/api/http/middleware`: **84.8%**
   - `internal/api/http/handler`: **80.3%**
6. **Integritas Rute Terkunci:** Seluruh 53 rute HTTP API diverifikasi identik terhadap snapshot [`testdata/routes.golden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/testdata/routes.golden).
7. **E2E Automation:** 21/21 assertions lulus pada [`testing/e2e/script/router_refactor_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/router_refactor_e2e.sh).

---

## 2. Matriks Status Register BE-R01 s/d BE-R18

Seluruh 18 temuan celah keamanan, keandalan, dan konsistensi bisnis telah diperbaiki dan diverifikasi pada source code:

| ID | Prio | Deskripsi Temuan Awal | Status | Bukti Implementasi & Pengujian |
|---|:---:|---|:---:|---|
| **BE-R01** | P0 | Identitas staf dapat dipalsukan via role-string Bearer / X-User-Role | **CLOSED** | Header `X-User-Role` dihapus total. Sesi staf wajib berawalan `stf_` dan diverifikasi oleh [`staffauth.Service`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/staffauth/service.go) terhadap database PostgreSQL dengan hashing SHA-256. |
| **BE-R02** | P1 | DTO publik membocorkan catatan khusus / PII tamu | **CLOSED** | Sanitasi DTO pada [`handler/booking.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go). Special requests diproteksi di balik sesi tamu [`handler/assistance.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/assistance.go). |
| **BE-R03** | P1 | Challenge OTP & attempt counter tidak atomik | **CLOSED** | Transaksi SQL atomik dengan penegakan lockout 5x kegagalan pada [`internal/guest/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/postgres.go). |
| **BE-R04** | P1 | Logout mengembalikan status 200 meski pencabutan sesi gagal | **CLOSED** | Validasi error pencabutan sesi secara jujur (*honest revocation*) sebelum mengembalikan respon OK di [`handler/guest_auth.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_auth.go). |
| **BE-R05** | P1 | Ownership email & allowed_actions tidak konsisten | **CLOSED** | Normalisasi email canonical (lowercase/trim) dan perhitungan dinamis `allowed_actions` berbasis status booking & kebijakan pembatalan di [`internal/guest/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/guest/). |
| **BE-R06** | P1 | Create booking tanpa quote valid lolos dari consent | **CLOSED** | Validasi wajib `quote_id` yang masih berlaku serta consent `terms_accepted` & `privacy_accepted` di [`handler/booking.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go). |
| **BE-R07** | P1 | Kapasitas kamar tidak ditegakkan seragam | **CLOSED** | Validasi invariant kapasitas (`Adults`, `Children`, `Rooms`) ditegakkan identik di search, quote, dan checkout ([`internal/booking/search.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/search.go)). |
| **BE-R08** | P1 | Idempotency checkout rawan race condition | **CLOSED** | In-progress lock dengan status `IDEMPOTENCY_IN_PROGRESS` (header `Retry-After: 2`), caching payload replay, dan isolasi SHA-256 di [`middleware/idempotency.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/idempotency.go). |
| **BE-R09** | P1 | Tarif katalog CRUD tidak sinkron ke rate engine | **CLOSED** | Rate engine membaca langsung basis harga dinamis dari catalog store via `SetBaseRateSource` di [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go#L86). |
| **BE-R10** | P1 | Multi-room breakfast & child tier ambigu | **CLOSED** | Perhitungan sarapan dihitung proporsional per kamar dan batas umur anak divalidasi ketat di [`handler/guest_parser.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_parser.go). |
| **BE-R11** | P1 | Quote hanya disimpan di memori satu instance | **CLOSED** | Implementasi [`rates.ValkeyQuoteStore`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates/valkey_store.go) terdistribusi dengan TTL 15 menit dan mekanisme fail-closed. |
| **BE-R12** | P1 | Deadline pembatalan menggunakan UTC padahal aturan hotel WIB | **CLOSED** | Zona waktu hotel dipatok tegas ke `Asia/Jakarta` (WIB, UTC+7) pada kalkulasi cancellation cutoff di [`internal/booking/`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/). |
| **BE-R13** | P1 | Gateway timeout dianggap kegagalan permanen | **CLOSED** | Klasifikasi error gateway timeout (`PAYMENT_GATEWAY_TIMEOUT`, 504) dibedakan dari kegagalan transaksi definitif; integritas ledger tetap terlindungi. |
| **BE-R14** | P1 | Webhook tidak mencocokkan amount & invoice dengan ledger | **CLOSED** | Verifikasi invoice ID, payment method, dan nominal pembayaran terhadap ledger `payment_attempts` pada [`internal/booking/payment_event.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/payment_event.go). |
| **BE-R15** | P1 | Link pembayaran tidak dapat dipulihkan lintas sesi | **CLOSED** | Endpoint `/api/v1/bookings/:id/payment` dan `/api/v1/guest/bookings/:id/payment` memulihkan invoice aktif tanpa charge ganda. |
| **BE-R16** | P0 | Deployment production dapat keliru memakai fake gateway / log notifier | **CLOSED** | Pengecekan fail-fast saat startup di [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go#L95-L118); proses keluar (`os.Exit(1)`) jika berjalan di `ENV=production` dengan mock/fake gateway. |
| **BE-R17** | P1 | Pengiriman OTP tidak durable via outbox | **CLOSED** | Implementasi transactional outbox event `guest.otp_dispatch` dengan background relay dan anti-enumeration timing resistent di commit `492073b`. |
| **BE-R18** | P2 | Boundary payload dan format error API belum seragam | **CLOSED** | Middleware pembatas payload 1MB, validasi idempotency key $\le 64$ karakter, dan kontrak error RFC 7807 unified dual-shape di commit `1c0a0b2`. |

---

## 3. Audit Arsitektur & Modularisasi Transport (HTTP Layer)

### 3.1 Struktur Direktori Final
Struktur direktori `internal/api/` saat ini bersih dan tidak memiliki file dangling atau tercecer:

```text
internal/api/
└── http/
    ├── deps.go                 # Injeksi dependensi (type Deps = handler.Deps)
    ├── idempotency.go          # Implementasi store idempotency Postgres & Memory
    ├── idempotency_test.go     # Suite pengujian konkurensi store idempotency
    ├── mock_test.go            # Mock terisolasi untuk test suite router
    ├── rbac_test.go            # Suite pengujian otorisasi Casbin per-role
    ├── route_guard_test.go     # Guard penjamin konsistensi rute & migrasi DB
    ├── router.go               # Inisialisasi gin.Engine & middleware pipeline
    ├── router_test.go          # Pengujian komprehensif 53 rute terhadap golden file
    ├── routes.go               # Pendaftaran deklaratif endpoint API & RBAC
    ├── testdata/
    │   └── routes.golden       # 53 rute resmi terbekukan (frozen)
    ├── handler/                # 20 file handler domain + table-driven tests (cov: 80.3%)
    │   ├── assistance.go & assistance_test
    │   ├── booking.go & booking_test.go
    │   ├── catalog.go & catalog_test.go
    │   ├── deps.go & deps_test.go
    │   ├── errors.go
    │   ├── featureflag.go
    │   ├── finance.go
    │   ├── frontdesk.go
    │   ├── guest_auth.go
    │   ├── guest_parser.go & guest_parser_test.go
    │   ├── guest_portal.go & guest_portal_test.go
    │   ├── health.go & health_test.go
    │   ├── helpers.go
    │   ├── housekeeping.go
    │   ├── quote.go
    │   ├── search.go & search_and_quote_test.go
    │   ├── staff_auth.go & staff_and_ops_test.go
    │   ├── stay.go
    │   ├── validator.go & validator_test.go
    │   └── webhook.go
    └── middleware/             # Middleware pipeline terisolasi (cov: 84.8%)
        ├── auth.go & auth_test.go
        ├── core.go & core_test.go
        ├── featureflag.go & featureflag_test.go
        ├── guest_session.go & guest_session_test.go
        ├── idempotency.go & idempotency_test.go
        └── ratelimit.go
```

### 3.2 Eliminasi Compatibility Facade
Sebelumnya terdapat file `internal/api/api.go` yang bertindak sebagai jembatan sementara. Sesuai prinsip **Ponytail (Anti-Overengineering)**, facade tersebut telah dihapus secara tuntas. Tiga titik panggil repositori telah dimigrasi untuk langsung mengimpor package resmi:
1. [`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go):
   ```go
   import apihttp "github.com/example/hotel-booking/internal/api/http"
   ```
2. [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go):
   ```go
   import apihttp "github.com/example/hotel-booking/internal/api/http"
   ```
3. [`testing/integration/idempotency_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/integration/idempotency_test.go):
   ```go
   import "github.com/example/hotel-booking/internal/api/http"
   ```

### 3.3 Pembersihan File Tercecer
Sembilan file pengujian orphan di root/http (`*_api_test.go`, `webhook_test.go`, `assistance_api_test.go`) telah dihapus. Seluruh skenario pengujian telah direlokasi ke dalam unit test table-driven pada package handler dan middleware yang relevan.

---

## 4. Evaluasi Anti-Overengineering (Ponytail Analysis)

Pemeriksaan menyeluruh terhadap pola desain repositori menunjukkan:
* **Zero Speculative Abstraction (YAGNI):** Tidak ada interface kosong, factory bertingkat yang tidak perlu, atau arsitektur microservices prematur.
* **Protokol Transport Terisolasi:** HTTP transport diletakkan di `internal/api/http/`, sehingga jika suatu hari ditambahkan gRPC (`internal/api/grpc/`) atau Message Broker (`internal/api/broker/`), kode domain tetap murni dan tidak tercampur.
* **Type Aliasing Ramping:** File [`internal/api/http/deps.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/deps.go) hanya berisi 56 baris kode menggunakan type alias Go `type Deps = handler.Deps`, menghindari duplikasi struct dependensi yang redundan.
* **Standard Library First:** Menghindari dependensi eksternal yang berlebihan; encoding/json, crypto/sha256, crypto/rand, dan sync.Map dimanfaatkan secara optimal.
* **Kesimpulan Ponytail:** **Lean already. Ship.**

---

## 5. Audit Keamanan & Autentikasi

### 5.1 Staff Authentication & RBAC (BE-R01, BE-G14)
* **Kredensial Aman:** Login staf menggunakan bcrypt cost 12 ([`staffauth.Service`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/staffauth/service.go)).
* **Proteksi Brute-Force:** Maksimum 5 kegagalan login berturut-turut memicu penguncian akun selama 15 menit.
* **Anti-Enumeration:** Hash tiruan bcrypt dieksekusi saat username tidak ditemukan untuk mencegah *timing attack*.
* **Format Token:** Token sesi staf memiliki prefiks wajib `stf_`, disimpan dalam bentuk hash SHA-256 di PostgreSQL dengan masa berlaku 8 jam.
* **Fail-Closed Enforcer:** Casbin RBAC menolak seluruh permintaan (`503 AUTH_UNAVAILABLE` atau `403 FORBIDDEN`) jika enforcer nil atau role tidak memiliki izin pada `c.FullPath()`. Header `X-User-Role` telah dieliminasi sepenuhnya dari sistem.

### 5.2 Guest Authentication & Sesi Tamu (BE-R03, BE-R04, BE-R05)
* **OTP Verification:** Kode 6-digit diverifikasi dalam transaksi SQL atomik. Percobaan salah dibatasi hingga 5 kali sebelum status dikunci (`LOCKED`).
* **Sesi Tamu:** Token berprefiks `gst_sess_` disimpan dalam hash SHA-256 dengan TTL 7 hari.
* **Honest Logout:** Respon logout memverifikasi penghapusan baris sesi di database sebelum mengirimkan konfirmasi.

### 5.3 Boundary Protection & Idempotency (BE-R08, BE-R18)
* **Ukuran Body:** Dibatasi maksimal 1MB via middleware [`LimitBodySize`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/core.go).
* **Idempotency Key:** Dibatasi maksimal 64 karakter; request concurrent dengan key yang sama menerima `409 IDEMPOTENCY_IN_PROGRESS` beserta header `Retry-After: 2`.
* **Isolasi Key:** Hash payload dan identitas subjek diisolasi (`sha256(subject + ":" + key)`), mencegah eksploitasi tabrakan key antar-pengguna yang berbeda.
* **Dual-Shape Error Contract:** Format error RFC 7807 mengembalikan shape seragam yang kompatibel untuk frontend legacy dan modern:
  ```json
  {
    "type": "https://httpstatuses.com/400",
    "title": "Bad Request",
    "status": 400,
    "detail": "cancellation window has elapsed",
    "error": "cancellation window has elapsed",
    "code": "CANCELLATION_WINDOW_ELAPSED",
    "request_id": "7e90001b00ef4c0b86c16e0c7df7c7f4"
  }
  ```

---

## 6. Matriks Cakupan Roadmap Fitur (F01 s/d F15)

Status implementasi fungsional berdasarkan source code per 4 Oktober 2026:

| Fitur | Cakupan Fungsional di Source | Status Kontrak |
|---|---|:---:|
| **F01 (Booking Journey)** | Pencarian ketersediaan, quote locking 15 menit, checkout ber-consent, kalkulasi harga rupiah, reservasi kamar atomik. | **COMPLETE** |
| **F02 (Guest Access)** | OTP passwordless via email, sesi bertoken `gst_sess_`, info profil `me`, logout jujur. | **COMPLETE** |
| **F03 (My Bookings)** | Daftar riwayat & detail booking berdasarkan kepemilikan email canonical, aksi dinamis `allowed_actions`. | **COMPLETE** |
| **F04 (Artifacts)** | Receipt JSON privat terproteksi sesi, ekspor kalender iCalendar RFC 5545 (`.ics`). | **COMPLETE** |
| **F05 (Payment & Recovery)** | Pemulihan link pembayaran lintas sesi, webhook Xendit terverifikasi ledger, penanganan timeout 504, background hold expiry sweep. | **COMPLETE** |
| **F06 (Guest Assistance)** | Form permintaan khusus tamu (special requests), riwayat catatan, pembatalan mandiri berdasar batas waktu. | **COMPLETE** |
| **F07 (Front Desk)** | Roster harian tamu (arrivals, departures, stay-overs), buku catatan serah terima shift (shift handover), pemindahan kamar (room move), perpanjangan inap (extend stay). | **COMPLETE** |
| **F08 (Catalog & Inventory)** | CRUD 7 tipe kamar resmi Pulang ke Uttara, validasi kapasitas invariant, horizon ketersediaan. | **COMPLETE** |
| **F09 (Revenue & Rates)** | Dukungan Room Only & Breakfast, penanganan anak bertingkat, promo code `OCTOBREAK` (diskon 15%), sinkronisasi harga katalog ke engine. | **COMPLETE** |
| **F10 (Channel Sync)** | Pola Transactional Outbox terpasang untuk seluruh perubahan inventori dan status reservasi sebagai pondasi sinkronisasi PMS/OTA. | **FOUNDATION READY** |
| **F11 (Notifications)** | Adapter Resend terpasang, fallback log untuk dev, outbox relay worker untuk email konfirmasi & OTP. | **COMPLETE** |
| **F12 (Staff Identity)** | Manajemen autentikasi staf, enkripsi password bcrypt, session tokens, penegakan Casbin RBAC pada 5 role staf hotel. | **COMPLETE** |
| **F13 (Hotel Policy)** | Penegakan zona waktu `Asia/Jakarta` (WIB) pada aturan pembatalan, konfigurasi hold deadline, batasan usia tamu. | **COMPLETE** |
| **F14 (Finance & Refunds)** | Pencatatan ledger percobaan bayar, antarmuka pengembalian dana (refunds), rekonsiliasi selisih pembayaran, kasus sengketa (*payment cases*). | **COMPLETE** |
| **F15 (Verification & Pilot)** | Table-driven unit tests, database concurrency race detector pass, skrip automasi pengujian E2E (21 assertions pass). | **VERIFIED** |

---

## 7. Audit Kualitas Pengujian & Verifikasi Konkurensi

### 7.1 Linter & Vet Check
Perintah:
```bash
go vet ./...
```
**Hasil:** `0 warnings / 0 errors`. Lulus 100%.

### 7.2 Race Detector Check
Perintah:
```bash
go test -race ./...
```
**Hasil:** Seluruh 23 paket lulus tanpa tabrakan memori atau data race (*zero race conditions detected*).

### 7.3 Statement Coverage Check
Hasil pengujian coverage unit test aktif:
* [`internal/api/http`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http): **96.0%** (Target $\ge 80\%$)
* [`internal/api/http/middleware`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware): **84.8%** (Target $\ge 80\%$)
* [`internal/api/http/handler`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler): **80.3%** (Target $\ge 80\%$)
* [`internal/rates`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/rates): **96.5%**
* [`internal/assistance`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/assistance): **94.9%**
* [`internal/catalog`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/catalog): **92.9%**
* [`internal/adapter/notifier`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/adapter/notifier): **92.2%**
* [`internal/adapter/payment`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/adapter/payment): **87.5%**
* [`internal/frontdesk`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/frontdesk): **83.5%**
* [`internal/finance`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/finance): **83.2%**
* [`internal/platform/featureflag`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/featureflag): **83.1%**
* [`internal/stay`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/stay): **82.1%**
* [`internal/platform/auth`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/auth): **80.9%**
* [`internal/housekeeping`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/housekeeping): **80.3%**

### 7.4 End-to-End Suite Execution
Perintah:
```bash
./testing/e2e/script/router_refactor_e2e.sh
```
**Hasil:**
```text
==============================================================================
TOTAL TESTS : 21
PASSED      : 21
FAILED      : 0
==============================================================================
ALL E2E Test Suites PASSED!
```

---

## 8. Kesimpulan & Rekomendasi Selanjutnya

1. **Status Kesiapan:** Backend berada pada kondisi prima (*production-grade modular monolith*). Refactoring transport layer berhasil mengisolasi komponen HTTP secara bersih tanpa meninggalkan utang teknis (*zero dead code*).
2. **Handoff Frontend:** Seluruh kontrak DTO, kode error RFC 7807, dan endpoint sesi tamu serta staf siap dikonsumsi langsung oleh tim frontend.
3. **Konfirmasi Commit:** Seluruh perubahan siap untuk dikomit ke branch repository.
