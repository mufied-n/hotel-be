# PRD — Production Provider Fail-Fast & Capability Readiness (BE-R16)

**Fitur:** Production Provider Fail-Fast, Safe Readiness Capability & Fake-Pay Hardening  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** BE-R16 (P0), BE-G10, BE-G16, F05, F11, F15  

---

## 1. Latar Belakang & Masalah Bisnis

Pada sistem reservasi hotel berbintang 4 seperti **Pulang ke Uttara (Yogyakarta)**, integritas transaksi finansial dan perlindungan data pribadi tamu (UU PDP No. 27/2022) adalah prioritas mutlak.

Sebelum perbaikan ini:
1. **Silent Fallback di Lingkungan Produksi:** Bila server dijalankan dengan `APP_ENV=production` tetapi variabel lingkungan kredensial vendor pihak ketiga (`XENDIT_SECRET_KEY`, `XENDIT_WEBHOOK_TOKEN`, `RESEND_API_KEY`) tidak terkonfigurasi, aplikasi tetap melakukan booting dengan memilih `payment.NewFake()` dan `notifier.NewLog()`. Akibatnya, server tampak sehat di monitoring, namun tautan pembayaran menjadi dummy dan notifikasi konfirmasi serta kode OTP tamu masuk ke log aplikasi alih-alih dikirimkan secara riil via email.
2. **Potensi Kebocoran Kredensial OTP:** `LogNotifier` mencatat kode OTP verifikasi portal tamu ke structured log dalam bentuk teks polos (`otp: <code_otp>`). Jika fallback log aktif di lingkungan produksi, log aggregator mengekspos token otentikasi tamu.
3. **Ambiguity Status Readiness vs Liveness:** Probe `/healthz` hanya mengembalikan `{"status":"ok"}`, sedangkan `/ready` hanya menguji koneksi Postgres & Valkey tanpa mengekspos status kapabilitas provider (payment gateway & notifier) yang aktif, sehingga frontend tidak dapat membedakan status sandbox vs live.
4. **SQL Error Leakage pada Simulasi Dev (`/fake-pay`):** Endpoint simulasi dev `/fake-pay/:ref` menerima parameter tanpa validasi format UUID. Ketika menerima string acak, error PostgreSQL (`invalid input syntax for type uuid`) dibocorkan mentah-mentah ke response HTTP 500.

---

## 2. Persona & Pengguna Terkait

1. **DevOps & Platform Engineer (`sysadmin` / `sre`):**
   - Membutuhkan sistem yang gagal *booting* secara terkontrol (*fail-fast*) dengan pesan error eksplisit saat konfigurasi produksi tidak lengkap.
   - Membutuhkan probe `/ready` yang transparan menyajikan status kesiapan dependensi dan identitas provider.
2. **Tamu Publik (`guest`):**
   - Terlindungi dari transaksi semu (*fake charge*) saat memesan kamar di lingkungan produksi.
   - Terlindungi dari paparan kode OTP di log server terbuka.
3. **Frontend Engineer:**
   - Memperoleh informasi kapabilitas terverifikasi dari `/ready` untuk menampilkan lencana mode (Sandbox / Live) secara akurat.

---

## 3. Matriks Kebutuhan & Acceptance Criteria

| ID | Kategori | Deskripsi | Acceptance Criteria |
|---|---|---|---|
| AC-01 | Config Fail-Fast | Validasi startup ketat pada `APP_ENV=production` | `cfg.Validate()` menolak startup (exit code 1) jika `XENDIT_SECRET_KEY`, `XENDIT_WEBHOOK_TOKEN`, atau `RESEND_API_KEY` kosong. Pesan error jelas dan spesifik. |
| AC-02 | Production Safety Guard | Larangan mutlak adapter Fake/Log di Production | `cmd/server/main.go` memberikan proteksi berlapis; jika `APP_ENV=production`, inisialisasi `FakeGateway` atau `LogNotifier` langsung menggagalkan proses dengan log level FATAL/ERROR. |
| AC-03 | Log Redaction | Redaksi OTP pada `LogNotifier` | `LogNotifier` mendukung masking OTP (`[REDACTED]`). Jika fallback aktif di luar dev murni atau jika masking diaktifkan, teks OTP asli tidak tercatat ke log. |
| AC-04 | Capability Readiness | Pemisahan Liveness dan Capability Health | Endpoint `/ready` mengembalikan HTTP 200 dengan payload JSON yang mencakup `status: "ready"`, `environment`, `payment_gateway` (`"xendit"` / `"fake"`), dan `notifier` (`"resend"` / `"log"`). Bila dependensi tidak sehat, mengembalikan 503 `unavailable`. |
| AC-05 | Fake-Pay Hardening | Validasi UUID & Sanitasi Error pada Dev Endpoint | `/fake-pay/:ref` memvalidasi bahwa `booking_id` adalah UUID valid. Input non-UUID mengembalikan 400 Bad Request dengan kode `INVALID_BOOKING_ID` (bukan 500 SQL syntax error). Route `/fake-pay` tetap dilarang keras di production (404 Not Found). |

---

## 4. Kebutuhan Non-Fungsional (NFR)

1. **Keamanan (Security):** Mencegah *information disclosure* CWE-209 (error leakage) dan CWE-532 (sensitive data in logs).
2. **Kepatuhan (Compliance):** PCI-DSS v4.0 Requirement 6.4 (secure configuration & environment separation) dan UU PDP No. 27/2022 Pasal 35 (keamanan pemrosesan data pribadi).
3. **Observabilitas (Observability):** Logging terstruktur menggunakan `log/slog` dengan key yang konsisten (`environment`, `payment_gateway`, `notifier`).
