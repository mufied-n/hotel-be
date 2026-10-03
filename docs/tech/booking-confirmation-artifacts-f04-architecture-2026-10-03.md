# TECH Architecture — F04: Booking Confirmation Artifacts (Printable Invoice & iCalendar .ics)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Spesifikasi:
- **PRD Rujukan:** [PRD-F04-Artifacts](../prd/booking-confirmation-artifacts-f04-2026-10-03.md)
- **SRS Rujukan:** [SRS-F04-Artifacts](../srs/booking-confirmation-artifacts-f04-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F04](../walkthrough/booking-confirmation-artifacts-f04-walkthrough-2026-10-03.md)

---

## 1. Arsitektur Komponen (Hexagonal Architecture)

Modul ini diintegrasikan ke dalam domain `guest` dan transport layer `api` tanpa membuat service baru (mematuhi prinsip **Ponytail** dan arsitektur *Modular Monolith* eksisting):

```mermaid
flowchart TD
    Client["Klien Webapp (Nuxt 4 / Tamu)"] -->|GET /api/v1/guest/bookings/{id}/receipt| API["api.handleGuestBookingReceipt"]
    Client -->|GET /api/v1/guest/bookings/{id}/calendar.ics| API_CAL["api.handleGuestBookingCalendar"]
    
    API --> Middleware["api.requireGuestSession"]
    API_CAL --> Middleware
    
    Middleware --> Domain["guest.Service (GetBookingReceipt & GenerateCalendarICS)"]
    Domain --> Store["guest.Store (PostgresStore.GetBookingReceiptData)"]
    Domain --> ICS["Pure Go iCalendar RFC 5545 Builder"]
    Store --> DB[("PostgreSQL (bookings, room_types, payment_attempts)")]
```

---

## 2. Struktur Data & Model Domain (`internal/guest`)

```go
// ReceiptDTO merangkum data faktur resmi dan konfirmasi booking hotel.
type ReceiptDTO struct {
	InvoiceNumber      string           `json:"invoice_number"`
	InvoiceDate        string           `json:"invoice_date"`
	BookingID          string           `json:"booking_id"`
	Status             string           `json:"status"`
	HotelInfo          HotelInfo        `json:"hotel_info"`
	StayDetails        StayDetails      `json:"stay_details"`
	GuestDetails       GuestDetails     `json:"guest_details"`
	RoomItem           RoomItemReceipt  `json:"room_item"`
	PricingBreakdown   PricingBreakdown `json:"pricing_breakdown"`
	PaymentSummary     PaymentSummary   `json:"payment_summary"`
	Policies           PoliciesReceipt  `json:"policies"`
	QRPayload          string           `json:"qr_payload"`
}

type HotelInfo struct {
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website"`
}

type StayDetails struct {
	CheckInDate  string `json:"check_in_date"`
	CheckInTime  string `json:"check_in_time"`
	CheckOutDate string `json:"check_out_date"`
	CheckOutTime string `json:"check_out_time"`
	TotalNights  int    `json:"total_nights"`
	Timezone     string `json:"timezone"`
}

type GuestDetails struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	NumRooms        int    `json:"num_rooms"`
	NumGuests       int    `json:"num_guests"`
	SpecialRequests string `json:"special_requests,omitempty"`
}

type RoomItemReceipt struct {
	RoomTypeID       string `json:"room_type_id"`
	RoomTypeName     string `json:"room_type_name"`
	RatePlanCode     string `json:"rate_plan_code"`
	MealPlan         string `json:"meal_plan"`
	NumRooms         int    `json:"num_rooms"`
	TotalNights      int    `json:"total_nights"`
	NightlyRateMinor int64  `json:"nightly_rate_minor"`
	SubtotalMinor    int64  `json:"subtotal_minor"`
}

type PricingBreakdown struct {
	Currency             string `json:"currency"`
	RoomSubtotalMinor    int64  `json:"room_subtotal_minor"`
	BreakfastChargeMinor int64  `json:"breakfast_charge_minor"`
	DiscountMinor        int64  `json:"discount_minor"`
	TaxMinor             int64  `json:"tax_minor"`
	TotalPriceMinor      int64  `json:"total_price_minor"`
}

type PaymentSummary struct {
	Status            string `json:"status"`
	Provider          string `json:"provider"`
	ProviderReference string `json:"provider_reference,omitempty"`
	PaidAt            string `json:"paid_at,omitempty"`
}

type PoliciesReceipt struct {
	CheckInPolicy      string `json:"check_in_policy"`
	CancellationPolicy string `json:"cancellation_policy"`
}
```

---

## 3. Desain RFC 5545 iCalendar (Zero Dependency / Ponytail Compliant)

Generator iCalendar diimplementasikan menggunakan Go standard library murni (`strings.Builder`) dengan efisiensi memori tinggi:
1. **CRLF Line Endings (`\r\n`):** Memastikan kompatibilitas mutlak dengan parser Google Calendar, iOS Calendar, dan MS Outlook.
2. **Text Escaping:** Karakter spesial seperti `,`, `;`, `\`, dan baris baru `\n` di-escape menjadi `\,`, `\;`, `\\`, dan `\n`.
3. **Timezone:** Didefinisikan secara eksplisit sebagai `TZID=Asia/Jakarta` dengan definisi `VTIMEZONE` standar (WIB, UTC+7).
4. **Alarm Notification (`VALARM`):** Memicu notifikasi lokal pada perangkat tamu 1 hari (`-P1D`) sebelum waktu check-in (14:00 WIB).

---

## 4. Keamanan & Kepatuhan Regulasi

1. **Anti-IDOR Defense:** Kueri database untuk data receipt menggunakan kombinasi ganda:
   ```sql
   WHERE b.id = $1 AND b.guest_email = $2
   ```
   Bila booking ID tersebut bukan milik tamu yang terotentikasi, kueri menghasilkan nol baris dan handler merespons `404 Not Found` (mencegah penyerang menduga ID reservasi orang lain).
2. **Proteksi Cache:** Header HTTP `Cache-Control: no-store, private` mencegah proxy cache atau shared gateway menyimpan data finansial tamu.
3. **UU PDP No. 27/2022:** Data nomor telepon dan email hanya ditampilkan secara unmasked pada sesi tamu pemilik yang sah.
