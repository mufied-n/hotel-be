package inventory

import (
	"errors"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestCheck_OK(t *testing.T) {
	avail := []Availability{
		{Date: date("2026-10-10"), AvailableRooms: 3},
		{Date: date("2026-10-11"), AvailableRooms: 1},
		{Date: date("2026-10-12"), AvailableRooms: 5},
	}
	if err := Check(avail, date("2026-10-10"), date("2026-10-13"), 1); err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
}

func TestCheck_Insufficient(t *testing.T) {
	avail := []Availability{
		{Date: date("2026-10-10"), AvailableRooms: 3},
		{Date: date("2026-10-11"), AvailableRooms: 1},
	}
	err := Check(avail, date("2026-10-10"), date("2026-10-12"), 2)
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("Check = %v, want ErrInsufficient", err)
	}
}

func TestCheck_MissingRows(t *testing.T) {
	avail := []Availability{
		{Date: date("2026-10-10"), AvailableRooms: 3}, // 1 baris untuk 3 malam
	}
	err := Check(avail, date("2026-10-10"), date("2026-10-13"), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Check = %v, want ErrNotFound", err)
	}
}

func TestCheckWithBuffer(t *testing.T) {
	tests := []struct {
		name         string
		avail        []Availability
		from         time.Time
		to           time.Time
		rooms        int
		safetyBuffer int
		wantErr      error
	}{
		{
			name: "sufficient with safety buffer",
			avail: []Availability{
				{Date: date("2026-10-10"), AvailableRooms: 3},
				{Date: date("2026-10-11"), AvailableRooms: 2},
			},
			from:         date("2026-10-10"),
			to:           date("2026-10-12"),
			rooms:        1,
			safetyBuffer: 1,
			wantErr:      nil,
		},
		{
			name: "exact safety buffer threshold violation",
			avail: []Availability{
				{Date: date("2026-10-10"), AvailableRooms: 2},
				{Date: date("2026-10-11"), AvailableRooms: 1}, // 1 - 1 = 0 < buffer 1
			},
			from:         date("2026-10-10"),
			to:           date("2026-10-12"),
			rooms:        1,
			safetyBuffer: 1,
			wantErr:      ErrInsufficient,
		},
		{
			name: "negative safety buffer treated as zero",
			avail: []Availability{
				{Date: date("2026-10-10"), AvailableRooms: 1},
			},
			from:         date("2026-10-10"),
			to:           date("2026-10-11"),
			rooms:        1,
			safetyBuffer: -1,
			wantErr:      nil,
		},
		{
			name: "missing dates returns ErrNotFound",
			avail: []Availability{
				{Date: date("2026-10-10"), AvailableRooms: 5},
			},
			from:         date("2026-10-10"),
			to:           date("2026-10-12"),
			rooms:        1,
			safetyBuffer: 1,
			wantErr:      ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckWithBuffer(tt.avail, tt.from, tt.to, tt.rooms, tt.safetyBuffer)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("CheckWithBuffer() unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckWithBuffer() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

