package stay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/rates"
)

type mockStayStore struct {
	getBookingFunc    func(ctx context.Context, bookingID string) (*BookingDetails, error)
	moveRoomFunc      func(ctx context.Context, input RoomMoveInput, moveDate time.Time) (*RoomMoveResult, error)
	extendStayFunc    func(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*ExtendStayResult, error)
	listRoomMovesFunc func(ctx context.Context, bookingID string) ([]RoomMoveLog, error)
}

func (m *mockStayStore) GetBooking(ctx context.Context, bookingID string) (*BookingDetails, error) {
	if m.getBookingFunc != nil {
		return m.getBookingFunc(ctx, bookingID)
	}
	return nil, nil
}

func (m *mockStayStore) MoveRoom(ctx context.Context, input RoomMoveInput, moveDate time.Time) (*RoomMoveResult, error) {
	if m.moveRoomFunc != nil {
		return m.moveRoomFunc(ctx, input, moveDate)
	}
	return nil, nil
}

func (m *mockStayStore) ExtendStay(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*ExtendStayResult, error) {
	if m.extendStayFunc != nil {
		return m.extendStayFunc(ctx, bookingID, additionalNights, newCheckOut, additionalRates, additionalTotal)
	}
	return nil, nil
}

func (m *mockStayStore) ListRoomMoves(ctx context.Context, bookingID string) ([]RoomMoveLog, error) {
	if m.listRoomMovesFunc != nil {
		return m.listRoomMovesFunc(ctx, bookingID)
	}
	return nil, nil
}

type mockRateQuoter struct {
	quoteFunc func(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time) ([]rates.Quote, error)
}

func (m *mockRateQuoter) Quote(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time) ([]rates.Quote, error) {
	if m.quoteFunc != nil {
		return m.quoteFunc(ctx, roomTypeID, checkIn, checkOut)
	}
	return nil, nil
}

func TestStayService_MoveRoom_TableTest(t *testing.T) {
	fixedNow := time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		input      RoomMoveInput
		mockResult *RoomMoveResult
		mockErr    error
		expectErr  error
	}{
		{
			name: "Valid room move returns success result",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "305",
				ReasonCategory:   ReasonMaintenanceDefect,
				Notes:            "AC issue",
			},
			mockResult: &RoomMoveResult{
				Status:             "ok",
				BookingID:          "bk-001",
				PreviousRoomNumber: "101",
				NewRoomNumber:      "305",
				MoveDate:           "2026-10-03",
				Message:            "pemindahan kamar berhasil",
			},
			expectErr: nil,
		},
		{
			name: "Empty booking ID returns ErrBookingNotFound",
			input: RoomMoveInput{
				BookingID:        "",
				TargetRoomNumber: "305",
				ReasonCategory:   ReasonMaintenanceDefect,
			},
			expectErr: ErrBookingNotFound,
		},
		{
			name: "Empty target room number returns ErrTargetRoomNotReady",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "",
				ReasonCategory:   ReasonMaintenanceDefect,
			},
			expectErr: ErrTargetRoomNotReady,
		},
		{
			name: "Invalid reason category returns ErrInvalidReasonCategory",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "305",
				ReasonCategory:   "invalid_reason",
			},
			expectErr: ErrInvalidReasonCategory,
		},
		{
			name: "Target room not ready returns ErrTargetRoomNotReady from store",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "305",
				ReasonCategory:   ReasonGuestRequest,
			},
			mockErr:   ErrTargetRoomNotReady,
			expectErr: ErrTargetRoomNotReady,
		},
		{
			name: "Same room move returns ErrSameRoomMove from store",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "101",
				ReasonCategory:   ReasonUpgrade,
			},
			mockErr:   ErrSameRoomMove,
			expectErr: ErrSameRoomMove,
		},
		{
			name: "Non-checked-in booking returns ErrInvalidBookingStatus",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "305",
				ReasonCategory:   ReasonMaintenanceDefect,
			},
			mockErr:   ErrInvalidBookingStatus,
			expectErr: ErrInvalidBookingStatus,
		},
		{
			name: "Overlapping reservation returns ErrRoomPhysicalOverlap",
			input: RoomMoveInput{
				BookingID:        "bk-001",
				TargetRoomNumber: "305",
				ReasonCategory:   ReasonNoiseComplaint,
			},
			mockErr:   ErrRoomPhysicalOverlap,
			expectErr: ErrRoomPhysicalOverlap,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStayStore{
				moveRoomFunc: func(ctx context.Context, input RoomMoveInput, moveDate time.Time) (*RoomMoveResult, error) {
					if tc.mockErr != nil {
						return nil, tc.mockErr
					}
					return tc.mockResult, nil
				},
			}

			svc := NewService(store, nil, nil)
			svc.SetClock(func() time.Time { return fixedNow })

			res, err := svc.MoveRoom(context.Background(), tc.input)

			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.expectErr)
				}
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.NewRoomNumber != tc.input.TargetRoomNumber {
				t.Errorf("expected new room %s, got %s", tc.input.TargetRoomNumber, res.NewRoomNumber)
			}
		})
	}
}

func TestStayService_ExtendStay_TableTest(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	checkOut := now.AddDate(0, 0, 2)

	tests := []struct {
		name          string
		input         ExtendStayInput
		booking       *BookingDetails
		getBookingErr error
		quotes        []rates.Quote
		quoteErr      error
		extendErr     error
		expectErr     error
	}{
		{
			name: "Valid extension calculates dynamic rates and succeeds",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 2,
			},
			booking: &BookingDetails{
				ID:              "bk-001",
				Status:          "checked_in",
				RoomTypeID:      "01900000-0000-7000-8000-000000000001",
				CheckIn:         now,
				CheckOut:        checkOut,
				NumRooms:        1,
				TotalPriceMinor: 1_100_000,
			},
			quotes: []rates.Quote{
				{Date: checkOut, RateMinor: 550_000},
				{Date: checkOut.AddDate(0, 0, 1), RateMinor: 687_500}, // weekend rate
			},
			expectErr: nil,
		},
		{
			name: "Empty booking ID returns ErrBookingNotFound",
			input: ExtendStayInput{
				BookingID:        "",
				AdditionalNights: 1,
			},
			expectErr: ErrBookingNotFound,
		},
		{
			name: "Zero additional nights returns ErrInvalidAdditionalNights",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 0,
			},
			expectErr: ErrInvalidAdditionalNights,
		},
		{
			name: "Excessive additional nights (>30) returns ErrInvalidAdditionalNights",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 31,
			},
			expectErr: ErrInvalidAdditionalNights,
		},
		{
			name: "Booking not found in store returns ErrBookingNotFound",
			input: ExtendStayInput{
				BookingID:        "bk-not-exist",
				AdditionalNights: 1,
			},
			getBookingErr: ErrBookingNotFound,
			expectErr:     ErrBookingNotFound,
		},
		{
			name: "Booking with cancelled status returns ErrInvalidBookingStatus",
			input: ExtendStayInput{
				BookingID:        "bk-cancelled",
				AdditionalNights: 1,
			},
			booking: &BookingDetails{
				ID:       "bk-cancelled",
				Status:   "cancelled",
				CheckOut: checkOut,
			},
			expectErr: ErrInvalidBookingStatus,
		},
		{
			name: "No inventory available returns ErrNoAvailabilityForExtension",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 1,
			},
			booking: &BookingDetails{
				ID:         "bk-001",
				Status:     "checked_in",
				RoomTypeID: "01900000-0000-7000-8000-000000000001",
				CheckIn:    now,
				CheckOut:   checkOut,
				NumRooms:   1,
			},
			quotes: []rates.Quote{
				{Date: checkOut, RateMinor: 550_000},
			},
			extendErr: ErrNoAvailabilityForExtension,
			expectErr: ErrNoAvailabilityForExtension,
		},
		{
			name: "Room physical overlap returns ErrRoomPhysicalOverlap",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 1,
			},
			booking: &BookingDetails{
				ID:         "bk-001",
				Status:     "checked_in",
				RoomTypeID: "01900000-0000-7000-8000-000000000001",
				CheckIn:    now,
				CheckOut:   checkOut,
				NumRooms:   1,
			},
			quotes: []rates.Quote{
				{Date: checkOut, RateMinor: 550_000},
			},
			extendErr: ErrRoomPhysicalOverlap,
			expectErr: ErrRoomPhysicalOverlap,
		},
		{
			name: "Rate engine error returns wrapped error",
			input: ExtendStayInput{
				BookingID:        "bk-001",
				AdditionalNights: 1,
			},
			booking: &BookingDetails{
				ID:         "bk-001",
				Status:     "checked_in",
				RoomTypeID: "01900000-0000-7000-8000-000000000001",
				CheckIn:    now,
				CheckOut:   checkOut,
				NumRooms:   1,
			},
			quoteErr:  errors.New("quote calculation failure"),
			expectErr: errors.New("stay: calculate extension rates: quote calculation failure"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStayStore{
				getBookingFunc: func(ctx context.Context, bookingID string) (*BookingDetails, error) {
					if tc.getBookingErr != nil {
						return nil, tc.getBookingErr
					}
					return tc.booking, nil
				},
				extendStayFunc: func(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*ExtendStayResult, error) {
					if tc.extendErr != nil {
						return nil, tc.extendErr
					}
					return &ExtendStayResult{
						Status:                "ok",
						BookingID:             bookingID,
						PreviousCheckOut:      checkOut.Format("2006-01-02"),
						NewCheckOut:           newCheckOut.Format("2006-01-02"),
						AdditionalNights:      additionalNights,
						AdditionalAmountMinor: additionalTotal,
						NewTotalPriceMinor:    tc.booking.TotalPriceMinor + additionalTotal,
					}, nil
				},
			}

			quoter := &mockRateQuoter{
				quoteFunc: func(ctx context.Context, roomTypeID string, checkIn, checkOut time.Time) ([]rates.Quote, error) {
					if tc.quoteErr != nil {
						return nil, tc.quoteErr
					}
					return tc.quotes, nil
				},
			}

			svc := NewService(store, quoter, nil)
			res, err := svc.ExtendStay(context.Background(), tc.input)

			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.expectErr)
				}
				if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.AdditionalNights != tc.input.AdditionalNights {
				t.Errorf("expected additional nights %d, got %d", tc.input.AdditionalNights, res.AdditionalNights)
			}
		})
	}
}

func TestStayService_ExtendStay_NilRateEngineFallback(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	checkOut := now.AddDate(0, 0, 2)

	store := &mockStayStore{
		getBookingFunc: func(ctx context.Context, bookingID string) (*BookingDetails, error) {
			return &BookingDetails{
				ID:              "bk-001",
				Status:          "checked_in",
				RoomTypeID:      "01900000-0000-7000-8000-000000000001",
				CheckIn:         now,
				CheckOut:        checkOut,
				NumRooms:        2,
				TotalPriceMinor: 2_200_000,
			}, nil
		},
		extendStayFunc: func(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*ExtendStayResult, error) {
			return &ExtendStayResult{
				Status:                "ok",
				BookingID:             bookingID,
				AdditionalNights:      additionalNights,
				AdditionalAmountMinor: additionalTotal,
			}, nil
		},
	}

	svc := NewService(store, nil, nil)
	res, err := svc.ExtendStay(context.Background(), ExtendStayInput{
		BookingID:        "bk-001",
		AdditionalNights: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.AdditionalAmountMinor != 3_000_000 { // 2 rooms * 2 nights * 750_000
		t.Errorf("expected 3_000_000, got %d", res.AdditionalAmountMinor)
	}
}

func TestStayService_ListRoomMoves(t *testing.T) {
	store := &mockStayStore{
		listRoomMovesFunc: func(ctx context.Context, bookingID string) ([]RoomMoveLog, error) {
			if bookingID == "bk-001" {
				return []RoomMoveLog{
					{
						ID:             "move-1",
						BookingID:      "bk-001",
						FromRoomNumber: "101",
						ToRoomNumber:   "305",
						MoveDate:       "2026-10-03",
						ReasonCategory: string(ReasonMaintenanceDefect),
						Notes:          "AC broken",
					},
				}, nil
			}
			return nil, nil
		},
	}

	svc := NewService(store, nil, nil)

	// Positive test
	logs, err := svc.ListRoomMoves(context.Background(), "bk-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].FromRoomNumber != "101" || logs[0].ToRoomNumber != "305" {
		t.Errorf("unexpected log data: %v", logs[0])
	}

	// Negative test: empty booking ID
	_, err = svc.ListRoomMoves(context.Background(), "   ")
	if !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("expected ErrBookingNotFound, got %v", err)
	}
}
