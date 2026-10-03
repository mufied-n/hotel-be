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
	CreatedAt            time.Time      `json:"created_at"`
	AllowedActions       AllowedActions `json:"allowed_actions"`
}
