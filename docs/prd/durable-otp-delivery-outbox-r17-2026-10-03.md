# Product Requirements Document (PRD) — Pengiriman OTP Andal Berbasis Outbox & Anti-Enumeration (BE-R17)

**Nomor Dokumen:** PRD-PULANG-BE-R17  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering & Architecture Agent  
**Target Fitur / Gap:** [`BE-R17`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md) (*OTP delivery tidak memiliki retry durable dan tetap mengaku terkirim*)  
**Hubungan:** BE-G16; F02 (*Guest Access & Session*), F11 (*Notification Delivery*)  

---

## 1. Latar Belakang & Masalah Bisnis

Pada arsitektur awal modul autentikasi portal tamu (*Guest Portal Auth*):
1. **Kegagalan Pengiriman Siluman (*Silent Delivery Failure*):**  
   Saat tamu meminta kode OTP 6-digit untuk login ke *Booking Saya* (`POST /api/v1/auth/guest/challenge`), sistem menyimpan *challenge* ke database lalu memanggil notifier email (*Resend REST API*) secara inline. Jika Resend mengalami *network blip*, *timeout*, atau *rate limit*, error hanya dicatat ke log sementara tamu mendapatkan respons sukses palsu (*false positive delivery*).
2. **Ketiadaan *Durable Retry*:**  
   Tidak ada mekanisme antrian persisten untuk mencoba ulang (*retry*) pengiriman kode OTP yang gagal terkirim. Akibatnya, tamu terkunci di halaman login selama masa *cooldown* 60 detik tanpa pernah menerima email.
3. **Risiko Kolisi *Idempotency-Key*:**  
   Notifier Resend sebelumnya membentuk header `Idempotency-Key` menggunakan format `otp-{email}-{unix_minute}`. Dua permintaan verifikasi berbeda yang dibuat dalam menit yang sama menghasilkan key identik, berisiko menyebabkan *provider* menolak atau mengabaikan salah satu pesan.
4. **Klaim Pengiriman Prematur (*Premature Delivery Claim*):**  
   Pesan API menyatakan `"kode verifikasi 6 digit telah dikirimkan ke email Anda"`, padahal pengiriman baru saja diajukan dan belum terkonfirmasi oleh SMTP/gateway pengirim. Ini melanggar prinsip kejujuran sistem (*honest API design*) dan standar OWASP ASVS v4.

---

## 2. Tujuan & Nilai Bisnis (*Business Objectives*)

1. **Keandalan Pengiriman 99.9% (*Delivery Reliability*):**  
   Memanfaatkan *Transactional Outbox Pattern* di PostgreSQL sehingga pembuatan tantangan OTP dan antrian pengiriman terikat dalam satu transaksi atomik. Jika database gagal menyimpan tantangan, email tidak akan dikirim. Jika database sukses, pengiriman dijamin dieksekusi dengan *exponential backoff retry*.
2. **Idempotensi Berbasis Tantangan Unik (*Per-Challenge Idempotency*):**  
   Header `Idempotency-Key` Resend wajib menggunakan `otp-challenge-{challenge_id}` (UUID v7 unik), mencegah tabrakan pada permintaan paralel dan menjamin tidak ada duplikasi email pada saat *worker retry*.
3. **Pencegahan Pengiriman OTP Kedaluwarsa (*Anti-Stale OTP Delivery*):**  
   Sebelum mengirim ulang, *outbox worker* memverifikasi apakah masa berlaku kode (*10 menit*) masih aktif atau apakah tamu telah memverifikasi kode lain. Jika sudah kedaluwarsa, pesan didiskualifikasi (*discarded*) dan tidak dikirimkan ke tamu.
4. **Keamanan Anti-Enumerasi & Pesan Jujur (*Anti-Enumeration & Honest Status*):**  
   Respons API menggunakan status penerimaan (*accepted*) tanpa membocorkan apakah email terdaftar, dan tanpa menjanjikan pengiriman instan.

---

## 3. Persona Pengguna & Matriks Hak Akses

| Persona | Kebutuhan / Perilaku | Hak Akses Fitur |
| :--- | :--- | :--- |
| **Tamu Hotel Publik (`guest`)** | Meminta kode OTP untuk melihat reservasi aktif tanpa takut kode hilang di tengah jalan saat jaringan internet provider email lambat. | Memanggil `POST /api/v1/auth/guest/challenge` dan `POST /api/v1/auth/guest/verify`. |
| **Front Desk / Resepsionis** | Membantu tamu yang mengalami kendala login tanpa perlu mengakses isi kode OTP. | Tidak memiliki akses membaca kode OTP tamu (UU PDP No. 27/2022). |
| **DevOps / Engineer** | Memantau metrik retry dan kegagalan antrian outbox melalui log terstruktur yang telah diredaksi (tanpa bocoran OTP plaintext). | Monitoring healthz outbox relay & dead-letter queue. |

---

## 4. Kriteria Penerimaan (*Acceptance Criteria*)

- **AC-01 (Atomisitas Transaksional Outbox):**  
  Penyimpanan tantangan OTP di tabel `guest_auth_challenges` dan pencatatan event ke tabel `outbox` dengan topik `guest.otp_dispatch` wajib dieksekusi dalam satu transaksi database PostgreSQL yang sama (`BEGIN ... COMMIT`).
- **AC-02 (Durable Retry dengan Exponential Backoff):**  
  Jika pemanggilan provider email mengalami kegagalan (5xx, timeout, network error), *OutboxRelay* wajib menjadwalkan retry secara otomatis dengan jeda eksponensial (5s, 10s, 20s, ...) hingga batas maksimum *MaxAttempts* (8 kali).
- **AC-03 (Idempotency Key per Challenge ID):**  
  Setiap panggilan pengiriman OTP ke provider wajib membawa `Idempotency-Key: otp-challenge-{challenge_id}`. Retry atas event yang sama wajib memakai key yang sama; tantangan baru wajib menghasilkan key baru.
- **AC-04 (Proteksi Kode Kedaluwarsa / Usang):**  
  Jika event `guest.otp_dispatch` diproses setelah masa berlaku tantangan (`expires_at`) terlampaui atau setelah kode diverifikasi (`verified_at IS NOT NULL`), worker wajib menandai event selesai (`done`) tanpa mengirimkan email ke tamu.
- **AC-05 (Respons API Jujur & Anti-Enumerasi):**  
  Respons `POST /api/v1/auth/guest/challenge` wajib berstatus HTTP 200 dengan payload `"status": "ok"`, `"delivery_status": "accepted"`, dan pesan netral yang tidak menjamin pengiriman instan, aman dari serangan enumerasi email (UU PDP No. 27/2022).
- **AC-06 (Redaksi PII & Zero-Leak Plaintext OTP di Log Production):**  
  Isi kode OTP 6-digit dilarang keras dicatat dalam log production ataupun terekspos ke metadata respons publik.
