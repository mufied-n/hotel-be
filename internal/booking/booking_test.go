package booking

import (
	"errors"
	"testing"
	"time"
)

func TestTransition_Legal(t *testing.T) {
	cases := []struct {
		from, to Status
	}{
		{StatusPending, StatusConfirmed},
		{StatusPending, StatusExpired},
		{StatusPending, StatusFailed},
		{StatusPending, StatusCancelled},
		{StatusConfirmed, StatusCheckedIn},
		{StatusConfirmed, StatusCancelled},
		{StatusConfirmed, StatusNoShow},
		{StatusCheckedIn, StatusCheckedOut},
	}
	for _, c := range cases {
		if err := Transition(c.from, c.to); err != nil {
			t.Errorf("Transition(%s → %s) = %v, want nil", c.from, c.to, err)
		}
	}
}

func TestTransition_Illegal(t *testing.T) {
	cases := []struct {
		from, to Status
	}{
		{StatusPending, StatusCheckedIn},   // belum dibayar
		{StatusPending, StatusCheckedOut},  // lompat jauh
		{StatusConfirmed, StatusPending},   // tidak boleh mundur
		{StatusConfirmed, StatusExpired},   // expired hanya dari pending
		{StatusCancelled, StatusConfirmed}, // terminal
		{StatusExpired, StatusConfirmed},   // terminal
		{StatusCheckedOut, StatusCheckedIn},
	}
	for _, c := range cases {
		err := Transition(c.from, c.to)
		if err == nil {
			t.Errorf("Transition(%s → %s) = nil, want error", c.from, c.to)
			continue
		}
		if !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("Transition(%s → %s) error bukan ErrIllegalTransition: %v", c.from, c.to, err)
		}
	}
}

func TestBooking_Nights(t *testing.T) {
	b := Booking{
		CheckIn:  time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC),
		CheckOut: time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC),
	}
	if got := b.Nights(); got != 3 {
		t.Errorf("b.Nights() = %d, want 3", got)
	}
}

func TestBooking_ToPublicDTO(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		booking Booking
	}{
		{
			name: "masks guest PII and token",
			booking: Booking{
				ID:              "bk-001",
				Status:          StatusConfirmed,
				RoomTypeID:      "deluxe",
				CheckIn:         now,
				CheckOut:        now.Add(48 * time.Hour),
				NumRooms:        2,
				NumGuests:       4,
				TotalPriceMinor: 2_500_000,
				Currency:        "IDR",
				GuestName:       "Rahasia Tamu",
				GuestEmail:           "rahasia@example.com",
				GuestToken:           "gst_super_secret_token",
				EstimatedArrivalTime: "14:00",
				SpecialRequests:      "Alergi kacang dan setup honeymoon",
				CreatedAt:            now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dto := tt.booking.ToPublicDTO()
			if dto.ID != tt.booking.ID {
				t.Errorf("ID = %s, want %s", dto.ID, tt.booking.ID)
			}
			if dto.Status != tt.booking.Status {
				t.Errorf("Status = %s, want %s", dto.Status, tt.booking.Status)
			}
			if dto.RoomTypeID != tt.booking.RoomTypeID {
				t.Errorf("RoomTypeID = %s, want %s", dto.RoomTypeID, tt.booking.RoomTypeID)
			}
			if dto.NumRooms != tt.booking.NumRooms {
				t.Errorf("NumRooms = %d, want %d", dto.NumRooms, tt.booking.NumRooms)
			}
			if dto.SpecialRequests != "" {
				t.Errorf("SpecialRequests = %s, want empty (BE-R02 masked)", dto.SpecialRequests)
			}
			if dto.EstimatedArrivalTime != "" {
				t.Errorf("EstimatedArrivalTime = %s, want empty (BE-R02 masked)", dto.EstimatedArrivalTime)
			}
		})
	}
}

