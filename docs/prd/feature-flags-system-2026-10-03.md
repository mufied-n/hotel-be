# Product Requirements Document (PRD) — Feature Flags & Runtime Configuration System
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen ID:** `PRD-FEATURE-FLAGS-2026-10-03`
- **Versi:** 1.0.0
- **Tanggal:** 2026-10-03
- **Status:** Approved / In Execution
- **Target Persona:** `gm_admin` (General Manager / Sysadmin), `revenue_mgr` (Revenue Manager), `receptionist` (Front Desk), `guest` (Tamu Publik)
- **Modul:** `internal/platform/featureflag`, `internal/api`, `cmd/server`

---

## 1. Latar Belakang & Urgensi Bisnis (Business Background)

Hotel **Pulang ke Uttara** adalah hotel butik bintang-4 dengan 95 kamar fisik di Yogyakarta yang mengoperasikan sistem pemesanan kamar mandiri (*direct booking engine*). Sistem ini mengintegrasikan berbagai modul krusial, antara lain katalog kamar, mesin kuotasi tarif dinamis, checkout ber-idempotensi, payment gateway Xendit, outbox email Resend, portal tamu passwordless, rekonsiliasi refund keuangan, dan status kesiapan kamar housekeeping.

Dalam operasional harian hotel bintang-4 dengan tingkat hunian tinggi (khususnya saat musim liburan dan akhir pekan), muncul kebutuhan operasional yang kritis:
1. **Mitigasi Insiden Pihak Ketiga (Third-Party Incident Mitigation):**
   * Jika penyedia eksternal seperti Xendit Payment Gateway atau Resend Email mengalami gangguan jaringan (*outage*), hotel tidak boleh menghentikan seluruh layanan booking. Hotel membutuhkan tombol darurat (*Kill Switch*) untuk menonaktifkan integrasi tersebut dan beralih ke mode cadangan (*graceful degradation / offline bank transfer*) dalam hitungan detik tanpa perlu melakukan *re-deployment* kode atau restart server binary.
2. **Rilis Bertahap & Pengujian Aman (Canary / Role-Based Rollout):**
   * Sebelum sebuah modul baru (misal: *Housekeeping Readiness Board* atau *Guest OTP Login*) dibuka untuk seluruh tamu publik dan 95 kamar, manajemen memerlukan kapabilitas untuk mengaktifkan fitur tersebut **hanya untuk peran tertentu** (misalnya role `gm_admin` dan `receptionist` sebagai *beta tester* internal).
3. **Kontrol Komersial Tanpa Downtime:**
   * Manajemen tarif dan GM harus dapat mengaktifkan atau menonaktifkan promosi musiman (`OCTOBREAK`), quote locking engine, atau kebijakan pembatalan ketat secara instan saat kampanye pemasaran dimulai atau berakhir.

---

## 2. Profil Persona & Hak Akses (RBAC & Feature Scope)

| Persona | Peran dalam Feature Flags | Hak Akses (*Capabilities*) |
| :--- | :--- | :--- |
| **General Manager (`gm_admin`)** | Pemilik Otoritas Sistem | Membaca daftar status seluruh flag, mengubah status flag (`enabled: true/false`), dan menetapkan filter peran (`allowed_roles`). |
| **Revenue Manager (`revenue_mgr`)** | Pengelola Komersial | Mengaktifkan/menonaktifkan flag terkait komersial: promosi, rate plans, dan quote engine. |
| **Resepsionis & Staf HK** | Pengguna Fitur Internal | Terpengaruh oleh status flag role-based (misal: fitur housekeeping board hanya aktif bila role diizinkan). |
| **Tamu Publik (`guest`)** | Pengguna Eksternal | Terpengaruh oleh status flag publik; jika fitur mati, menerima respons standar `503 FEATURE_DISABLED` atau `404 NOT_FOUND`. |

---

## 3. Matriks 17 Feature Flags Sistem

Sistem menyediakan 17 feature flags terstandardisasi untuk seluruh modul yang ada pada [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go):

| Key Flag | Nama & Kategori | Default | Allowed Roles | Perilaku Saat OFF (Disabled) |
|---|---|:---:|---|---|
| `ff_catalog_write` | Catalog Mutation CRUD | `true` | `["revenue_mgr", "gm_admin"]` | Endpoint POST/PUT/DELETE catalog return `503 FEATURE_DISABLED`. Pembacaan GET catalog tetap aktif. |
| `ff_multi_variant_search` | Multi-Variant Search | `true` | `[]` (Semua) | Endpoint `GET /api/v1/search` return `503 FEATURE_DISABLED`; dialihkan ke availability per tipe kamar. |
| `ff_quote_locking_engine` | 15-Minute Locked Quote | `true` | `[]` (Semua) | Endpoint `POST /api/v1/quotes` nonaktif; booking fallback ke base rate tanpa token quote. |
| `ff_promotions_engine` | Promo Code Discounts | `true` | `[]` (Semua) | Kode promo (misal `OCTOBREAK`) ditolak atau tidak memotong harga. |
| `ff_checkout_idempotency` | Checkout Idempotency Guard | `true` | `[]` (Semua) | Header `Idempotency-Key` di-bypass (booking diproses langsung tanpa deduplikasi replay). |
| `ff_pii_masking_guard` | Guest PII Data Masking | `true` | `[]` (Semua) | Jika OFF, return DTO lengkap tanpa masking PII (khusus internal test). |
| `ff_strict_cancellation_policy` | Strict Cancellation Rules | `true` | `[]` (Semua) | Jika OFF, tamu diizinkan membatalkan kamar non-refundable. |
| `ff_xendit_payment_gateway` | Xendit Invoice Gateway | `true` | `[]` (Semua) | Fallback otomatis ke FakeGateway / instruksi transfer bank manual. |
| `ff_resend_email_notifier` | Resend Transactional Email | `true` | `[]` (Semua) | Fallback otomatis ke LogNotifier (email hanya dicatat di log outbox). |
| `ff_guest_portal_auth` | Guest Passwordless OTP Auth | `true` | `[]` (Semua) | Endpoint `/api/v1/auth/guest/*` return `503 FEATURE_DISABLED`. |
| `ff_guest_my_bookings` | Guest My Bookings Portal | `true` | `[]` (Semua) | Endpoint `/api/v1/guest/bookings*` return `503 FEATURE_DISABLED`. |
| `ff_booking_artifacts_receipt` | Printable Invoice Receipt | `true` | `[]` (Semua) | Endpoint `/receipt` return `503 FEATURE_DISABLED`. |
| `ff_booking_artifacts_icalendar` | RFC 5545 iCalendar (.ics) | `true` | `[]` (Semua) | Endpoint `/calendar.ics` return `503 FEATURE_DISABLED`. |
| `ff_finance_reconciliation` | Finance Cases & Summary | `true` | `["finance", "gm_admin"]` | Endpoint finance audit & cases return `503 FEATURE_DISABLED`. |
| `ff_gateway_automated_refund` | Xendit Automated Refund API | `true` | `["finance", "gm_admin"]` | Eksekusi otomatis ke gateway ditahan; refund dicatat pending manual. |
| `ff_housekeeping_board` | Housekeeping Room Board | `true` | `["housekeeping", "receptionist", "gm_admin"]` | Endpoint `/api/v1/housekeeping/*` return `503 FEATURE_DISABLED`. |
| `ff_room_readiness_checkin_guard`| Check-In Inspected Room Guard | `true` | `[]` (Semua) | Jika OFF, resepsionis dapat check-in ke kamar tanpa syarat status `inspected`. |

---

## 4. Aturan Bisnis (Business Rules)

- **BR-FF-01: Evaluasi Bertingkat (Hierarchical Evaluation):**
  1. Jika flag `enabled == false`, akses langsung **DITOLAK** untuk seluruh aktor dan background worker.
  2. Jika flag `enabled == true`, periksa `allowed_roles`:
     - Jika `allowed_roles` kosong (`[]`), akses **DIIZINKAN** untuk semua role yang lolos otorisasi RBAC Casbin.
     - Jika `allowed_roles` terisi, akses **HANYA DIIZINKAN** jika role caller yang sah terdapat di dalam daftar tersebut.
- **BR-FF-02: Zero External Dependency (Filosofi Ponytail):**
  Sistem feature flag diimplementasikan secara murni dengan Go standard library, PostgreSQL (tabel `feature_flags`), dan Valkey pub/sub tanpa menambahkan vendor SaaS berbayar atau pustaka pihak ketiga yang membengkakkan binary.
- **BR-FF-03: Zero I/O Read Overhead:**
  Evaluasi flag pada setiap HTTP request wajib membaca data dari snapshot memori RAM lokal (`atomic.Pointer`) dengan waktu eksekusi sub-mikrodetik (< 50 nanodetik). Tidak diizinkan melakukan query SQL pada hot path request.
- **BR-FF-04: Standar Respons RFC 7807:**
  Permintaan ke endpoint yang sedang dinonaktifkan wajib mengembalikan format *Problem Details* dengan status `HTTP 503 Service Unavailable` atau `HTTP 404 Not Found` dan kode mesin `FEATURE_DISABLED`.

---

## 5. Kriteria Keberterimaan (Acceptance Criteria)

- [ ] **AC-FF-01:** Tabel `feature_flags` dibuat melalui Goose migration `00013_feature_flags.sql` lengkap dengan seed 17 flag resmi.
- [ ] **AC-FF-02:** Package `internal/platform/featureflag` menyediakan interface `Manager` dengan method `IsEnabled(ctx context.Context, key string) bool`.
- [ ] **AC-FF-03:** Chi middleware `RequireFeature(ff, key)` menolak request saat flag nonaktif atau role tidak berhak dengan HTTP 503 RFC 7807.
- [ ] **AC-FF-04:** API internal manajemen flag (`GET /api/v1/admin/feature-flags` dan `PUT /api/v1/admin/feature-flags/{key}`) terproteksi RBAC untuk role `gm_admin`.
- [ ] **AC-FF-05:** Mekanisme sinkronisasi hybrid: Perubahan flag via API langsung mempublikasikan event ke Valkey channel `pku:feature_flags:updated` dan menyinkronkan RAM seluruh node dalam < 10 ms, didukung jaring pengaman polling berkala (30s).
- [ ] **AC-FF-06:** Seluruh unit test menggunakan table-driven test dengan coverage $\ge 80\%$ dan lulus `go vet ./...`.
