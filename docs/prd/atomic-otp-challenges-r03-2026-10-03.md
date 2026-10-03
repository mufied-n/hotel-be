# Product Requirements Document (PRD) — Atomic OTP Challenge & Attempt Limiting (BE-R03)

**Fitur:** Atomic OTP Challenge, Attempt Limiting & Single-Use Verification  
**ID Gap:** BE-R03 (P1)  
**Terkait:** F02, BE-G13, BE-G14  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  
**Target:** Monolith Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)  

---

## 1. Konteks Bisnis & Masalah

Portal tamu *Pulang ke Uttara* menggunakan mekanisme autentikasi *passwordless* berbasis *One-Time Password* (OTP) 6 digit via email untuk mengakses riwayat reservasi dan faktur resmi (*Booking Saya* / F02).

Untuk mencegah serangan *brute force* dan penyalahgunaan kredensial, sistem menetapkan:
1. Batas maksimal 3 kali percobaan salah per kode OTP.
2. Batas waktu kedaluwarsa OTP 10 menit.
3. *Cooldown* permintaan OTP baru 60 detik per alamat email.
4. *Single-use consumption*: Satu kode OTP hanya dapat ditukarkan menjadi tepat satu sesi tamu aktif.

### Temuan Masalah (BE-R03)
1. **Verifikasi dan Konsumsi Terpisah (Non-Atomik)**: Pembacaan tantangan aktif, komparasi hash kode, pencatatan percobaan salah (`attempts`), penandaan verifikasi (`verified_at`), dan pembuatan sesi dilakukan melalui kueri terpisah tanpa penguncian baris (`FOR UPDATE`).
2. **Potensi Eksploitasi Konkurensi (Race Condition)**:
   - *Parallel Guessing*: Beberapa tebakan salah yang dikirim serentak dapat membaca nilai `attempts` yang sama (misal 0), lalu masing-masing menuliskan `attempts = 1`, sehingga pembatas 3 kali percobaan dapat dilewati (*brute-force window*).
   - *Parallel Replay*: Dua atau lebih permintaan verifikasi dengan OTP yang valid secara serentak dapat membaca `verified_at == nil` sebelum sempat ditandai, menghasilkan beberapa token sesi aktif sekaligus dari satu OTP.
   - *Cooldown Bypass*: Permintaan tantangan OTP paralel dapat membaca state sebelum tantangan pertama tersimpan, memicu pengiriman OTP ganda dan melewati jeda 60 detik.
3. **Pengabaian Error Persistensi**: Pemanggilan `UpdateChallengeAttempts` dan `MarkChallengeVerified` mengabaikan error database (`_ = ...`), sehingga kegagalan DB tidak menggagalkan verifikasi.

---

## 2. Kriteria Penerimaan (Acceptance Criteria)

- [x] **AC-01 (Single-Use OTP Atomicity):** 20 permintaan verifikasi serentak dengan satu kode OTP valid hanya menghasilkan tepat 1 sesi tamu aktif (1 sukses, 19 ditolak dengan `ErrInvalidOrExpiredCode`).
- [x] **AC-02 (Strict Attempt Counting):** Percobaan verifikasi kode salah secara paralel tidak dapat melampaui batas maksimal percobaan (`max_attempts = 3`). Setelah 3 kali salah, seluruh percobaan berikutnya ditolak dengan `ErrMaxAttemptsExceeded`.
- [x] **AC-03 (Atomic Cooldown Protection):** Permintaan tantangan OTP paralel untuk email yang sama tidak dapat melewati *cooldown* 60 detik. Tepat satu permintaan berhasil membuat OTP, permintaan konkuren lainnya ditolak dengan `ErrRateLimited`.
- [x] **AC-04 (Rollback Integrity):** Jika pembuatan sesi gagal di tengah transaksi verifikasi, transaksi di-rollback sepenuhnya; OTP tidak dianggap terpakai (`verified_at` tidak ditandai) dan tidak ada token sesi ilegal yang diterbitkan.
- [x] **AC-05 (Zero Error Suppression):** Seluruh kegagalan persistensi database dipropagasikan secara transparan, bukan diabaikan.
