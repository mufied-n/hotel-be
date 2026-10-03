package handler

import (
	"net/http"
	"testing"
)

func TestParseGuestQuery_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		rooms      string
		adults     string
		children   string
		childAges  string
		wantRooms  int
		wantAdults int
		wantKids   int
		wantTotal  int
		wantStatus int
		wantCode   string
	}{
		{
			name:       "valid standard query defaults to 1 room 1 adult",
			rooms:      "",
			adults:     "",
			children:   "",
			childAges:  "",
			wantRooms:  1,
			wantAdults: 1,
			wantKids:   0,
			wantTotal:  1,
		},
		{
			name:       "invalid room count > 8 returns 400 INVALID_ROOM_COUNT",
			rooms:      "9",
			adults:     "2",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_COUNT",
		},
		{
			name:       "invalid adults < 1 returns 400 INVALID_GUEST_COUNT",
			rooms:      "1",
			adults:     "0",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "adults < rooms returns 400 INVALID_GUEST_COUNT",
			rooms:      "2",
			adults:     "1",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "negative children returns 400 INVALID_GUEST_COUNT",
			rooms:      "1",
			adults:     "1",
			children:   "-1",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "children 0 with child_ages returns 400 CHILD_AGE_COUNT_MISMATCH",
			rooms:      "1",
			adults:     "1",
			children:   "0",
			childAges:  "5",
			wantStatus: http.StatusBadRequest,
			wantCode:   "CHILD_AGE_COUNT_MISMATCH",
		},
		{
			name:       "child age > 17 returns 400 INVALID_CHILD_AGE",
			rooms:      "1",
			adults:     "1",
			children:   "1",
			childAges:  "18",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_CHILD_AGE",
		},
		{
			name:       "mismatched child_ages count returns 400 CHILD_AGE_COUNT_MISMATCH",
			rooms:      "1",
			adults:     "1",
			children:   "2",
			childAges:  "5",
			wantStatus: http.StatusBadRequest,
			wantCode:   "CHILD_AGE_COUNT_MISMATCH",
		},
		{
			name:       "valid 2 rooms 2 adults 1 child",
			rooms:      "2",
			adults:     "2",
			children:   "1",
			childAges:  "7",
			wantRooms:  2,
			wantAdults: 2,
			wantKids:   1,
			wantTotal:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gc, err := parseGuestQuery(tc.rooms, tc.adults, tc.children, tc.childAges)
			if tc.wantCode != "" {
				if err == nil {
					t.Fatalf("expected error code %s, got nil", tc.wantCode)
				}
				if err.code != tc.wantCode {
					t.Errorf("code = %s, want %s", err.code, tc.wantCode)
				}
				if err.status != tc.wantStatus {
					t.Errorf("status = %d, want %d", err.status, tc.wantStatus)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gc.Rooms != tc.wantRooms || gc.Adults != tc.wantAdults || gc.Children != tc.wantKids || gc.Total != tc.wantTotal {
				t.Errorf("got %+v, want rooms=%d, adults=%d, kids=%d, total=%d", gc, tc.wantRooms, tc.wantAdults, tc.wantKids, tc.wantTotal)
			}
		})
	}
}

func TestValidateQuoteGuestCounts_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		rooms      int
		guests     int
		adults     int
		children   int
		childAges  []int
		wantTotal  int
		wantStatus int
		wantCode   string
	}{
		{
			name:      "fallback legacy num_guests defaults to 1",
			rooms:     1,
			guests:    0,
			wantTotal: 1,
		},
		{
			name:       "rooms > 8 returns 400 INVALID_ROOM_COUNT",
			rooms:      9,
			guests:     2,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ROOM_COUNT",
		},
		{
			name:       "adults < rooms returns 400 INVALID_GUEST_COUNT",
			rooms:      2,
			adults:     1,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "children < 0 returns 400 INVALID_GUEST_COUNT",
			rooms:      1,
			adults:     1,
			children:   -1,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_GUEST_COUNT",
		},
		{
			name:       "children 0 with ages returns 400 CHILD_AGE_COUNT_MISMATCH",
			rooms:      1,
			adults:     1,
			children:   0,
			childAges:  []int{5},
			wantStatus: http.StatusBadRequest,
			wantCode:   "CHILD_AGE_COUNT_MISMATCH",
		},
		{
			name:       "child age > 17 returns 400 INVALID_CHILD_AGE",
			rooms:      1,
			adults:     1,
			children:   1,
			childAges:  []int{18},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_CHILD_AGE",
		},
		{
			name:       "children count mismatch returns 400 CHILD_AGE_COUNT_MISMATCH",
			rooms:      1,
			adults:     1,
			children:   2,
			childAges:  []int{5},
			wantStatus: http.StatusBadRequest,
			wantCode:   "CHILD_AGE_COUNT_MISMATCH",
		},
		{
			name:      "valid structured guests",
			rooms:     2,
			adults:    2,
			children:  2,
			childAges: []int{4, 8},
			wantTotal: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gc, err := validateQuoteGuestCounts(tc.rooms, tc.guests, tc.adults, tc.children, tc.childAges)
			if tc.wantCode != "" {
				if err == nil {
					t.Fatalf("expected error code %s, got nil", tc.wantCode)
				}
				if err.code != tc.wantCode {
					t.Errorf("code = %s, want %s", err.code, tc.wantCode)
				}
				if err.Error() == "" {
					t.Errorf("expected non-empty Error()")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gc.Total != tc.wantTotal {
				t.Errorf("Total = %d, want %d", gc.Total, tc.wantTotal)
			}
		})
	}
}
