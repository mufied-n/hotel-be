package housekeeping

import "time"

// CleanlinessStatus merepresentasikan status kebersihan dan kelayakan huni kamar fisik.
type CleanlinessStatus string

const (
	StatusVacantDirty  CleanlinessStatus = "vacant_dirty"
	StatusCleaning     CleanlinessStatus = "cleaning"
	StatusVacantClean  CleanlinessStatus = "vacant_clean"
	StatusInspected    CleanlinessStatus = "inspected"
	StatusOccupied     CleanlinessStatus = "occupied"
	StatusOutOfService CleanlinessStatus = "out_of_service"
	StatusOutOfOrder   CleanlinessStatus = "out_of_order"
)

// RoomOperationalView adalah representasi data kamar pada dashboard Housekeeping & Front Desk.
type RoomOperationalView struct {
	RoomNumber        string            `json:"room_number"`
	RoomTypeID        string            `json:"room_type_id"`
	RoomTypeName      string            `json:"room_type_name"`
	Floor             int               `json:"floor"`
	CleanlinessStatus CleanlinessStatus `json:"cleanliness_status"`
	MaintenanceNotes  string            `json:"maintenance_notes"`
	CurrentBookingID  *string           `json:"current_booking_id,omitempty"`
	GuestName         *string           `json:"guest_name,omitempty"`
	UpdatedAt         time.Time         `json:"updated_at"`
	UpdatedBy         string            `json:"updated_by"`
}

// RoomBoardSummary merangkum agregasi status seluruh kamar fisik hotel (95 kamar).
type RoomBoardSummary struct {
	TotalRooms int                           `json:"total_rooms"`
	Summary    map[CleanlinessStatus]int     `json:"summary"`
	Rooms      []RoomOperationalView         `json:"rooms"`
}

// UpdateStatusInput adalah DTO untuk pembaruan status kebersihan kamar.
type UpdateStatusInput struct {
	RoomNumber string            `json:"room_number"`
	ToStatus   CleanlinessStatus `json:"to_status"`
	Notes      string            `json:"notes"`
	ActorID    string            `json:"actor_id"`
	ActorRole  string            `json:"actor_role"`
}
