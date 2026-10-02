# Software Requirements Specification (SRS) — Batch BE-C: Tarif, Paket, Quote Engine & Kebijakan Pembatalan
**Fitur:** Dynamic Rate Plans, Money Contract, 15-Minute Quote Lock Engine, & Cancellation Policy Enforcement  
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Dokumen ID:** `SRS-BATCH-C-2026-10-03`  
**Status:** Approved for Implementation  
**Target Rilis:** Q4 2026  
**Referensi Gap Audit:** `BE-G04`, `BE-G05`, `BE-G06`, `BE-G08`, `BE-G19`

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-C01: Struktur Tipe Data Moneter (`Money`)
1. Setiap besaran moneter wajib menggunakan tipe `Money` dengan integer murni:
   ```go
   type Money struct {
       Amount   int64  `json:"amount"`   // Satuan minor (untuk IDR = Rp 1)
       Currency string `json:"currency"` // Standar ISO 4217 ("IDR")
       Exponent int    `json:"exponent"` // 0 untuk IDR
   }
   ```
2. Tidak diizinkan melakukan operasi desimal floating-point (`float64`) pada perhitungan uang.

### FR-C02: Spesifikasi Paket & Rate Plan
1. Sistem mendukung 2 kode *rate plan* baku:
   - `room_only`: Tarif kamar murni tanpa sarapan. Kebijakan pembatalan default: `flexible_48h`.
   - `bed_and_breakfast`: Tarif kamar ditambah sarapan Rp 100.000 per orang dewasa per malam. Kebijakan pembatalan default: `flexible_48h`.
2. Opsi non-refundable (`non_refundable`) dapat disematkan pada paket promo diskon khusus.

### FR-C03: Mesin Kode Promo (Promo Code Engine)
1. Sistem memvalidasi kode promo pada saat pembuatan penawaran (quote).
2. Kode promo `OCTOBREAK`:
   - Diskon: 15% dari subtotal tarif dasar kamar (dihitung sebagai integer: `(subtotal * 15) / 100`).
   - Kebijakan: `non_refundable` (tarif promo hemat tidak dapat dibatalkan).
3. Kode promo yang tidak dikenal atau kedaluwarsa menghasilkan error `INVALID_PROMO_CODE` (HTTP 400).

### FR-C04: Rincian Harga Transparan (Price Breakdown)
Kalkulasi rincian harga wajib menghasilkan komponen:
$$\text{room\_subtotal} = \sum (\text{nightly\_base\_rate}) \times \text{num\_rooms}$$
$$\text{breakfast\_charge} = 100.000 \times \text{num\_adults} \times \text{nights} \times \text{num\_rooms} \quad (\text{hanya jika BB})$$
$$\text{discount} = \begin{cases} (\text{room\_subtotal} \times 15) / 100, & \text{jika promo OCTOBREAK} \\ 0, & \text{lainnya} \end{cases}$$
$$\text{taxable\_amount} = \text{room\_subtotal} + \text{breakfast\_charge} - \text{discount}$$
$$\text{tax\_pb1} = (\text{taxable\_amount} \times 10) / 100 \quad (10\% \text{ PB1 Yogyakarta})$$
$$\text{total\_price} = \text{taxable\_amount} + \text{tax\_pb1}$$

### FR-C05: Endpoint Pembuatan Penawaran Terkunci (`POST /api/v1/quotes`)
1. **Request Body:**
   ```json
   {
     "room_type_id": "01900000-0000-7000-8000-000000000001",
     "rate_plan_code": "bed_and_breakfast",
     "check_in": "2026-10-10",
     "check_out": "2026-10-12",
     "num_rooms": 1,
     "num_guests": 2,
     "promo_code": "OCTOBREAK"
   }
   ```
2. **Response Body (HTTP 200 OK):**
   ```json
   {
     "quote_id": "01900000-xxxx-7xxx-8xxx-xxxxxxxxxxxx",
     "created_at": "2026-10-03T02:30:00+07:00",
     "expires_at": "2026-10-03T02:45:00+07:00",
     "room_type_id": "01900000-0000-7000-8000-000000000001",
     "rate_plan_code": "bed_and_breakfast",
     "rate_plan_name": "Bed & Breakfast Package",
     "cancellation_policy": "non_refundable",
     "cancellation_description": "Tarif promo tidak dapat dibatalkan atau di-refund.",
     "currency": "IDR",
     "pricing": {
       "room_subtotal_minor": 1100000,
       "breakfast_charge_minor": 400000,
       "discount_minor": 165000,
       "tax_minor": 133500,
       "total_price_minor": 1468500
     }
   }
   ```

### FR-C06: Validasi Quote pada Pembuatan Reservasi (`POST /api/v1/bookings`)
1. Klien wajib mengirimkan `quote_id` yang valid pada payload pembuatan booking.
2. Jika `quote_id` tidak ditemukan atau waktu `now > expires_at`, server menolak dengan HTTP 410 `QUOTE_EXPIRED`.
3. Parameter reservasi (`room_type_id`, tanggal, jumlah kamar, jumlah tamu) dicocokkan dengan snapshot quote. Jika tidak cocok, tolak dengan HTTP 400 `QUOTE_MISMATCH`.
4. Total harga dan rincian harga dikunci mutlak dari snapshot quote yang terverifikasi.

### FR-C07: Penegakan Persetujuan Syarat (*Terms & Privacy Consent*)
1. Payload `POST /api/v1/bookings` wajib menyertakan:
   - `terms_accepted: true`
   - `privacy_accepted: true`
2. Jika salah satu bernilai `false`, server mengembalikan error HTTP 400 `CONSENT_REQUIRED`.
3. Waktu persetujuan `terms_accepted_at` dicatat dalam model database.

### FR-C08: Penegakan Kebijakan Pembatalan
1. Pada status `pending` (hold belum bayar), tamu diizinkan membatalkan / melepaskan hold kapan saja.
2. Pada status `confirmed` (sudah bayar):
   - Jika `cancellation_policy == "non_refundable"`, pembatalan oleh tamu publik **DITOLAK** dengan HTTP 409 `NON_REFUNDABLE_BOOKING`.
   - Jika `cancellation_policy == "flexible_48h"`, tamu diizinkan membatalkan tanpa penalti jika dilakukan sebelum batas 48 jam sebelum check-in (pukul 14:00 WIB). Jika melewati batas waktu, kembalikan HTTP 409 `CANCELLATION_DEADLINE_EXCEEDED`.
   - Staf berwenang (`receptionist` atau `gm_admin`) dapat melakukan pembatalan dengan alasan operasional resmi.

---

## 2. Definisi Kode Error RFC 7807 Problem Details

| Kode Error | HTTP Status | Penjelasan |
| :--- | :---: | :--- |
| `INVALID_RATE_PLAN` | 400 | Kode rate plan tidak dikenal atau tidak tersedia untuk kamar ini. |
| `INVALID_PROMO_CODE` | 400 | Kode promo tidak valid, salah ketik, atau masa berlakunya telah habis. |
| `QUOTE_REQUIRED` | 400 | Pembuatan booking wajib menyertakan `quote_id` yang valid. |
| `QUOTE_EXPIRED` | 410 | Penawaran harga telah kedaluwarsa (>15 menit). Klien harus membuat quote baru. |
| `QUOTE_MISMATCH` | 400 | Parameter booking tidak sesuai dengan data yang terkunci pada quote. |
| `CONSENT_REQUIRED` | 400 | Tamu belum menyetujui syarat & ketentuan dan kebijakan privasi hotel. |
| `NON_REFUNDABLE_BOOKING` | 409 | Reservasi bertarif non-refundable tidak dapat dibatalkan oleh tamu. |
| `CANCELLATION_DEADLINE_EXCEEDED`| 409 | Batas waktu pembatalan gratis (H-2 jam 14:00 WIB) telah terlewati. |
