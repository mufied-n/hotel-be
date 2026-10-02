# Software Requirements Specification (SRS)
# Hotel Booking Engine Parity & Gap Remediation
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `SRS-GAP-PARITY-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / Specification Baseline  
**Dokumen PRD Terkait:** [`docs/prd/hotel-booking-parity-and-gap-remediation-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/hotel-booking-parity-and-gap-remediation-2026-10-03.md)  

---

## 1. Pendahuluan & Cakupan Sistem

Dokumen Spesifikasi Kebutuhan Perangkat Lunak (*Software Requirements Specification* / SRS) ini merinci kebutuhan teknis, kontrak antarmuka API, dan skema database untuk menutup seluruh 22 celah (*gap*) yang ditemukan pada audit `BE-G01` sampai dengan `BE-G22`.

---

## 2. Kebutuhan Fungsional Terperinci (FR-01 s/d FR-22)

### Kategori A: Keamanan, Akses & Kontrak API (BE-A)

* **FR-10 (Gate Rute Simulasi):** Sistem wajib menonaktifkan rute `/fake-pay/*` ketika konfigurasi `APP_ENV=production`. Sistem wajib menolak booting jika pada mode produksi secret webhook atau kredensial payment gateway nyata tidak disuplai.
* **FR-13 (Perlindungan PII & Guest Scoped Token):**
  * Setiap pembuatan pemesanan berhasil menghasilkan `guest_token` (token acak 32-byte berkekuatan tinggi).
  * Request `GET /api/v1/bookings/:id` publik tanpa token hanya mengembalikan DTO publik minimal (`id`, `status`, `room_type_name`, `check_in`, `check_out`).
  * Detail sensitif (nama, email, nomor HP, rincian pembayaran) hanya dikembalikan jika request menyertakan header `X-Guest-Token: <token>` yang cocok atau token staf berwenang.
* **FR-14 (Otentikasi Staf Terverifikasi & Fail-Closed RBAC):**
  * Header `X-User-Role` dan `X-User-ID` dari klien luar wajib diabaikan / dibersihkan.
  * Autentikasi staf wajib menggunakan `Authorization: Bearer <jwt/session_token>` yang divalidasi ke tabel `staff_users` (`is_active = TRUE`).
  * Jika Casbin Enforcer gagal dimuat, router wajib merespon dengan `503 Service Unavailable` (**Fail-Closed**), bukan meloloskan request.
* **FR-15 (Rate Limiting & Standarisasi Error RFC 7807):**
  * Membatasi request pencarian (maksimal 30 req/menit per IP) dan create booking (maksimal 5 req/menit per IP) dengan respon `429 Too Many Requests`.
  * Seluruh respon error mengikuti format Problem Details RFC 7807:
    ```json
    {
      "type": "https://api.pulangkeuttara.com/errors/invalid-date-range",
      "title": "Invalid Date Range",
      "status": 400,
      "detail": "check_in date must be prior to check_out date",
      "code": "INVALID_DATE_RANGE"
    }
    ```

### Kategori B: Katalog & Pencarian Multi-Varian (BE-B)

* **FR-01 (Katalog Fisik Pulang ke Uttara):**
  * Mendukung 5 room families dan 7 sellable variants (Deluxe Balcony King/Twin, Deluxe Bay Window King/Twin, Executive Suite, Suite, Family Suite).
  * Endpoint `GET /api/v1/catalog/rooms` mengembalikan spesifikasi kamar, fasilitas (*amenities*), kapasitas ranjang, luas area, dan foto beresolusi tinggi.
* **FR-02 (Pencarian Lintas Varian & Validasi Kontinu):**
  * Endpoint `GET /api/v1/search?check_in=YYYY-MM-DD&check_out=YYYY-MM-DD&adults=2&children=0` mengembalikan ketersediaan seluruh varian kamar.
  * Validasi memeriksa setiap malam secara kontinu. Jika satu malam tidak memiliki stok, varian tersebut ditandai `is_available = false` dengan keterangan `reason = "SOLD_OUT_ON_SOME_DATES"`.
* **FR-03 (Batas Okupansi & Usia Anak):**
  * Menolak pencarian atau pemesanan dengan jumlah tamu melebihi kapasitas fisik unit (`max_capacity`).
  * Mendukung input usia anak (`child_ages`) untuk menentukan kelayakan ranjang tambahan (*extra bed*) atau kebijakan gratis untuk bayi/balita.
  * Durasi menginap dibatasi maksimal 30 malam.
* **FR-18 (Rolling Horizon & Blok Kamar Pemeliharaan):**
  * Inventaris tersedia otomatis bergulir (*rolling window*) 365 hari ke depan.
  * Staf berwenang dapat memasukkan blok pemeliharaan (*maintenance hold*) yang secara otomatis memotong stok kamar yang dapat dipesan publik.

### Kategori C: Tarif, Paket, Quote & Kebijakan (BE-C)

* **FR-04 (Rate Plans & Sarapan):**
  * Mendukung skema tarif: `ROOM_ONLY` (hanya kamar) dan `BED_AND_BREAKFAST` (termasuk sarapan untuk jumlah tamu terdaftar).
  * Mendukung kode promosi (*promo code*) yang memverifikasi periode inap dan batas penggunaan kuota promo.
* **FR-05 (Monetary Contract & Rincian Harga Presisi):**
  * Seluruh nilai keuangan menggunakan integer minor unit dalam mata uang IDR (`exponent = 0`).
  * Rincian harga menyajikan: `base_rate`, `tax_amount` (PB1 10%), `service_charge`, dan `discount_amount`.
* **FR-06 (Quote Bertenggat Waktu / TTL):**
  * Setiap hasil pilihan harga menghasilkan `quote_id` dengan TTL 15 menit.
  * Pembuatan reservasi wajib menyertakan `quote_id`. Jika tarif telah berubah atau quote kedaluwarsa, sistem menolak dengan kode `QUOTE_EXPIRED` (HTTP 409).
* **FR-08 (Penegakan Kebijakan Pembatalan & Terms Consent):**
  * Sistem merekam snapshot kebijakan pembatalan saat reservasi dibuat (`policy_type`: `NON_REFUNDABLE` atau `FREE_CANCELLATION_UNTIL_H3`).
  * Request pemesanan wajib menyertakan persetujuan syarat & ketentuan: `terms_consented = true`. Sistem merekam timestamp persetujuan.
  * Tamu dilarang membatalkan reservasi berstatus `NON_REFUNDABLE`.
* **FR-19 (Metadata Properti & Timezone):**
  * Seluruh perhitungan tanggal terikat secara kaku pada zona waktu `Asia/Jakarta` (WIB / UTC+7). Waktu check-in default 15:00 WIB dan check-out 12:00 WIB.

### Kategori D: Checkout, Idempotensi & Pemulihan Pembayaran (BE-D)

* **FR-07 (Detail Kontak & Permintaan Khusus):**
  * Validasi nama tamu minimal 3 karakter, format email terstandarisasi, dan nomor telepon berformat E.164.
  * Kolom catatan khusus (`special_requests`) dibatasi maksimal 500 karakter teks Unicode dengan penandaan bahwa permintaan bersifat *subject to availability*.
* **FR-09 (Idempotensi Create Booking):**
  * Endpoint `POST /api/v1/bookings` mewajibkan header `Idempotency-Key: <UUID>`.
  * Jika request dengan key yang sama tiba dalam 24 jam dengan payload identik, sistem mengembalikan respon sukses sebelumnya tanpa mengurangi stok inventaris kembali.
* **FR-11 (Ledger Percobaan Pembayaran):**
  * Setiap interaksi gateway pembayaran dicatat di tabel `payment_attempts` dengan referensi unik, jumlah uang, status (`pending`, `completed`, `failed`), dan payload webhook.
* **FR-12 (Otoritas Batas Waktu Hold Kamar):**
  * Tenggat waktu hold kamar (`expires_at`) disimpan di database.
  * Webhook konfirmasi pembayaran memeriksa apakah saat konfirmasi tiba, status masih `pending` dan waktu server belum melewati `expires_at`.

### Kategori E: Keandalan Operasional & Konkurensi (BE-E)

* **FR-16 (Notifikasi Email Nyata & Deduplikasi Outbox):**
  * Event outbox `booking.confirmed` memicu pengiriman email voucher resmi kepada tamu dengan template HTML responsif.
  * Pengiriman menggunakan deduplikasi berbasis `idempotency_key` agar email tidak pernah terkirim ganda saat terjadi worker restart.
* **FR-17 (Alokasi Kamar Paralel Bebas False-Conflict):**
  * Algoritma `PickAndAssignRooms` menggunakan `SELECT ... FOR UPDATE SKIP LOCKED` atau kunci berurutan agar check-in simultan untuk dua kamar berbeda tidak saling menggagalkan transaksi.
* **FR-21 (Observabilitas & Graceful Shutdown):**
  * Pemeriksaan `rows.Err()` pada background sweeper.
  * Graceful shutdown menunggu penyelesaian job outbox in-flight hingga batas timeout 10 detik sebelum menutup pool koneksi database.
* **FR-22 (Penanganan Early Checkout & Batas No-Show):**
  * Reservasi yang mengalami `early_checkout` mengembalikan ketersediaan kamar untuk sisa malam yang belum dijalani.
  * Status `no_show` hanya dapat dieksekusi setelah pukul 06:00 WIB di hari setelah jadwal check-in (H+1).

---

## 3. Spesifikasi Kontrak Antarmuka HTTP (API Contracts)

### 3.1 Endpoint Pencarian Ketersediaan Multi-Varian
* **Request:**
  ```http
  GET /api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&children=1&child_ages=5 HTTP/1.1
  Accept: application/json
  ```
* **Response (HTTP 200 OK):**
  ```json
  {
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "nights": 2,
    "results": [
      {
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "name": "Deluxe Balcony",
        "variant": "King Bed",
        "max_capacity": 3,
        "available_units": 4,
        "is_available": true,
        "rate_plans": [
          {
            "quote_id": "quo-8f921a-412",
            "rate_plan_code": "ROOM_ONLY",
            "name": "Room Only",
            "cancellation_policy": "FREE_CANCELLATION_UNTIL_H3",
            "total_price": {
              "currency": "IDR",
              "amount": 2200000,
              "breakdown": {
                "base_amount": 2000000,
                "tax_amount": 200000,
                "service_charge": 0
              }
            },
            "expires_at": "2026-10-03T02:00:00Z"
          },
          {
            "quote_id": "quo-8f921a-413",
            "rate_plan_code": "BED_AND_BREAKFAST",
            "name": "Bed & Breakfast",
            "meal_inclusion": "Daily Breakfast for 2 Adults",
            "cancellation_policy": "NON_REFUNDABLE",
            "total_price": {
              "currency": "IDR",
              "amount": 2500000,
              "breakdown": {
                "base_amount": 2272727,
                "tax_amount": 227273,
                "service_charge": 0
              }
            },
            "expires_at": "2026-10-03T02:00:00Z"
          }
        ]
      }
    ]
  }
  ```

### 3.2 Endpoint Create Booking (Idempoten dengan Quote & Consent)
* **Request:**
  ```http
  POST /api/v1/bookings HTTP/1.1
  Content-Type: application/json
  Idempotency-Key: 7b29a263-d14f-4a0b-934c-6238b97d3910

  {
    "quote_id": "quo-8f921a-413",
    "guest_details": {
      "full_name": "Budi Santoso",
      "email": "budi.santoso@example.com",
      "phone_number": "+6281234567890"
    },
    "stay_preferences": {
      "estimated_arrival_time": "15:00",
      "special_requests": "Mohon lantai tinggi non-smoking bila tersedia."
    },
    "terms_consented": true
  }
  ```
* **Response (HTTP 201 Created):**
  ```json
  {
    "booking_id": "01900000-0000-7000-8000-000000000010",
    "guest_access_token": "gst_9fa281b9d031cba884210f92",
    "status": "pending",
    "room_name": "Deluxe Balcony (King Bed)",
    "rate_plan": "Bed & Breakfast",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "hold_expires_at": "2026-10-03T02:15:00Z",
    "total_payment": {
      "currency": "IDR",
      "amount": 2500000
    },
    "payment_url": "https://payment.pulangkeuttara.com/pay/ch_98127391823"
  }
  ```

---

## 4. Skema Database yang Ditingkatkan

```sql
-- Rate Plans
CREATE TABLE IF NOT EXISTS rate_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    includes_breakfast BOOLEAN NOT NULL DEFAULT FALSE,
    cancellation_policy VARCHAR(50) NOT NULL DEFAULT 'FREE_CANCELLATION_UNTIL_H3',
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

-- Idempotency Records
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(100) PRIMARY KEY,
    payload_hash VARCHAR(64) NOT NULL,
    status_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Payment Attempts Ledger
CREATE TABLE IF NOT EXISTS payment_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_reference VARCHAR(100) UNIQUE,
    amount_minor BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status VARCHAR(50) NOT NULL DEFAULT 'initiated',
    webhook_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Penambahan Kolom pada Bookings
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS guest_token VARCHAR(64),
    ADD COLUMN IF NOT EXISTS phone_number VARCHAR(30),
    ADD COLUMN IF NOT EXISTS rate_plan_code VARCHAR(50),
    ADD COLUMN IF NOT EXISTS special_requests TEXT,
    ADD COLUMN IF NOT EXISTS estimated_arrival VARCHAR(10),
    ADD COLUMN IF NOT EXISTS terms_consented_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancellation_policy VARCHAR(50);
```
