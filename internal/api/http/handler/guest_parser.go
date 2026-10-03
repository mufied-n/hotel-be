package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	maxRooms           = 8
	maxChildAge        = 17
	maxStayNights      = 30
	bookingHorizonDays = 365
)

// GuestCounts merepresentasikan hasil parsing jumlah tamu dan kamar (FR-13).
type GuestCounts struct {
	Rooms     int
	Adults    int
	Children  int
	ChildAges []int
	Total     int
}

type guestValidationErr struct {
	status int
	code   string
	msg    string
}

func (e *guestValidationErr) Error() string { return e.msg }

// parseGuestQuery memvalidasi query parameter pencarian kamar (FR-13).
func parseGuestQuery(roomsStr, adultsStr, childrenStr, childAgesStr string) (GuestCounts, *guestValidationErr) {
	rooms := 1
	if roomsStr != "" {
		r, err := strconv.Atoi(roomsStr)
		if err != nil || r < 1 || r > maxRooms {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_ROOM_COUNT",
				msg:    fmt.Sprintf("rooms must be between 1 and %d", maxRooms),
			}
		}
		rooms = r
	}

	adults := 1
	if adultsStr != "" {
		a, err := strconv.Atoi(adultsStr)
		if err != nil || a < 1 {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_GUEST_COUNT",
				msg:    "adults must be at least 1",
			}
		}
		adults = a
	}

	if adults < rooms {
		return GuestCounts{}, &guestValidationErr{
			status: http.StatusBadRequest,
			code:   "INVALID_GUEST_COUNT",
			msg:    "adults count must be greater than or equal to rooms count",
		}
	}

	children := 0
	hasChildrenParam := childrenStr != ""
	if hasChildrenParam {
		c, err := strconv.Atoi(childrenStr)
		if err != nil || c < 0 {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_GUEST_COUNT",
				msg:    "children cannot be negative",
			}
		}
		children = c
	}

	if hasChildrenParam && children == 0 && childAgesStr != "" {
		return GuestCounts{}, &guestValidationErr{
			status: http.StatusBadRequest,
			code:   "CHILD_AGE_COUNT_MISMATCH",
			msg:    "child_ages cannot be provided when children is 0",
		}
	}

	var childAges []int
	if childAgesStr != "" {
		for _, ageStr := range strings.Split(childAgesStr, ",") {
			trimmed := strings.TrimSpace(ageStr)
			if trimmed == "" {
				continue
			}
			age, err := strconv.Atoi(trimmed)
			if err != nil || age < 0 || age > maxChildAge {
				return GuestCounts{}, &guestValidationErr{
					status: http.StatusBadRequest,
					code:   "INVALID_CHILD_AGE",
					msg:    fmt.Sprintf("child age must be between 0 and %d", maxChildAge),
				}
			}
			childAges = append(childAges, age)
		}
		if hasChildrenParam && len(childAges) != children {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "CHILD_AGE_COUNT_MISMATCH",
				msg:    "child_ages count must match children count",
			}
		}
		if !hasChildrenParam {
			children = len(childAges)
		}
	} else if hasChildrenParam && children > 0 {
		return GuestCounts{}, &guestValidationErr{
			status: http.StatusBadRequest,
			code:   "CHILD_AGE_COUNT_MISMATCH",
			msg:    "child_ages count must match children count",
		}
	}

	return GuestCounts{
		Rooms:     rooms,
		Adults:    adults,
		Children:  children,
		ChildAges: childAges,
		Total:     adults + children,
	}, nil
}

// validateQuoteGuestCounts memvalidasi jumlah tamu dan kamar pada DTO quote (FR-13).
func validateQuoteGuestCounts(numRooms, numGuests, adults, children int, childAges []int) (GuestCounts, *guestValidationErr) {
	if numRooms <= 0 {
		numRooms = 1
	}
	if numRooms > maxRooms {
		return GuestCounts{}, &guestValidationErr{
			status: http.StatusBadRequest,
			code:   "INVALID_ROOM_COUNT",
			msg:    fmt.Sprintf("rooms must be between 1 and %d", maxRooms),
		}
	}

	if adults > 0 || children > 0 || len(childAges) > 0 {
		if adults < numRooms {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_GUEST_COUNT",
				msg:    "adults count must be greater than or equal to rooms count",
			}
		}
		if children < 0 {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_GUEST_COUNT",
				msg:    "children cannot be negative",
			}
		}
		if children == 0 && len(childAges) > 0 {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "CHILD_AGE_COUNT_MISMATCH",
				msg:    "child_ages cannot be provided when children is 0",
			}
		}
		if children > 0 {
			if len(childAges) != children {
				return GuestCounts{}, &guestValidationErr{
					status: http.StatusBadRequest,
					code:   "CHILD_AGE_COUNT_MISMATCH",
					msg:    "child_ages count must match children count",
				}
			}
			for _, age := range childAges {
				if age < 0 || age > maxChildAge {
					return GuestCounts{}, &guestValidationErr{
						status: http.StatusBadRequest,
						code:   "INVALID_CHILD_AGE",
						msg:    fmt.Sprintf("child age must be between 0 and %d", maxChildAge),
					}
				}
			}
		}
		numGuests = adults + children
	} else {
		if numGuests < 1 {
			numGuests = 1
		}
		if numGuests < numRooms {
			return GuestCounts{}, &guestValidationErr{
				status: http.StatusBadRequest,
				code:   "INVALID_GUEST_COUNT",
				msg:    "guests count must be greater than or equal to rooms count",
			}
		}
	}

	return GuestCounts{
		Rooms:     numRooms,
		Adults:    adults,
		Children:  children,
		ChildAges: childAges,
		Total:     numGuests,
	}, nil
}
