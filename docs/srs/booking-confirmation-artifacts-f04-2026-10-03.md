# SRS — F04: Booking Confirmation Artifacts (Printable Invoice & iCalendar .ics)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Spesifikasi:
- **PRD Rujukan:** [PRD-F04-Artifacts](../prd/booking-confirmation-artifacts-f04-2026-10-03.md)
- **Tech Architecture:** [TECH-F04-Artifacts](../tech/booking-confirmation-artifacts-f04-architecture-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F04](../walkthrough/booking-confirmation-artifacts-f04-walkthrough-2026-10-03.md)

---

## 1. Functional Requirements (FR)

- **FR-01 (Generate Printable Invoice DTO):** Sistem wajib menyediakan DTO terperinci yang memuat identitas hotel, nomor invoice terformat, periode menginap, profil tamu, item kamar dan paket makanan, breakdown harga dan pajak daerah PB1 10%, serta status pembayaran.
- **FR-02 (Generate iCalendar Stream):** Sistem wajib menghasilkan dokumen kalender RFC 5545 valid dengan format `text/calendar; charset=utf-8`, UID unik stabil, timezone `Asia/Jakarta`, serta pengingat `VALARM` 24 jam sebelum check-in.
- **FR-03 (Payment Status Guard):** Sistem wajib menolak pembuatan invoice dan kalender untuk booking yang belum lunas (`status == "pending"` atau `failed` / `cancelled`) dengan kode status HTTP 400 `RECEIPT_NOT_AVAILABLE`.
- **FR-04 (Cryptographic Session & Anti-IDOR Defense):** Sistem wajib memvalidasi sesi tamu aktif (`X-Guest-Session`) dan mencocokkan email pemilik. Jika booking ID tidak cocok dengan email sesi, kembalikan HTTP 404 `BOOKING_NOT_FOUND`.
- **FR-05 (HTTP Response Headers):** Respons artefak wajib menyertakan `Cache-Control: no-store, private` dan `Content-Disposition: attachment; filename="pulang-booking-<id>.ics"` (khusus kalender).

---

## 2. Spesifikasi Endpoint HTTP RESTful

### 2.1 GET /api/v1/guest/bookings/{id}/receipt

* **Otentikasi:** Wajib header `X-Guest-Session`
* **Response Sukses (HTTP 200 OK):**
```json
{
  "invoice_number": "INV/PKU/202610/PKU-20261003-8F2A",
  "invoice_date": "2026-10-03T10:30:00Z",
  "booking_id": "01924b12-3456-789a-bcde-f0123456789a",
  "status": "PAID",
  "hotel_info": {
    "name": "Pulang ke Uttara",
    "tagline": "Urban Boutique Hotel & Residence",
    "address": "Jl. Kaliurang Km 5.6 No. 1, Caturtunggal, Depok, Sleman, D.I. Yogyakarta 55281",
    "phone": "+62 274 5022888",
    "email": "stay@pulangkeuttara.id",
    "website": "https://pulangkeuttara.id"
  },
  "stay_details": {
    "check_in_date": "2026-10-10",
    "check_in_time": "14:00 WIB",
    "check_out_date": "2026-10-12",
    "check_out_time": "12:00 WIB",
    "total_nights": 2,
    "timezone": "Asia/Jakarta"
  },
  "guest_details": {
    "name": "Rian Ardianto",
    "email": "rian@example.com",
    "phone": "+6281234567890",
    "num_rooms": 1,
    "num_guests": 2,
    "special_requests": "Lantai tinggi, non-smoking"
  },
  "room_item": {
    "room_type_id": "superior-room",
    "room_type_name": "Superior Room",
    "rate_plan_code": "BB",
    "meal_plan": "Sarapan Termasuk (Breakfast Included)",
    "num_rooms": 1,
    "total_nights": 2,
    "nightly_rate_minor": 85000000,
    "subtotal_minor": 170000000
  },
  "pricing_breakdown": {
    "currency": "IDR",
    "room_subtotal_minor": 170000000,
    "breakfast_charge_minor": 0,
    "discount_minor": 25500000,
    "tax_minor": 14450000,
    "total_price_minor": 158950000
  },
  "payment_summary": {
    "status": "PAID",
    "provider": "Xendit",
    "provider_reference": "inv_12345",
    "paid_at": "2026-10-03T10:30:00Z"
  },
  "policies": {
    "check_in_policy": "Wajib menunjukkan kartu identitas resmi (KTP/Paspor) saat check-in. Waktu check-in mulai 14:00 WIB.",
    "cancellation_policy": "Kebijakan pembatalan fleksibel: dapat dibatalkan sebelum H-1 14:00 WIB."
  },
  "qr_payload": "https://pulangkeuttara.id/verify/booking/01924b12-3456-789a-bcde-f0123456789a"
}
```

### 2.2 GET /api/v1/guest/bookings/{id}/calendar.ics

* **Otentikasi:** Wajib header `X-Guest-Session`
* **Response Sukses (HTTP 200 OK):**
  * `Content-Type: text/calendar; charset=utf-8`
  * `Content-Disposition: attachment; filename="pulang-booking-<id>.ics"`
  * `Cache-Control: no-store, private`
* **Payload Body (RFC 5545):**
```text
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Pulang ke Uttara//Hotel Booking Engine v1.0//ID
CALSCALE:GREGORIAN
METHOD:PUBLISH
BEGIN:VTIMEZONE
TZID:Asia/Jakarta
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0700
TZOFFSETTO:+0700
TZNAME:WIB
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:booking-01924b12-3456-789a-bcde-f0123456789a@pulangkeuttara.id
DTSTAMP:20261003T103000Z
DTSTART;TZID=Asia/Jakarta:20261010T140000
DTEND;TZID=Asia/Jakarta:20261012T120000
SUMMARY:Menginap di Pulang ke Uttara (Superior Room)
DESCRIPTION:Kode Reservasi: PKU-20261003-8F2A\nTamu: Rian Ardianto\nKamar: Superior Room (1 kamar)\nCheck-in: 2026-10-10 14:00 WIB\nCheck-out: 2026-10-12 12:00 WIB\nAlamat: Jl. Kaliurang Km 5.6 No. 1, Sleman, Yogyakarta\nTelepon: +62 274 5022888
LOCATION:Pulang ke Uttara, Jl. Kaliurang Km 5.6 No. 1, Caturtunggal, Depok, Sleman, D.I. Yogyakarta 55281
STATUS:CONFIRMED
BEGIN:VALARM
ACTION:DISPLAY
DESCRIPTION:Pengingat Check-in: Besok jadwal check-in di Pulang ke Uttara (14:00 WIB)
TRIGGER:-P1D
END:VALARM
END:VEVENT
END:VCALENDAR
```

---

## 3. Penanganan Error (Error Codes & Responses)

| Skenario | HTTP Status | Kode Error | Deskripsi |
|---|---|---|---|
| Sesi hilang / kadaluarsa | 401 Unauthorized | `UNAUTHORIZED` | Header `X-Guest-Session` tidak valid atau sesi telah berakhir |
| Booking ID milik tamu lain | 404 Not Found | `BOOKING_NOT_FOUND` | Data booking tidak ditemukan untuk tamu ini (IDOR defense) |
| Booking belum dibayar | 400 Bad Request | `RECEIPT_NOT_AVAILABLE` | Invoice resmi dan kalender hanya tersedia setelah pembayaran lunas |
| Internal database failure | 500 Internal Error | `INTERNAL_SERVER_ERROR` | Terjadi kendala saat memproses kueri database |
