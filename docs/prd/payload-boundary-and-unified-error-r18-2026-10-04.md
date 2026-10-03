# Product Requirements Document (PRD) — Penyeragaman Batas Payload & Skema Error API (BE-R18)

**Nomor Dokumen:** PRD-PULANG-BE-R18  
**Tanggal:** 4 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering & Architecture Agent  
**Target Fitur / Gap:** [`BE-R18`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md) (*Boundary payload dan error API belum seragam*)  
**Hubungan:** BE-G09, BE-G15; F01 (*Booking Journey Completion*), F02 (*Guest Access & Session*)  

---

## 1. Latar Belakang & Masalah Bisnis

Pada audit sistem perhotelan Pulang ke Uttara:
1. **Ketiadaan Pembatasan Ukuran Request Body (*Unbounded Payload Consumption*):**  
   Endpoint kritis seperti pembuatan booking (`POST /api/v1/bookings`) dan webhook Xendit (`POST /api/v1/webhooks/xendit`) membaca seluruh body ke memori menggunakan `io.ReadAll` tanpa `http.MaxBytesReader`. Ini membuka celah kerentanan *Denial of Service* (DoS) berbasis konsumsi memori tak terbatas (OWASP API4:2023 *Unrestricted Resource Consumption*).
2. **Ketiadaan Validasi Batas Header `Idempotency-Key`:**  
   Header `Idempotency-Key` dibaca tanpa validasi panjang string, padahal standar idempotensi menentukan panjang 1 hingga 64 karakter. Caller dapat mengirim key sembarang ribuan karakter yang membebani memori dan database store.
3. **Format Error Tidak Seragam (*Inconsistent Error Schemas*):**  
   - Modul booking dan operasional menggunakan RFC 7807 Problem Details (`{error, title, status, detail, code}`).
   - Modul portal tamu dan autentikasi menggunakan format `{error, message}` di mana `error` berisi *machine code* dan `message` berisi teks deskripsi.
   - Frontend Nuxt terpaksa melakukan normalisasi ganda untuk mendeteksi apakah suatu error harus dibaca dari `.detail`, `.message`, `.code`, atau `.error`.

---

## 2. Tujuan & Nilai Bisnis (*Business Objectives*)

1. **Perlindungan Terhadap Serangan DoS Payload Raksasa (*Resource Defense*):**  
   Membatasi ukuran request body maksimal 1 MB pada seluruh endpoint API menggunakan `http.MaxBytesReader`. Permintaan yang melebihi batas langsung ditolak dengan HTTP 413 *Payload Too Large* (`PAYLOAD_TOO_LARGE`) sebelum membebani CPU/RAM.
2. **Penegakan Validasi Header Idempotency-Key (1–64 Karakter):**  
   Memastikan `Idempotency-Key` divalidasi panjangnya (1 s/d 64 karakter); jika melebihi batas, ditolak dengan HTTP 400 *Bad Request* (`INVALID_IDEMPOTENCY_KEY`).
3. **Unifikasi Skema Error Dual-Shape Kompatibel (*Unified Error Contract*):**  
   Menyatukan respons error agar setiap payload error selalu memuat baik kode mesin (`code` & `error`), deskripsi manusiawi (`message` & `detail`), dan status numerik (`status`). Hal ini memberikan kompatibilitas 100% tanpa memutus client/frontend yang sudah ada.

---

## 3. Persona Pengguna & Matriks Hak Akses

| Persona | Kebutuhan / Interaksi | Manfaat Penyeragaman |
| :--- | :--- | :--- |
| **Frontend Engineer & Tamu Web** | Mengonsumsi endpoint booking, auth, dan portal tamu. | Penanganan error tunggal tanpa perkecualian per modul. Pesan validasi tampil seragam. |
| **Payment Gateway Webhook (Xendit)** | Mengirim callback notifikasi pembayaran. | Payload terlindungi batas ukuran dan error terdefinisi rapi. |
| **DevOps / Security Engineer** | Menjaga stabilitas aplikasi dari payload abuse. | Beban server terprediksi, zero OOM karena payload raksasa. |

---

## 4. Kriteria Penerimaan (*Acceptance Criteria*)

- **AC-01 (Batas Ukuran Request Body 1 MB):**  
  Seluruh endpoint JSON yang menerima body dibatasi maksimal 1 MB (1,048,576 bytes). Request yang melampaui batas wajib mengembalikan HTTP 413 *Payload Too Large* dengan kode error `PAYLOAD_TOO_LARGE`.
- **AC-02 (Validasi Panjang Idempotency-Key):**  
  Header `Idempotency-Key` pada `POST /api/v1/bookings` wajib dibatasi maksimal 64 karakter. Jika melebihi batas, server wajib mengembalikan HTTP 400 *Bad Request* dengan kode `INVALID_IDEMPOTENCY_KEY`.
- **AC-03 (Skema Error Dual-Shape yang Kompatibel):**  
  Seluruh respons error di API wajib menyediakan atribut konsisten:
  - `code` (string machine-readable)
  - `error` (string machine-readable / description fallback)
  - `message` (string human-readable)
  - `detail` (string human-readable)
  - `status` (integer HTTP status)
- **AC-04 (Penanganan JSON Rusak yang Seragam):**  
  Request dengan format JSON rusak wajib menghasilkan HTTP 400 *Bad Request* dengan kode `INVALID_JSON`.
