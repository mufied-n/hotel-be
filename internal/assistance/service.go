package assistance

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Service antarmuka domain logika bisnis pengelolaan permintaan khusus tamu.
type Service interface {
	CreateGuestRequest(ctx context.Context, guestEmail string, input CreateRequestInput) (*SpecialRequest, error)
	ListGuestRequests(ctx context.Context, guestEmail, bookingID string) ([]SpecialRequest, error)
	ListStaffQueue(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error)
	UpdateStatus(ctx context.Context, input UpdateStatusInput) (*SpecialRequest, error)
}

// DefaultService implementasi standar Service.
type DefaultService struct {
	store Store
	log   *slog.Logger
	clock func() time.Time
}

// NewService membuat instance baru DefaultService.
func NewService(store Store, log *slog.Logger) *DefaultService {
	if log == nil {
		log = slog.Default()
	}
	return &DefaultService{
		store: store,
		log:   log,
		clock: time.Now,
	}
}

// SetClock mengatur fungsi penunjuk waktu (berguna untuk testing).
func (s *DefaultService) SetClock(fn func() time.Time) {
	s.clock = fn
}

// CreateGuestRequest memvalidasi input tamu, mengecek kepemilikan booking (Anti-IDOR), dan membuat permintaan.
func (s *DefaultService) CreateGuestRequest(ctx context.Context, guestEmail string, input CreateRequestInput) (*SpecialRequest, error) {
	input.BookingID = strings.TrimSpace(input.BookingID)
	input.Description = strings.TrimSpace(input.Description)
	input.TargetTime = strings.TrimSpace(input.TargetTime)
	guestEmail = strings.TrimSpace(guestEmail)

	if input.BookingID == "" {
		return nil, ErrBookingNotFound
	}
	if !IsValidCategory(input.Category) {
		return nil, ErrInvalidCategory
	}
	if input.Description == "" {
		return nil, ErrEmptyDescription
	}

	ownerEmail, exists, err := s.store.GetBookingOwner(ctx, input.BookingID)
	if err != nil {
		return nil, err
	}
	if !exists || !strings.EqualFold(ownerEmail, guestEmail) {
		return nil, ErrBookingNotFound // Anti-IDOR: cegah kebocoran keberadaan booking tamu lain
	}

	dept := ResolveDepartment(input.Category)
	req := SpecialRequest{
		BookingID:   input.BookingID,
		Category:    input.Category,
		Department:  dept,
		Description: input.Description,
		TargetTime:  input.TargetTime,
		Status:      StatusPending,
		StaffNotes:  "",
		CreatedAt:   s.clock(),
		UpdatedAt:   s.clock(),
	}

	return s.store.CreateRequest(ctx, req)
}

// ListGuestRequests menyajikan daftar permintaan khusus milik reservasi tamu setelah verifikasi kepemilikan.
func (s *DefaultService) ListGuestRequests(ctx context.Context, guestEmail, bookingID string) ([]SpecialRequest, error) {
	bookingID = strings.TrimSpace(bookingID)
	guestEmail = strings.TrimSpace(guestEmail)

	if bookingID == "" {
		return nil, ErrBookingNotFound
	}

	ownerEmail, exists, err := s.store.GetBookingOwner(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !exists || !strings.EqualFold(ownerEmail, guestEmail) {
		return nil, ErrBookingNotFound
	}

	return s.store.ListByBookingID(ctx, bookingID)
}

// ListStaffQueue menyajikan antrean tugas staf yang terfilter menurut departemen dan status.
func (s *DefaultService) ListStaffQueue(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error) {
	filter.Department = strings.TrimSpace(filter.Department)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.BookingID = strings.TrimSpace(filter.BookingID)

	return s.store.ListStaffQueue(ctx, filter)
}

// UpdateStatus memvalidasi transisi status pemenuhan dan mencatat aktor staf serta alasan.
func (s *DefaultService) UpdateStatus(ctx context.Context, input UpdateStatusInput) (*SpecialRequest, error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.StaffNotes = strings.TrimSpace(input.StaffNotes)
	input.HandledBy = strings.TrimSpace(input.HandledBy)

	if input.RequestID == "" {
		return nil, ErrRequestNotFound
	}
	if !IsValidStatus(input.ToStatus) || input.ToStatus == StatusPending {
		return nil, ErrInvalidStatusTransition
	}

	existing, err := s.store.GetRequestByID(ctx, input.RequestID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrRequestNotFound
	}

	// Status terminal tidak dapat diubah kembali
	if existing.Status == StatusFulfilled || existing.Status == StatusDeclined {
		return nil, ErrInvalidStatusTransition
	}

	// Penolakan wajib memberikan catatan staf minimal 5 karakter
	if input.ToStatus == StatusDeclined && len(input.StaffNotes) < 5 {
		return nil, ErrStaffNotesRequired
	}

	if input.HandledBy == "" {
		input.HandledBy = "staff"
	}

	return s.store.UpdateStatus(ctx, input.RequestID, input.ToStatus, input.StaffNotes, input.HandledBy, s.clock())
}
