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
