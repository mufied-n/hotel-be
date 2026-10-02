# Software Requirements Specification (SRS) — Batch BE-D: Checkout, Idempotensi & Pemulihan Pembayaran
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** Approved for Implementation
- **Target Parity Gap:** `BE-G07`, `BE-G09`, `BE-G11`, `BE-G12`

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-D01: Skema Tambahan Profil Tamu & Permintaan Khusus (`BE-G07`)
1. Payload `POST /api/v1/bookings` diperluas dengan field:
   - `guest_phone` (string, format E.164, opsional tetapi jika diisi wajib diawali tanda `+` diikuti 7 hingga 15 digit angka).
   - `estimated_arrival_time` (string, opsional, format `HH:MM` 24-jam, misal: `"14:00"`, `"18:30"`).
   - `special_requests` (string, opsional, maksimal 500 karakter Unicode). Jika melebihi 500 karakter, server mengembalikan error `400 SPECIAL_REQUEST_TOO_LONG`.
2. Model respon `Booking` dan `PublicDTO` memuat field `special_requests` dan `estimated_arrival_time`. Nomor telepon `guest_phone` diperlakukan sebagai PII terlindungi (disembunyikan pada `PublicDTO`, ditampilkan pada `X-Guest-Token` atau peran staf).

### FR-D02: Mekanisme Idempotensi Jaringan (`BE-G09`)
1. Server menerima header HTTP `Idempotency-Key` (panjang 1 hingga 64 karakter) pada `POST /api/v1/bookings`.
2. Hash payload dihitung menggunakan SHA-256: `hash = Hex(SHA256(request_body))`.
3. Alur evaluasi idempotensi:
   - Jika `Idempotency-Key` belum ada di tabel `idempotency_keys`: eksekusi use-case pembuatan booking, simpan status code (201) dan response body ke tabel, lalu return ke klien.
   - Jika `Idempotency-Key` sudah ada dengan `request_hash` yang **sama**: kembalikan cached response (HTTP 201 dengan payload identik) tanpa memicu duplikasi transaksi database atau reservasi kamar baru.
   - Jika `Idempotency-Key` sudah ada tetapi `request_hash` **berbeda**: kembalikan error HTTP 409 Conflict dengan kode RFC 7807 `IDEMPOTENCY_CONFLICT`.

### FR-D03: Buku Besar Percobaan Pembayaran (`BE-G11`)
1. Sediakan tabel `payment_attempts` untuk mencatat setiap percobaan pembayaran.
2. Setiap kali use case `CreateCharge` dipanggil, catat baris attempt baru dengan status `initiated`.
3. Saat webhook pembayaran atau simulasi fake-pay tiba:
   - Verifikasi bahwa attempt terdaftar.
   - Catat status menjadi `success` atau `failed`.
   - Update bersifat idempoten: jika webhook dengan nomor referensi yang sama dipanggil berulang kali, kembalikan status sukses (200 OK) tanpa transisi status ilegal.

### FR-D04: Otoritas Mutlak Server terhadap Batas Waktu Hold (`BE-G12`)
1. Respon pembuatan reservasi `POST /api/v1/bookings` wajib menyertakan:
   - `expires_at`: Waktu kedaluwarsa hold (RFC 3339 UTC).
   - `server_time`: Waktu server saat ini (RFC 3339 UTC).
2. Use case `Confirm` (saat webhook pembayaran masuk) wajib mengeksekusi validasi di dalam transaksi `FOR UPDATE`:
   - Jika status booking `pending` dan `time.Now().UTC() > booking.ExpiresAt`:
     - Tolak konfirmasi dengan error domain `ErrHoldExpired`.
     - Kembalikan HTTP 409 Conflict dengan kode `HOLD_EXPIRED`.
     - Catat riwayat di `payment_attempts` dengan status `received_after_expiry`.

---

## 2. Definisi Kode Error RFC 7807

| Kode Error | HTTP Status | Penjelasan |
| :--- | :---: | :--- |
| `INVALID_PHONE_NUMBER` | 400 | Format nomor telepon tamu tidak valid (harus E.164, misal +6281234567890). |
| `INVALID_ARRIVAL_TIME` | 400 | Format jam kedatangan tidak valid (harus HH:MM). |
| `SPECIAL_REQUEST_TOO_LONG` | 400 | Catatan permintaan khusus melebihi batas 500 karakter. |
| `IDEMPOTENCY_CONFLICT` | 409 | Idempotency-Key telah digunakan sebelumnya dengan isi payload yang berbeda. |
| `HOLD_EXPIRED` | 409 | Batas waktu hold kamar (30 menit) telah kedaluwarsa sebelum pembayaran dikonfirmasi. |
