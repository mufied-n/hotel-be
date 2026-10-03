# Software Requirements Specification (SRS): Penyimpanan Kuotasi Harga Terdistribusi (Durable Quote Store) (BE-R11)

**Nomor Dokumen:** SRS-PULANG-BE-R11  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait Audit & PRD:** `BE-R11`, `PRD-PULANG-BE-R11`

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Penyimpanan Kuotasi Terkunci ke Valkey/Redis
- Sistem wajib menyediakan implementasi adapter `ValkeyQuoteStore` yang mengimplementasikan antarmuka `rates.QuoteStore`.
- Format kunci Valkey: `hotel:quote:{quote_id}`.
- Payload yang disimpan adalah representasi JSON lengkap dari domain struct `rates.LockedQuote`.
- Masa simpan kunci (*expiration / TTL*) di Valkey disetel secara otomatis berdasarkan `time.Until(LockedQuote.ExpiresAt)`. Jika tidak ditentukan atau kurang dari nol, default adalah 15 menit (900 detik).

### FR-02: Pengambilan & Verifikasi Kuotasi Lintas Instansi
- Fungsi `GetQuote(ctx context.Context, id string) (LockedQuote, error)` wajib mengambil data dari Valkey melalui perintah `GET hotel:quote:{id}`.
- Jika kunci tidak ditemukan di Valkey (`redis.Nil`), sistem wajib mengembalikan `rates.ErrQuoteNotFound`.
- Jika waktu sistem lokal `time.Now()` telah melampaui `ExpiresAt` pada data kuotasi yang diambil, sistem wajib mengembalikan `rates.ErrQuoteExpired`.
- Jika data JSON valid, sistem mengembalikan struct `LockedQuote`.

### FR-03: Propagasi Kegagalan Persistensi (Fail-Closed)
- Metode `rates.Engine.CalculateLockedQuote` wajib memeriksa hasil pengembalian `e.quoteStore.SaveQuote(ctx, lq)`.
- Jika penyimpanan ke Valkey menghasilkan error (misal: koneksi terputus, timeout, memory limit exceeded):
  - Sistem **TIDAK BOLEH** mengabaikan error tersebut.
  - Sistem wajib mengembalikan error terbungkus `rates.ErrSaveQuoteFailed`.
- Handler HTTP `calculateQuote` pada transport API wajib mengenali error ini dan merespons klien dengan status HTTP 500:
  ```json
  {
    "error": "gagal menyimpan kuotasi harga",
    "title": "Internal Server Error",
    "status": 500,
    "detail": "gagal menyimpan kuotasi harga",
    "code": "INTERNAL_ERROR"
  }
  ```

### FR-04: Kompatibilitas Transaksi Pemesanan Multi-Replika
- Pada alur `POST /api/v1/bookings`, `booking.Service.Create` memanggil `s.quoteStore.GetQuote(ctx, in.QuoteID)`.
- Karena seluruh replika server membaca ke Valkey cluster yang sama, pemesanan dapat diproses secara sukses oleh replika manapun terlepas dari replika mana yang pertama kali menerbitkan kuotasi.

---

## 2. Format Kunci & Skema Data Valkey / Redis

### 2.1 Spesifikasi Kunci
- **Pola Kunci:** `hotel:quote:{quote_id}`
- **Tipe Data:** `string` (JSON UTF-8)
- **TTL:** 900 detik (15 menit)

### 2.2 Format JSON Payload
```json
{
  "id": "01923e1f-7b56-7889-bcde-123456789abc",
  "created_at": "2026-10-03T23:05:00Z",
  "expires_at": "2026-10-03T23:20:00Z",
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "rate_plan_code": "bed_and_breakfast",
  "rate_plan_name": "Bed & Breakfast",
  "cancellation_code": "flexible_48h",
  "cancellation_desc": "Pembatalan bebas biaya hingga 48 jam sebelum pukul 14:00 WIB tanggal check-in.",
  "check_in": "2026-10-10T00:00:00Z",
  "check_out": "2026-10-12T00:00:00Z",
  "num_rooms": 1,
  "num_guests": 2,
  "adults": 2,
  "children": 0,
  "nightly_rates": [
    {
      "date": "2026-10-10T00:00:00Z",
      "base_minor": 550000,
      "multiplier": 1.25,
      "rate_minor": 687500,
      "is_weekend": true
    },
    {
      "date": "2026-10-11T00:00:00Z",
      "base_minor": 550000,
      "multiplier": 1.0,
      "rate_minor": 550000,
      "is_weekend": false
    }
  ],
  "pricing": {
    "room_subtotal_minor": 1237500,
    "breakfast_charge_minor": 400000,
    "discount_minor": 0,
    "tax_minor": 163750,
    "total_price_minor": 1801250,
    "currency": "IDR"
  }
}
```

---

## 3. Spesifikasi Kontrak Error Kode

| Skenario | HTTP Status | Code | Detail Pesan |
| :--- | :---: | :--- | :--- |
| Valkey simpan gagal saat POST /quotes | 500 | `INTERNAL_ERROR` | `gagal menyimpan kuotasi harga` |
| Quote ID tidak ditemukan saat booking | 400 | `QUOTE_EXPIRED` | `kuotasi harga telah kedaluwarsa atau tidak valid, silakan muat ulang halaman` |
| Quote ID telah melewati waktu 15 menit | 400 | `QUOTE_EXPIRED` | `kuotasi harga telah kedaluwarsa atau tidak valid, silakan muat ulang halaman` |
| Atribut booking tidak cocok dengan quote | 400 | `QUOTE_MISMATCH` | `parameter reservasi tidak sesuai dengan kuotasi terkunci` |
