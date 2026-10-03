package assistance

import (
	"errors"
	"strings"
	"time"
)

// Category jenis kategori permintaan khusus tamu.
type Category string

const (
	CategoryEarlyArrival     Category = "early_arrival"
	CategoryLateDeparture    Category = "late_departure"
	CategoryHighFloor        Category = "high_floor"
	CategoryQuietRoom        Category = "quiet_room"
	CategoryBedType          Category = "bed_type"
	CategoryCelebrationSetup Category = "celebration_setup"
	CategoryBabyCrib         Category = "baby_crib"
	CategoryDietaryAllergy   Category = "dietary_allergy"
	CategoryOther            Category = "other"
)

// Department departemen penanggung jawab pemenuhan tugas.
type Department string

const (
	DepartmentFrontDesk   Department = "front_desk"
	DepartmentHousekeeping Department = "housekeeping"
)

// Status siklus hidup pemenuhan permintaan khusus.
type Status string

const (
	StatusPending      Status = "pending"
	StatusAcknowledged Status = "acknowledged"
	StatusFulfilled    Status = "fulfilled"
	StatusDeclined     Status = "declined"
)

var (
	ErrInvalidCategory         = errors.New("kategori permintaan khusus tidak valid")
	ErrEmptyDescription        = errors.New("deskripsi permintaan khusus tidak boleh kosong")
	ErrBookingNotFound         = errors.New("reservasi tidak ditemukan")
	ErrRequestNotFound         = errors.New("permintaan khusus tidak ditemukan")
	ErrInvalidStatusTransition = errors.New("transisi status pemenuhan tidak valid")
	ErrStaffNotesRequired      = errors.New("catatan staf wajib diisi saat menolak permintaan (minimal 5 karakter)")
)

// SpecialRequest entitas permintaan khusus tamu.
type SpecialRequest struct {
	ID          string     `json:"id"`
	BookingID   string     `json:"booking_id"`
	Category    Category   `json:"category"`
	Department  Department `json:"department"`
	Description string     `json:"description"`
	TargetTime  string     `json:"target_time,omitempty"`
	Status      Status     `json:"status"`
	StaffNotes  string     `json:"staff_notes"`
	HandledBy   string     `json:"handled_by,omitempty"`
	HandledAt   *time.Time `json:"handled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateRequestInput payload input untuk membuat permintaan khusus baru.
type CreateRequestInput struct {
	BookingID   string   `json:"booking_id"`
	Category    Category `json:"category"`
	Description string   `json:"description"`
	TargetTime  string   `json:"target_time"`
}

// UpdateStatusInput payload input bagi staf untuk memperbarui status.
type UpdateStatusInput struct {
	RequestID  string `json:"request_id"`
	ToStatus   Status `json:"to_status"`
	StaffNotes string `json:"staff_notes"`
	HandledBy  string `json:"handled_by"`
}

// StaffQueueItem item antrean tugas staf yang diperkaya data reservasi.
type StaffQueueItem struct {
	SpecialRequest
	GuestName    string   `json:"guest_name"`
	RoomNumbers  []string `json:"room_numbers"`
	CheckInDate  string   `json:"check_in_date"`
	CheckOutDate string   `json:"check_out_date"`
}

// ListFilter filter kueri antrean staf.
type ListFilter struct {
	Department string
	Status     string
	BookingID  string
}

// ResolveDepartment memetakan kategori permintaan ke departemen yang bertanggung jawab secara deterministik.
func ResolveDepartment(cat Category) Department {
	switch cat {
	case CategoryCelebrationSetup, CategoryBabyCrib, CategoryQuietRoom, CategoryHighFloor, CategoryBedType:
		return DepartmentHousekeeping
	case CategoryEarlyArrival, CategoryLateDeparture, CategoryDietaryAllergy, CategoryOther:
		return DepartmentFrontDesk
	default:
		return DepartmentFrontDesk
	}
}

// IsValidCategory memeriksa keabsahan kategori.
func IsValidCategory(cat Category) bool {
	switch cat {
	case CategoryEarlyArrival, CategoryLateDeparture, CategoryHighFloor, CategoryQuietRoom,
		CategoryBedType, CategoryCelebrationSetup, CategoryBabyCrib, CategoryDietaryAllergy, CategoryOther:
		return true
	default:
		return false
	}
}

// IsValidStatus memeriksa keabsahan status target.
func IsValidStatus(st Status) bool {
	switch st {
	case StatusPending, StatusAcknowledged, StatusFulfilled, StatusDeclined:
		return true
	default:
		return false
	}
}

// CleanString membersihkan whitespace ekstra.
func CleanString(s string) string {
	return strings.TrimSpace(s)
}
