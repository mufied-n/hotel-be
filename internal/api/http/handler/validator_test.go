package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type allValidationTestStruct struct {
	Rooms            int    `json:"rooms" validate:"omitempty,min=1,max=8"`
	Adults           int    `json:"adults" validate:"omitempty,min=1"`
	Children         int    `json:"children" validate:"omitempty,gte=0"`
	ChildAges        int    `json:"child_ages" validate:"omitempty,max=17"`
	RoomTypeID       string `json:"room_type_id" validate:"omitempty,len=5"`
	CheckIn          string `json:"check_in" validate:"omitempty,len=10"`
	CheckOut         string `json:"check_out" validate:"omitempty,len=10"`
	Code             string `json:"code" validate:"omitempty,len=4"`
	AmountMinor      int64  `json:"amount_minor" validate:"omitempty,gt=0"`
	Reason           string `json:"reason" validate:"omitempty,min=5"`
	Action           string `json:"action" validate:"omitempty,min=3"`
	TargetRoomNumber string `json:"target_room_number" validate:"omitempty,min=2"`
	ReasonCategory   string `json:"reason_category" validate:"omitempty,min=3"`
	AdditionalNights int    `json:"additional_nights" validate:"omitempty,gt=0"`
	Shift            string `json:"shift" validate:"omitempty,min=2"`
	ToStatus         string `json:"to_status" validate:"omitempty,min=3"`
	OtherField       string `json:"other_field" validate:"omitempty,email"`
}

func TestValidator_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		input    allValidationTestStruct
		wantCode string
	}{
		{
			name:     "valid struct passes",
			input:    allValidationTestStruct{},
			wantCode: "",
		},
		{
			name:     "rooms invalid",
			input:    allValidationTestStruct{Rooms: 10},
			wantCode: "INVALID_ROOM_COUNT",
		},
		{
			name:     "adults invalid",
			input:    allValidationTestStruct{Adults: -1},
			wantCode: "INVALID_GUEST_COUNT",
		},
		{
			name:     "children invalid",
			input:    allValidationTestStruct{Children: -1},
			wantCode: "INVALID_GUEST_COUNT",
		},
		{
			name:     "child ages invalid",
			input:    allValidationTestStruct{ChildAges: 20},
			wantCode: "INVALID_CHILD_AGE",
		},
		{
			name:     "room type id invalid",
			input:    allValidationTestStruct{RoomTypeID: "a"},
			wantCode: "INVALID_DATE_FORMAT",
		},
		{
			name:     "code invalid",
			input:    allValidationTestStruct{Code: "a"},
			wantCode: "INVALID_ROOM_DATA",
		},
		{
			name:     "amount minor invalid",
			input:    allValidationTestStruct{AmountMinor: -10},
			wantCode: "INVALID_AMOUNT",
		},
		{
			name:     "reason invalid",
			input:    allValidationTestStruct{Reason: "no"},
			wantCode: "REASON_REQUIRED",
		},
		{
			name:     "action invalid",
			input:    allValidationTestStruct{Action: "a"},
			wantCode: "ACTION_REQUIRED",
		},
		{
			name:     "target room number invalid",
			input:    allValidationTestStruct{TargetRoomNumber: "1"},
			wantCode: "INVALID_ROOM_NUMBER",
		},
		{
			name:     "reason category invalid",
			input:    allValidationTestStruct{ReasonCategory: "a"},
			wantCode: "INVALID_REASON_CATEGORY",
		},
		{
			name:     "additional nights invalid",
			input:    allValidationTestStruct{AdditionalNights: -2},
			wantCode: "INVALID_ADDITIONAL_NIGHTS",
		},
		{
			name:     "shift invalid",
			input:    allValidationTestStruct{Shift: "a"},
			wantCode: "INVALID_SHIFT",
		},
		{
			name:     "to status invalid",
			input:    allValidationTestStruct{ToStatus: "a"},
			wantCode: "INVALID_STATUS",
		},
		{
			name:     "other field fallback",
			input:    allValidationTestStruct{OtherField: "not-an-email"},
			wantCode: "INVALID_PAYLOAD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

			valid := validateDTO(c, &tc.input)
			if tc.wantCode == "" {
				if !valid {
					t.Fatalf("expected valid dto, got invalid")
				}
				return
			}

			if valid {
				t.Fatalf("expected invalid dto with code %s, but got valid", tc.wantCode)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}

	// Test non-validation error fallback
	t.Run("non validation error fallback", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		handleValidationError(c, assertErr("generic error"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
