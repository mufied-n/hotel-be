package stay

import "time"

// ReasonCategory enum alasan pemindahan kamar tamu.
type ReasonCategory string

const (
	ReasonMaintenanceDefect ReasonCategory = "maintenance_defect"
	ReasonNoiseComplaint    ReasonCategory = "noise_complaint"
	ReasonUpgrade           ReasonCategory = "upgrade"
	ReasonGuestRequest      ReasonCategory = "guest_request"
)

// IsValid memvalidasi apakah kategori alasan pemindahan kamar diizinkan.
func (r ReasonCategory) IsValid() bool {
	switch r {
	case ReasonMaintenanceDefect, ReasonNoiseComplaint, ReasonUpgrade, ReasonGuestRequest:
		return true
	default:
		return false
	}
}

// RoomMoveInput DTO masukan pemindahan kamar.
type RoomMoveInput struct {
	BookingID        string         `json:"booking_id"`
	TargetRoomNumber string         `json:"target_room_number"`
	ReasonCategory   ReasonCategory `json:"reason_category"`
	Notes            string         `json:"notes"`
	ActorID          string         `json:"actor_id"`
	ActorRole        string         `json:"actor_role"`
}

// RoomMoveResult DTO kembalian hasil pemindahan kamar fisik.
type RoomMoveResult struct {
	Status             string `json:"status"`
	BookingID          string `json:"booking_id"`
	PreviousRoomNumber string `json:"previous_room_number"`
	NewRoomNumber      string `json:"new_room_number"`
	MoveDate           string `json:"move_date"`
	Message            string `json:"message"`
}

// ExtendStayInput DTO masukan perpanjangan menginap.
type ExtendStayInput struct {
	BookingID        string `json:"booking_id"`
	AdditionalNights int    `json:"additional_nights"`
	PaymentMethod    string `json:"payment_method"`
	ActorID          string `json:"actor_id"`
}

// ExtendStayResult DTO kembalian hasil perpanjangan menginap.
type ExtendStayResult struct {
	Status                string `json:"status"`
	BookingID             string `json:"booking_id"`
	PreviousCheckOut      string `json:"previous_check_out"`
	NewCheckOut           string `json:"new_check_out"`
	AdditionalNights      int    `json:"additional_nights"`
	AdditionalAmountMinor int64  `json:"additional_amount_minor"`
	NewTotalPriceMinor    int64  `json:"new_total_price_minor"`
	PaymentStatus         string `json:"payment_status,omitempty"`
}

// RoomMoveLog entri riwayat perpindahan kamar.
type RoomMoveLog struct {
	ID             string    `json:"id"`
	BookingID      string    `json:"booking_id"`
	FromRoomNumber string    `json:"from_room_number"`
	ToRoomNumber   string    `json:"to_room_number"`
	MoveDate       string    `json:"move_date"`
	ReasonCategory string    `json:"reason_category"`
	Notes          string    `json:"notes"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// BookingDetails data ringkas reservasi untuk kalkulasi perpanjangan dan pemindahan.
type BookingDetails struct {
	ID              string
	Status          string
	RoomTypeID      string
	CheckIn         time.Time
	CheckOut        time.Time
	NumRooms        int
	TotalPriceMinor int64
	CurrentRoom     string
}
