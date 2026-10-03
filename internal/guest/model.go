package guest

import (
	"time"
)

// GuestSession mewakili sesi tamu yang terautentikasi (OWASP ASVS V3).
type GuestSession struct {
	ID           string    `json:"id"`
	GuestEmail   string    `json:"guest_email"`
	TokenHash    string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// Challenge mewakili kode OTP tantangan sementara (OWASP ASVS V2).
type Challenge struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	CodeHash    string     `json:"-"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	ExpiresAt   time.Time  `json:"expires_at"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ProfileView adalah informasi profil tamu aktif yang dikembalikan ke klien.
type ProfileView struct {
	Email               string    `json:"email"`
	ActiveBookingsCount int       `json:"active_bookings_count"`
	LastActiveAt        time.Time `json:"last_active_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// BookingSummary mewakili ringkasan pesanan pada daftar "Booking Saya".
type BookingSummary struct {
	ID               string    `json:"id"`
	RoomTypeID       string    `json:"room_type_id"`
	RoomTypeName     string    `json:"room_type_name"`
	CheckIn          string    `json:"check_in"`
	CheckOut         string    `json:"check_out"`
	NumRooms         int       `json:"num_rooms"`
	NumGuests        int       `json:"num_guests"`
	Status           string    `json:"status"`
	TotalPriceMinor  int64     `json:"total_price_minor"`
	Currency         string    `json:"currency"`
	CreatedAt        time.Time `json:"created_at"`
}

// AllowedActions merangkum aksi yang dapat diambil oleh tamu berdasarkan status pemesanan.
type AllowedActions struct {
	CanPay               bool `json:"can_pay"`
	CanCancel            bool `json:"can_cancel"`
	CanDownloadReceipt   bool `json:"can_download_receipt"`
	CanRequestAssistance bool `json:"can_request_assistance"`
}

// BookingDetail mewakili detail lengkap pemesanan milik tamu terverifikasi (UU PDP No. 27/2022).
type BookingDetail struct {
	ID                   string         `json:"id"`
	RoomTypeID           string         `json:"room_type_id"`
	RoomTypeName         string         `json:"room_type_name"`
	CheckIn              string         `json:"check_in"`
	CheckOut             string         `json:"check_out"`
	NumRooms             int            `json:"num_rooms"`
	NumGuests            int            `json:"num_guests"`
	Status               string         `json:"status"`
	TotalPriceMinor      int64          `json:"total_price_minor"`
	Currency             string         `json:"currency"`
	GuestName            string         `json:"guest_name"`
	GuestEmail           string         `json:"guest_email"`
	GuestPhone           string         `json:"guest_phone"`
	EstimatedArrivalTime string         `json:"estimated_arrival_time,omitempty"`
	SpecialRequests      string         `json:"special_requests,omitempty"`
	CancellationPolicy   string         `json:"cancellation_policy,omitempty"`
	RatePlanCode         string         `json:"rate_plan_code,omitempty"`
	ExpiresAt            *time.Time     `json:"expires_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	PaymentURL           string         `json:"payment_url,omitempty"`
	AllowedActions       AllowedActions `json:"allowed_actions"`
}

// HotelInfo berisi identitas dan kontak properti Pulang ke Uttara.
type HotelInfo struct {
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website"`
}

// StayDetails merinci periode waktu dan zona waktu menginap.
type StayDetails struct {
	CheckInDate  string `json:"check_in_date"`
	CheckInTime  string `json:"check_in_time"`
	CheckOutDate string `json:"check_out_date"`
	CheckOutTime string `json:"check_out_time"`
	TotalNights  int    `json:"total_nights"`
	Timezone     string `json:"timezone"`
}

// GuestDetails merinci profil tamu pemesan resmi.
type GuestDetails struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	NumRooms        int    `json:"num_rooms"`
	NumGuests       int    `json:"num_guests"`
	SpecialRequests string `json:"special_requests,omitempty"`
}

// RoomItemReceipt merinci varian kamar dan paket makan yang dipesan.
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

// PricingBreakdown merinci komponen harga, diskon, dan pajak daerah PB1.
type PricingBreakdown struct {
	Currency             string `json:"currency"`
	RoomSubtotalMinor    int64  `json:"room_subtotal_minor"`
	BreakfastChargeMinor int64  `json:"breakfast_charge_minor"`
	DiscountMinor        int64  `json:"discount_minor"`
	TaxMinor             int64  `json:"tax_minor"`
	TotalPriceMinor      int64  `json:"total_price_minor"`
}

// PaymentSummary merinci bukti pelunasan transaksi gateway.
type PaymentSummary struct {
	Status            string `json:"status"`
	Provider          string `json:"provider"`
	ProviderReference string `json:"provider_reference,omitempty"`
	PaidAt            string `json:"paid_at,omitempty"`
}

// PoliciesReceipt memuat aturan check-in dan pembatalan resmi.
type PoliciesReceipt struct {
	CheckInPolicy      string `json:"check_in_policy"`
	CancellationPolicy string `json:"cancellation_policy"`
}

// ReceiptDTO adalah representasi faktur resmi dan konfirmasi reservasi (Printable Invoice).
type ReceiptDTO struct {
	InvoiceNumber    string           `json:"invoice_number"`
	InvoiceDate      string           `json:"invoice_date"`
	BookingID        string           `json:"booking_id"`
	BookingReference string           `json:"booking_reference"`
	Status           string           `json:"status"`
	HotelInfo        HotelInfo        `json:"hotel_info"`
	StayDetails      StayDetails      `json:"stay_details"`
	GuestDetails     GuestDetails     `json:"guest_details"`
	RoomItem         RoomItemReceipt  `json:"room_item"`
	PricingBreakdown PricingBreakdown `json:"pricing_breakdown"`
	PaymentSummary   PaymentSummary   `json:"payment_summary"`
	Policies         PoliciesReceipt  `json:"policies"`
	QRPayload        string           `json:"qr_payload"`
}

