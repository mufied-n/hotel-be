# Software Requirements Specification (SRS): Ketahanan Timeout Gateway Pembayaran & Integritas Buku Besar (BE-R13)

**Nomor Dokumen:** SRS-PULANG-BE-R13  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait Audit & PRD:** `BE-R13`, `PRD-PULANG-BE-R13`

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Pencatatan Intent Percobaan Pembayaran Pra-Gateway
- Sebelum mengeksekusi panggilan jaringan ke penyedia gateway pembayaran (`PaymentGateway.CreateCharge`), domain `booking.Service` wajib membuat entri `PaymentAttempt` baru di tabel `payment_attempts`.
- Entri dibuat dengan atribut:
  - `id`: UUID v7 unik
  - `booking_id`: ID pemesanan aktif
  - `provider`: Nama provider pembayaran (default: `gateway` atau `xendit`)
  - `amount_minor`: Total nominal tagihan dalam integer rupiah
  - `currency`: "IDR"
  - `status`: `"initiated"`
  - `created_at` & `updated_at`: Waktu UTC sekarang

### FR-02: Deteksi Timeout & Pelestarian Stok (Non-Destructive Preservation)
- Sistem wajib menyediakan fungsi pendeteksi error timeout:
  - Mengembalikan `true` apabila error merupakan `context.DeadlineExceeded`, `context.Canceled`, atau memuat substring `timeout`, `deadline exceeded`, `connection refused`, `connection reset`, `502`, `503`, `504`, atau `eof`.
- Jika terdeteksi timeout:
  - Status percobaan pembayaran pada baris `id` diperbarui menjadi `"unknown_timeout"` beserta detail pesan error pada kolom `payload`.
  - Sistem **DILARANG KERAS** memanggil `s.Cancel` atau merilis stok kamar di tabel `inventory`.
  - Sistem mengembalikan error `ErrPaymentGatewayTimeout`.

### FR-03: Penanganan Kegagalan Definitif & Kompensasi (Definitive Failure & Compensation)
- Jika gateway mengembalikan error definitif (bukan timeout, misalnya penolakan parameter 4xx):
  - Status percobaan pembayaran pada baris `id` diperbarui menjadi `"failed"`.
  - Sistem mengeksekusi kompensasi `s.Cancel(ctx, bookingID)` untuk membatalkan booking dan mengembalikan stok kamar.
  - Jika kompensasi `s.Cancel` gagal, sistem wajib mencatat pesan log ERROR terstruktur dengan konteks `booking.compensation.cancel_failed`.
  - Sistem mengembalikan error `ErrPaymentDefinitiveFailure`.

### FR-04: Pembaruan Percobaan Pembayaran Berbasis ID Spesifik
- Antarmuka `PaymentAttemptStore` dan implementasi `PostgresPaymentAttemptStore` wajib menyediakan metode:
  ```go
  UpdateAttemptByID(ctx context.Context, attemptID string, status string, providerReference string, payload map[string]any) error
  ```
- Kueri SQL hanya memperbarui baris dengan `id = $1`, mencegah *status race* antar percobaan pada satu pemesanan yang sama.

---

## 2. Spesifikasi Kontrak HTTP API Transport

### 2.1 Respons Saat Gateway Timeout (HTTP 504 Gateway Timeout)
```http
HTTP/1.1 504 Gateway Timeout
Content-Type: application/json

{
  "error": "koneksi gateway pembayaran terputus, reservasi tetap tersimpan dalam antrean pemulihan",
  "title": "Gateway Timeout",
  "status": 504,
  "detail": "koneksi gateway pembayaran terputus, reservasi tetap tersimpan dalam antrean pemulihan",
  "code": "GATEWAY_TIMEOUT"
}
```

### 2.2 Respons Saat Gateway Penolakan Definitif (HTTP 502 Bad Gateway)
```http
HTTP/1.1 502 Bad Gateway
Content-Type: application/json

{
  "error": "gateway pembayaran menolak transaksi",
  "title": "Bad Gateway",
  "status": 502,
  "detail": "gateway pembayaran menolak transaksi",
  "code": "PAYMENT_FAILED"
}
```

---

## 3. Matriks Status Percobaan Pembayaran (`payment_attempts.status`)

| Nilai Status | Deskripsi Status | Tindakan Terhadap Booking |
| :--- | :--- | :--- |
| `initiated` | Percobaan pembayaran dibuat dan dikirim ke gateway. | Tetap `pending` (hold aktif) |
| `unknown_timeout` | Respons gateway timeout/putus; outcome belum diketahui. | **Tetap `pending` (hold aktif)** |
| `failed` | Gateway menolak transaksi secara definitif. | Dibatalkan (`cancelled`), stok dikembalikan |
| `success` | Webhook mengonfirmasi pembayaran lunas (*PAID/SETTLED*). | Ditransisikan ke `confirmed` |
| `received_after_expiry` | Pembayaran diterima setelah batas waktu hold habis. | Hold expired, diarahkan ke alur refund |
