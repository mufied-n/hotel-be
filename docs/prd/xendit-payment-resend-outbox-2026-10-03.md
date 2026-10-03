# Product Requirements Document (PRD)
# Integrasi Payment Gateway Xendit & Notifikasi Outbox Resend
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `PRD-XENDIT-RESEND-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / Ready for Execution  
**Referensi Standar:** PCI-DSS v4.0 SAQ A, ISO/IEC 25010, OWASP API Security Top 10 (2023), UU PDP No. 27/2022  

---

## 1. Executive Summary & Latar Belakang Bisnis

Setelah menyelesaikan 22 gap paritas backend (`BE-G01` s/d `BE-G22`) dan membuktikan integritas konkurensi di PostgreSQL 18 nyata, sistem pemesanan hotel **Pulang ke Uttara** membutuhkan integrasi pihak ketiga tingkat produksi (*production-grade integrations*) untuk dua pilar komersial utama:

1. **Pemrosesan Pembayaran Nyata (Xendit Invoice API v2):**
   * Menggantikan rute simulasi `/fake-pay` dengan checkout invoice resmi Xendit (Mendukung QRIS, Virtual Account BCA/Mandiri/BNI/BRI, Kartu Kredit, dan E-Wallet).
   * Kepatuhan **PCI-DSS v4.0 SAQ A**: Data sensitif kartu kredit tidak pernah menyentuh server hotel; tamu dialihkan (*hosted redirect*) ke halaman checkout tersertifikasi PCI-DSS Level 1 Xendit.
   * Penanganan webhook callback atomik dan idempoten dengan verifikasi header `x-callback-token` untuk mencegah manipulasi eksternal.

2. **Pengiriman Notifikasi Email Transaksional (Resend API):**
   * Menggantikan log stub dengan pengiriman email nyata via Resend REST API.
   * Setiap reservasi terkonfirmasi menerima email tanda bukti resmi (*booking confirmation*) dengan template HTML responsif bermerek Pulang ke Uttara.
   * Penegakan idempotensi pengiriman melalui header `Idempotency-Key: email-confirmed-<booking_id>` untuk mencegah tamu menerima email ganda saat worker outbox melakukan retry.
   * Kepatuhan terhadap **UU PDP No. 27/2022**: Mengirimkan email hanya kepada pemesan terdaftar, memuat rincian masa tinggal tanpa mengekspos token sesi rahasia.

---

## 2. Persona & Alur Pengguna (User Journey)

### Persona 1: Tamu Pemesan (Public Guest)
* **Goal:** Membayar kamar yang di-hold secara aman menggunakan metode pembayaran lokal (QRIS, VA Bank) dan segera menerima voucher konfirmasi ke email pribadi.
* **Journey:**
  1. Tamu menyelesaikan review booking di `/booking/review` dan menekan "Lanjut ke Pembayaran".
  2. Backend membuat pesanan (hold 30 menit) dan memanggil Xendit Invoice API untuk memperoleh `payment_url` (`https://checkout.xendit.co/web/...`).
  3. Tamu diarahkan ke Xendit dan menyelesaikan pembayaran sebelum batas hold kedaluwarsa.
  4. Tamu diarahkan kembali ke `/booking/status/:id?status=success`.
  5. Dalam beberapa detik, tamu menerima email konfirmasi resmi dari Resend berisi kode booking, rincian kamar, dan barcode check-in.

### Persona 2: Front Desk / Resepsionis
* **Goal:** Memastikan tamu yang datang ke hotel memiliki booking berstatus `confirmed` yang sah secara finansial dengan riwayat transaksi yang tercatat di ledger `payment_attempts`.

### Persona 3: Finance & Auditor
* **Goal:** Mencocokkan data rekonsiliasi antara mutasi rekening bank Xendit, callback webhook, dan catatan akuntansi di database PostgreSQL hotel.

---

## 3. Matriks Kebutuhan Produk (Product Requirements)

| Kode | Kategori | Kebutuhan Produk | Standar Kepatuhan |
| :--- | :--- | :--- | :--- |
| **PR-X01** | Payment | Pembuatan invoice Xendit dengan parameter eksak: `external_id` (Booking UUID), `amount` (IDR minor unit integer), `payer_email`, `description`, `invoice_duration` (sisa durasi hold), dan URL pengalihan kembali. | PCI-DSS v4.0 SAQ A |
| **PR-X02** | Payment | Penolakan webhook palsu: Validasi header `x-callback-token` menggunakan *constant-time comparison* (`subtle.ConstantTimeCompare`) terhadap token konfigurasi rahasia. | OWASP API Top 10 (Broken Auth) |
| **PR-X03** | Payment | Idempotensi webhook: Status `PAID` mentransisikan booking ke `confirmed`, mencatat status `successful` pada tabel `payment_attempts`, dan mempublikasikan event `booking.confirmed`. Panggilan ulang webhook yang sama harus mengembalikan 200 OK tanpa double execution. | ISO/IEC 25010 (Fault Tolerance) |
| **PR-X04** | Payment | Webhook `EXPIRED`: Jika tamu tidak membayar sebelum batas hold Xendit, webhook membatalkan booking dan mengembalikan inventaris kamar ke stok bebas. | ISO/IEC 27001 A.12.1.2 |
| **PR-R01** | Notifier | Pengiriman email booking confirmation via Resend REST API (`POST https://api.resend.com/emails`) dengan header `Authorization: Bearer <API_KEY>`. | RFC 5322 & RFC 7540 |
| **PR-R02** | Notifier | Idempotensi email: Header `Idempotency-Key: email-confirmed-<booking_id>` dikirimkan ke Resend agar retry otomatis outbox tidak mengirim email duplikat. | UU PDP No. 27/2022 |
| **PR-R03** | Notifier | Template email HTML profesional bermerek Pulang ke Uttara (warna warm-stone `#2D2B2A`, krem `#F7F5F0`, aksen tembaga `#9B4A2C`), memuat nomor reservasi, nama tamu, tanggal check-in/out, dan total bayar IDR. | Brand Visual Guidelines |
| **PR-ENV** | Configuration | Fallback ramah lingkungan: Jika `XENDIT_SECRET_KEY` atau `RESEND_API_KEY` tidak disetel (pada lingkungan lokal/development), sistem fallback ke `payment.FakeGateway` dan `notifier.LogNotifier` secara otomatis. | Ponytail Principle |

---

## 4. Kriteria Penerimaan (Acceptance Criteria)

1. **AC-01 (Xendit Invoice Creation):** Saat `Create` dipanggil pada booking dengan amount Rp 1.100.000, adapter Xendit mengirim HTTP POST ke `api.xendit.co/v2/invoices` dengan Basic Auth, menerima invoice URL, dan mengembalikan `ChargeResult{PaymentURL: "https://checkout.xendit.co/...", Reference: "inv_..."}`.
2. **AC-02 (Xendit Webhook Validation):**
   * Request ke `POST /api/v1/webhooks/xendit` tanpa header `x-callback-token` atau dengan token salah ditolak tegas dengan HTTP 401 Unauthorized.
   * Request dengan token yang cocok dan payload `status: "PAID"` mengonfirmasi booking, mencatat pembayaran sukses di `payment_attempts`, dan membalas 200 OK.
3. **AC-03 (Resend Email Dispatch):**
   * Handler event `booking.confirmed` pada worker outbox memanggil `notifier.SendBookingConfirmed`.
   * Adapter Resend mengirim HTTP POST ke `api.resend.com/emails` dengan header `Idempotency-Key` dan body JSON memuat email tujuan serta HTML template.
   * Jika Resend mengembalikan 200/201, outbox mencatat job sebagai `done`.
4. **AC-04 (Testing & Zero Overengineering):**
   * Seluruh kode pengujian menggunakan Go standard library `httptest.Server` dan table-driven tests dengan statement coverage $\ge 80\%$.
   * Tidak menambahkan library pihak ketiga raksasa (murni menggunakan standard library `net/http`).
