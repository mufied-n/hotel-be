package frontdesk

import "time"

// ShiftType enum regu kerja meja depan hotel.
type ShiftType string

const (
	ShiftMorning   ShiftType = "morning"
	ShiftAfternoon ShiftType = "afternoon"
	ShiftNight     ShiftType = "night"
)

// RosterMetrics merangkum metrik okupansi dan status fisik 95 kamar hotel Pulang ke Uttara.
type RosterMetrics struct {
	TotalRooms           int     `json:"total_rooms"`
	SellableRooms        int     `json:"sellable_rooms"`
	OutOfOrderRooms      int     `json:"out_of_order_rooms"`
	OccupiedRooms        int     `json:"occupied_rooms"`
	VacantInspectedRooms int     `json:"vacant_inspected_rooms"`
	VacantDirtyRooms     int     `json:"vacant_dirty_rooms"`
	CleaningRooms        int     `json:"cleaning_rooms"`
	OccupancyRatePercent float64 `json:"occupancy_rate_percent"`
}

// ExpectedArrivalItem merepresentasikan tamu yang dijadwalkan tiba pada tanggal operasional.
type ExpectedArrivalItem struct {
	BookingID            string   `json:"booking_id"`
	GuestName            string   `json:"guest_name"`
	GuestPhone           string   `json:"guest_phone,omitempty"`
	RoomTypeID           string   `json:"room_type_id"`
	RoomTypeName         string   `json:"room_type_name"`
	AssignedRooms        []string `json:"assigned_rooms"`
	NumRooms             int      `json:"num_rooms"`
	NumGuests            int      `json:"num_guests"`
	EstimatedArrivalTime string   `json:"estimated_arrival_time,omitempty"`
	SpecialRequests      string   `json:"special_requests,omitempty"`
	TotalPriceMinor      int64    `json:"total_price_minor"`
}

// ExpectedDepartureItem merepresentasikan tamu yang dijadwalkan keluar pada tanggal operasional.
type ExpectedDepartureItem struct {
	BookingID    string   `json:"booking_id"`
	GuestName    string   `json:"guest_name"`
	RoomNumbers  []string `json:"room_numbers"`
	CheckInDate  string   `json:"check_in_date"`
	CheckOutDate string   `json:"check_out_date"`
}

// DailyRoster agregasi lengkap dashboard harian meja depan.
type DailyRoster struct {
	Date               string                  `json:"date"`
	Metrics            RosterMetrics           `json:"metrics"`
	ExpectedArrivals   []ExpectedArrivalItem   `json:"expected_arrivals"`
	ExpectedDepartures []ExpectedDepartureItem `json:"expected_departures"`
	InHouseCount       int                     `json:"in_house_count"`
}

// HandoverNote entri catatan serah terima shift meja depan.
type HandoverNote struct {
	ID             string    `json:"id"`
	Shift          ShiftType `json:"shift"`
	CashFloatMinor int64     `json:"cash_float_minor"`
	PendingIssues  string    `json:"pending_issues"`
	VIPGuestNotes  string    `json:"vip_guest_notes"`
	ActorID        string    `json:"actor_id"`
	ActorRole      string    `json:"actor_role"`
	CreatedAt      time.Time `json:"created_at"`
}

// RecordHandoverInput DTO untuk membuat catatan serah terima shift baru.
type RecordHandoverInput struct {
	Shift          ShiftType `json:"shift"`
	CashFloatMinor int64     `json:"cash_float_minor"`
	PendingIssues  string    `json:"pending_issues"`
	VIPGuestNotes  string    `json:"vip_guest_notes"`
	ActorID        string    `json:"actor_id"`
	ActorRole      string    `json:"actor_role"`
}
