package housekeeping

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Service mendefinisikan use cases bisnis operasional kebersihan kamar.
type Service interface {
	GetRoomBoard(ctx context.Context, floor int, status string, roomTypeID string) (*RoomBoardSummary, error)
	UpdateStatus(ctx context.Context, input UpdateStatusInput) error
	ValidateRoomForCheckIn(ctx context.Context, roomNumber string) error
	MarkRoomDirtyOnCheckOut(ctx context.Context, roomNumber string) error
	MarkRoomOutOfOrder(ctx context.Context, roomNumber string, startDate, endDate time.Time, reason string) error
}

// DefaultService adalah implementasi standar dari Service.
type DefaultService struct {
	store Store
	log   *slog.Logger
}

// NewService membuat instance baru DefaultService.
func NewService(store Store, log *slog.Logger) *DefaultService {
	if log == nil {
		log = slog.Default()
	}
	return &DefaultService{
		store: store,
		log:   log,
	}
}

// isTransitionLegal memeriksa legalitas perpindahan status kebersihan kamar.
func isTransitionLegal(from, to CleanlinessStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusVacantDirty:
		return to == StatusCleaning || to == StatusOutOfService || to == StatusOutOfOrder
	case StatusCleaning:
		return to == StatusVacantClean || to == StatusVacantDirty || to == StatusOutOfService || to == StatusOutOfOrder
	case StatusVacantClean:
		return to == StatusInspected || to == StatusVacantDirty || to == StatusCleaning || to == StatusOutOfService || to == StatusOutOfOrder
	case StatusInspected:
		return to == StatusOccupied || to == StatusVacantDirty || to == StatusOutOfService || to == StatusOutOfOrder
	case StatusOccupied:
		return to == StatusVacantDirty || to == StatusOutOfService || to == StatusOutOfOrder
	case StatusOutOfService:
		return to == StatusVacantDirty || to == StatusCleaning || to == StatusVacantClean || to == StatusInspected || to == StatusOutOfOrder
	case StatusOutOfOrder:
		return to == StatusVacantDirty || to == StatusOutOfService
	default:
		return false
	}
}

// GetRoomBoard menyajikan ringkasan dan daftar kamar operasional.
func (s *DefaultService) GetRoomBoard(ctx context.Context, floor int, status string, roomTypeID string) (*RoomBoardSummary, error) {
	rooms, err := s.store.ListRooms(ctx, floor, strings.TrimSpace(status), strings.TrimSpace(roomTypeID))
	if err != nil {
		return nil, err
	}

	summaryMap := make(map[CleanlinessStatus]int)
	// Inisialisasi seluruh enum status
	summaryMap[StatusVacantDirty] = 0
	summaryMap[StatusCleaning] = 0
	summaryMap[StatusVacantClean] = 0
	summaryMap[StatusInspected] = 0
	summaryMap[StatusOccupied] = 0
	summaryMap[StatusOutOfService] = 0
	summaryMap[StatusOutOfOrder] = 0

	for _, r := range rooms {
		summaryMap[r.CleanlinessStatus]++
	}

	return &RoomBoardSummary{
		TotalRooms: len(rooms),
		Summary:    summaryMap,
		Rooms:      rooms,
	}, nil
}

// UpdateStatus memvalidasi dan mengeksekusi perpindahan status kebersihan kamar.
func (s *DefaultService) UpdateStatus(ctx context.Context, input UpdateStatusInput) error {
	cleanRoom := strings.TrimSpace(input.RoomNumber)
	if cleanRoom == "" {
		return ErrRoomNotFound
	}

	current, err := s.store.GetRoom(ctx, cleanRoom)
	if err != nil {
		return err
	}

	// Otorisasi role: OOO hanya boleh oleh gm_admin
	if input.ToStatus == StatusOutOfOrder && input.ActorRole != "gm_admin" {
		return ErrUnauthorizedTransition
	}

	// Validasi aturan state machine
	if !isTransitionLegal(current.CleanlinessStatus, input.ToStatus) {
		s.log.WarnContext(ctx, "housekeeping.illegal_transition_rejected",
			"room", cleanRoom,
			"from", current.CleanlinessStatus,
			"to", input.ToStatus,
		)
		return fmt.Errorf("%w: dari %s ke %s", ErrInvalidTransition, current.CleanlinessStatus, input.ToStatus)
	}

	actor := strings.TrimSpace(input.ActorID)
	if actor == "" {
		actor = "staff:" + input.ActorRole
	}

	if err := s.store.UpdateRoomCleanliness(ctx, cleanRoom, input.ToStatus, strings.TrimSpace(input.Notes), actor); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "housekeeping.room_status_updated",
		"room", cleanRoom,
		"from", current.CleanlinessStatus,
		"to", input.ToStatus,
		"actor", actor,
	)

	return nil
}

// ValidateRoomForCheckIn memverifikasi kamar siap huni sebelum check-in dan mengubahnya menjadi occupied.
func (s *DefaultService) ValidateRoomForCheckIn(ctx context.Context, roomNumber string) error {
	cleanRoom := strings.TrimSpace(roomNumber)
	if cleanRoom == "" {
		return ErrRoomNotFound
	}

	room, err := s.store.GetRoom(ctx, cleanRoom)
	if err != nil {
		return err
	}

	// Guard: Hanya kamar berstatus inspected yang boleh di-check-in
	if room.CleanlinessStatus != StatusInspected {
		s.log.WarnContext(ctx, "housekeeping.checkin_blocked_dirty_room",
			"room", cleanRoom,
			"status", room.CleanlinessStatus,
		)
		return fmt.Errorf("%w: status saat ini '%s'", ErrRoomNotReady, room.CleanlinessStatus)
	}

	// Ubah status kamar menjadi occupied
	return s.store.UpdateRoomCleanliness(ctx, cleanRoom, StatusOccupied, "Tamu check-in", "front_desk")
}

// MarkRoomDirtyOnCheckOut menandai kamar fisik sebagai vacant_dirty saat tamu check-out.
func (s *DefaultService) MarkRoomDirtyOnCheckOut(ctx context.Context, roomNumber string) error {
	cleanRoom := strings.TrimSpace(roomNumber)
	if cleanRoom == "" {
		return nil
	}

	return s.store.UpdateRoomCleanliness(ctx, cleanRoom, StatusVacantDirty, "Tamu check-out, perlu pembersihan", "front_desk")
}

// MarkRoomOutOfOrder menandai kamar rusak berat dan memotong kuota inventaris penjualan.
func (s *DefaultService) MarkRoomOutOfOrder(ctx context.Context, roomNumber string, startDate, endDate time.Time, reason string) error {
	cleanRoom := strings.TrimSpace(roomNumber)
	room, err := s.store.GetRoom(ctx, cleanRoom)
	if err != nil {
		return err
	}

	if err := s.store.UpdateRoomCleanliness(ctx, cleanRoom, StatusOutOfOrder, reason, "gm_admin"); err != nil {
		return err
	}

	// Potong kuota inventaris agar kamar tidak terjual di web
	return s.store.DeductInventoryForOOO(ctx, room.RoomTypeID, startDate, endDate)
}
