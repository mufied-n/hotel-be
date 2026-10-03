package frontdesk

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Service mendefinisikan use cases bisnis meja depan hotel.
type Service interface {
	GetDailyRoster(ctx context.Context, targetDate time.Time) (*DailyRoster, error)
	RecordHandover(ctx context.Context, input RecordHandoverInput) (*HandoverNote, error)
	ListHandovers(ctx context.Context, limit, offset int) ([]HandoverNote, int, error)
}

// DefaultService implementasi standar Service.
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

// GetDailyRoster menyajikan roster operasional meja depan untuk tanggal tertentu.
func (s *DefaultService) GetDailyRoster(ctx context.Context, targetDate time.Time) (*DailyRoster, error) {
	if targetDate.IsZero() {
		targetDate = time.Now()
	}
	return s.store.GetDailyRoster(ctx, targetDate)
}

// RecordHandover memvalidasi dan menyimpan catatan serah terima shift baru.
func (s *DefaultService) RecordHandover(ctx context.Context, input RecordHandoverInput) (*HandoverNote, error) {
	shiftClean := strings.ToLower(strings.TrimSpace(string(input.Shift)))
	switch ShiftType(shiftClean) {
	case ShiftMorning, ShiftAfternoon, ShiftNight:
		// Sah
	default:
		return nil, ErrInvalidShift
	}

	if strings.TrimSpace(input.PendingIssues) == "" && strings.TrimSpace(input.VIPGuestNotes) == "" {
		return nil, ErrEmptyHandoverNote
	}

	actorID := strings.TrimSpace(input.ActorID)
	if actorID == "" {
		actorID = "staff:receptionist"
	}
	actorRole := strings.TrimSpace(input.ActorRole)
	if actorRole == "" {
		actorRole = "receptionist"
	}

	note := &HandoverNote{
		Shift:          ShiftType(shiftClean),
		CashFloatMinor: input.CashFloatMinor,
		PendingIssues:  strings.TrimSpace(input.PendingIssues),
		VIPGuestNotes:  strings.TrimSpace(input.VIPGuestNotes),
		ActorID:        actorID,
		ActorRole:      actorRole,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.store.CreateHandoverNote(ctx, note); err != nil {
		return nil, fmt.Errorf("frontdesk: record handover: %w", err)
	}

	s.log.InfoContext(ctx, "frontdesk.handover_recorded",
		"id", note.ID,
		"shift", note.Shift,
		"actor", note.ActorID,
		"cash_float", note.CashFloatMinor,
	)

	return note, nil
}

// ListHandovers mengambil riwayat catatan serah terima shift.
func (s *DefaultService) ListHandovers(ctx context.Context, limit, offset int) ([]HandoverNote, int, error) {
	return s.store.ListHandoverNotes(ctx, limit, offset)
}
