# Software Requirements Specification (SRS) — Public DTO Privacy & Token Protection (BE-R02)

**Fitur:** Public DTO Privacy, Sensitive Free-Text & Token Protection  
**ID Gap:** BE-R02 (P1)  
**Terkait:** BE-G13, F02/F03  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  

---

## 1. Functional Requirements

### FR-01: Skema PublicDTO Minimal
Sistem wajib mendefinisikan `PublicDTO` di paket `internal/booking` hanya dengan field non-sensitif berikut:
```json
{
  "id": "string",
  "room_type_id": "string",
  "check_in": "string (RFC3339)",
  "check_out": "string (RFC3339)",
  "num_rooms": 1,
  "status": "confirmed",
  "expires_at": "string (RFC3339, nullable)",
  "created_at": "string (RFC3339)"
}
```
Field `special_requests`, `estimated_arrival_time`, `guest_name`, `guest_email`, `guest_phone`, `guest_token`, dan seluruh rincian harga **dihapus sepenuhnya** dari `PublicDTO`.

### FR-02: Sanitasi Token Akses Tamu pada Pembacaan Booking
Pada endpoint `GET /api/v1/bookings/:id`:
1. Jika caller adalah Staf (`authCtx.Role != "guest"`):
   - Objek `Booking` dikembalikan dengan seluruh detail tamu dan operasional (`special_requests`, `estimated_arrival_time`).
   - Field `GuestToken` **wajib di-nolkan (`b.GuestToken = ""`)** sebelum diserialisasi ke JSON.
2. Jika caller adalah Tamu dengan `X-Guest-Token` yang valid:
   - Objek `Booking` dikembalikan dengan seluruh detail milik tamu.
   - Field `GuestToken` **wajib di-nolkan (`b.GuestToken = ""`)** sehingga `omitempty` menghilangkan field `guest_token` dari payload response.
3. Jika caller adalah Guest tanpa token atau dengan token salah/milik booking lain:
   - Handler wajib mengembalikan `PublicDTO` melalui `b.ToPublicDTO()`.

---

## 2. Kontrak HTTP & Respon

### Endpoint: `GET /api/v1/bookings/{id}`

#### Skenario A: Tamu Tanpa Token / Token Salah (Public Access)
- **Header:** `X-Guest-Token: [kosong atau invalid]`
- **Status:** `200 OK`
- **Body:**
```json
{
  "id": "01923456-789a-7b00-8c00-000000000001",
  "room_type_id": "deluxe",
  "check_in": "2026-11-01T00:00:00Z",
  "check_out": "2026-11-03T00:00:00Z",
  "num_rooms": 1,
  "status": "confirmed",
  "created_at": "2026-10-03T10:00:00Z"
}
```
*Catatan: Tidak ada field `special_requests`, `estimated_arrival_time`, atau `guest_token` dalam payload.*

#### Skenario B: Tamu Terautentikasi (Owner)
- **Header:** `X-Guest-Token: gst_valid_secret`
- **Status:** `200 OK`
- **Body:**
```json
{
  "id": "01923456-789a-7b00-8c00-000000000001",
  "room_type_id": "deluxe",
  "check_in": "2026-11-01T00:00:00Z",
  "check_out": "2026-11-03T00:00:00Z",
  "num_rooms": 1,
  "num_guests": 2,
  "status": "confirmed",
  "rate_plan_code": "RO",
  "cancellation_policy": "FLEXIBLE",
  "cancellation_description": "Pembatalan gratis hingga H-3 14:00 WIB",
  "room_subtotal_minor": 150000000,
  "total_price_minor": 150000000,
  "currency": "IDR",
  "guest_name": "Budi Santoso",
  "guest_email": "budi@example.com",
  "guest_phone": "+628123456789",
  "estimated_arrival_time": "14:00",
  "special_requests": "Alergi debu berat, mohon pembersihan ekstra dan bantal bebas bulu",
  "terms_accepted": true,
  "created_at": "2026-10-03T10:00:00Z"
}
```
*Catatan: `guest_token` tidak muncul dalam response JSON.*

#### Skenario C: Staf Hotel (Receptionist / Front Desk)
- **Header:** `Authorization: Bearer <valid_staff_token>` (atau session cookie staf)
- **Status:** `200 OK`
- **Body:** Sama dengan Skenario B, tanpa field `guest_token`.
