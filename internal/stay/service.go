package stay

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/rates"
)

// RateQuoter interface untuk menghitung tarif malam menginap tambahan.
type RateQuoter interface {
	Quote(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time) ([]rates.Quote, error)
}

// Service mendefinisikan use cases bisnis untuk modifikasi masa menginap.
type Service interface {
	MoveRoom(ctx context.Context, input RoomMoveInput) (*RoomMoveResult, error)
	ExtendStay(ctx context.Context, input ExtendStayInput) (*ExtendStayResult, error)
	ListRoomMoves(ctx context.Context, bookingID string) ([]RoomMoveLog, error)
}

// DefaultService implementasi standar Service.
type DefaultService struct {
	store      Store
	rateEngine RateQuoter
	log        *slog.Logger
	clock      func() time.Time
}

// NewService membuat instance baru DefaultService.
func NewService(store Store, rateEngine RateQuoter, log *slog.Logger) *DefaultService {
	if log == nil {
		log = slog.Default()
	}
	return &DefaultService{
		store:      store,
		rateEngine: rateEngine,
		log:        log,
		clock:      time.Now,
	}
}

// SetClock mengatur fungsi penunjuk waktu (berguna untuk testing).
func (s *DefaultService) SetClock(fn func() time.Time) {
	s.clock = fn
}

// MoveRoom memvalidasi alasan perpindahan dan memindahkan kamar fisik tamu.
func (s *DefaultService) MoveRoom(ctx context.Context, input RoomMoveInput) (*RoomMoveResult, error) {
	input.BookingID = strings.TrimSpace(input.BookingID)
	input.TargetRoomNumber = strings.TrimSpace(input.TargetRoomNumber)
	input.Notes = strings.TrimSpace(input.Notes)

	if input.BookingID == "" {
		return nil, ErrBookingNotFound
	}
	if input.TargetRoomNumber == "" {
		return nil, ErrTargetRoomNotReady
	}
	if !input.ReasonCategory.IsValid() {
		return nil, ErrInvalidReasonCategory
	}

	moveDate := s.clock().UTC()
	res, err := s.store.MoveRoom(ctx, input, moveDate)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "stay.room_moved",
		"booking_id", res.BookingID,
		"from_room", res.PreviousRoomNumber,
		"to_room", res.NewRoomNumber,
		"reason", input.ReasonCategory,
	)

	return res, nil
}

// ExtendStay memvalidasi malam perpanjangan, menghitung tarif tambahan, dan memperpanjang masa menginap.
func (s *DefaultService) ExtendStay(ctx context.Context, input ExtendStayInput) (*ExtendStayResult, error) {
	input.BookingID = strings.TrimSpace(input.BookingID)
	if input.BookingID == "" {
		return nil, ErrBookingNotFound
	}
	if input.AdditionalNights < 1 || input.AdditionalNights > 30 {
		return nil, ErrInvalidAdditionalNights
	}

	b, err := s.store.GetBooking(ctx, input.BookingID)
	if err != nil {
		return nil, err
	}

	if b.Status != "checked_in" && b.Status != "confirmed" {
		return nil, ErrInvalidBookingStatus
	}

	oldCheckOutClean := time.Date(b.CheckOut.Year(), b.CheckOut.Month(), b.CheckOut.Day(), 0, 0, 0, 0, time.UTC)
	newCheckOutClean := oldCheckOutClean.AddDate(0, 0, input.AdditionalNights)

	var additionalRates []int64
	var additionalTotalPerRoom int64

	if s.rateEngine != nil {
		quotes, err := s.rateEngine.Quote(ctx, b.RoomTypeID, oldCheckOutClean, newCheckOutClean)
		if err != nil {
			return nil, fmt.Errorf("stay: calculate extension rates: %w", err)
		}
		for _, q := range quotes {
			additionalRates = append(additionalRates, q.RateMinor)
			additionalTotalPerRoom += q.RateMinor
		}
	} else {
		// Fallback rate standar bila rate engine tidak terpasang
		baseRate := int64(750_000)
		for i := 0; i < input.AdditionalNights; i++ {
			additionalRates = append(additionalRates, baseRate)
			additionalTotalPerRoom += baseRate
		}
	}

	additionalTotal := additionalTotalPerRoom * int64(b.NumRooms)

	res, err := s.store.ExtendStay(ctx, input.BookingID, input.AdditionalNights, newCheckOutClean, additionalRates, additionalTotal)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "stay.stay_extended",
		"booking_id", res.BookingID,
		"old_checkout", res.PreviousCheckOut,
		"new_checkout", res.NewCheckOut,
		"additional_nights", res.AdditionalNights,
		"additional_amount", res.AdditionalAmountMinor,
	)

	return res, nil
}

// ListRoomMoves menyajikan riwayat pemindahan kamar untuk suatu reservasi.
func (s *DefaultService) ListRoomMoves(ctx context.Context, bookingID string) ([]RoomMoveLog, error) {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return nil, ErrBookingNotFound
	}
	return s.store.ListRoomMoves(ctx, bookingID)
}
