package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/stay"
)

type mockStayService struct {
	moveRoomFunc      func(ctx context.Context, input stay.RoomMoveInput) (*stay.RoomMoveResult, error)
	extendStayFunc    func(ctx context.Context, input stay.ExtendStayInput) (*stay.ExtendStayResult, error)
	listRoomMovesFunc func(ctx context.Context, bookingID string) ([]stay.RoomMoveLog, error)
}

func (m *mockStayService) MoveRoom(ctx context.Context, input stay.RoomMoveInput) (*stay.RoomMoveResult, error) {
	if m.moveRoomFunc != nil {
		return m.moveRoomFunc(ctx, input)
	}
	return nil, nil
}

func (m *mockStayService) ExtendStay(ctx context.Context, input stay.ExtendStayInput) (*stay.ExtendStayResult, error) {
	if m.extendStayFunc != nil {
		return m.extendStayFunc(ctx, input)
	}
	return nil, nil
}

func (m *mockStayService) ListRoomMoves(ctx context.Context, bookingID string) ([]stay.RoomMoveLog, error) {
	if m.listRoomMovesFunc != nil {
		return m.listRoomMovesFunc(ctx, bookingID)
	}
	return nil, nil
}

func TestStayAPI_RoomMove_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		bookingID      string
		body           string
		mockRes        *stay.RoomMoveResult
		mockErr        error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:      "Valid room move returns 200 OK",
			bookingID: "bk-123",
			body:      `{"target_room_number":"305","reason_category":"maintenance_defect","notes":"AC broken"}`,
			mockRes: &stay.RoomMoveResult{
				Status:             "ok",
				BookingID:          "bk-123",
				PreviousRoomNumber: "101",
				NewRoomNumber:      "305",
				MoveDate:           "2026-10-03",
				Message:            "pemindahan kamar berhasil",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid JSON body returns 400 Bad Request",
			bookingID:      "bk-123",
			body:           `{invalid-json`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_JSON",
		},
		{
			name:           "Booking not found returns 404 Not Found",
			bookingID:      "bk-not-found",
			body:           `{"target_room_number":"305","reason_category":"maintenance_defect"}`,
			mockErr:        stay.ErrBookingNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "BOOKING_NOT_FOUND",
		},
		{
			name:           "Non-checked-in booking returns 409 Conflict",
			bookingID:      "bk-pending",
			body:           `{"target_room_number":"305","reason_category":"maintenance_defect"}`,
			mockErr:        stay.ErrInvalidBookingStatus,
			expectedStatus: http.StatusConflict,
			expectedCode:   "INVALID_BOOKING_STATUS",
		},
		{
			name:           "Same room move returns 400 Bad Request",
			bookingID:      "bk-123",
			body:           `{"target_room_number":"101","reason_category":"maintenance_defect"}`,
			mockErr:        stay.ErrSameRoomMove,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_INPUT",
		},
		{
			name:           "Invalid reason category returns 400 Bad Request",
			bookingID:      "bk-123",
			body:           `{"target_room_number":"305","reason_category":"not_a_reason"}`,
			mockErr:        stay.ErrInvalidReasonCategory,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_REASON",
		},
		{
			name:           "Target room not ready returns 409 Conflict",
			bookingID:      "bk-123",
			body:           `{"target_room_number":"305","reason_category":"maintenance_defect"}`,
			mockErr:        stay.ErrTargetRoomNotReady,
			expectedStatus: http.StatusConflict,
			expectedCode:   "TARGET_ROOM_NOT_READY",
		},
		{
			name:           "Room physical overlap returns 409 Conflict",
			bookingID:      "bk-123",
			body:           `{"target_room_number":"305","reason_category":"maintenance_defect"}`,
			mockErr:        stay.ErrRoomPhysicalOverlap,
			expectedStatus: http.StatusConflict,
			expectedCode:   "ROOM_PHYSICAL_OVERLAP",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockStayService{
				moveRoomFunc: func(ctx context.Context, input stay.RoomMoveInput) (*stay.RoomMoveResult, error) {
					if tc.mockErr != nil {
						return nil, tc.mockErr
					}
					return tc.mockRes, nil
				},
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/bookings/:id/room-move", handleRoomMove(Deps{StaySvc: svc}))

			req := httptest.NewRequest(http.MethodPost, "/bookings/"+tc.bookingID+"/room-move", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tc.expectedStatus, w.Code, w.Body.String())
			}

			if tc.expectedCode != "" {
				var errResp map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &errResp)
				if errResp["code"] != tc.expectedCode {
					t.Errorf("expected error code %s, got %v", tc.expectedCode, errResp["code"])
				}
			}
		})
	}
}

func TestStayAPI_ExtendStay_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		bookingID      string
		body           string
		mockRes        *stay.ExtendStayResult
		mockErr        error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:      "Valid extend stay returns 200 OK",
			bookingID: "bk-123",
			body:      `{"additional_nights":2,"payment_method":"front_desk_edc"}`,
			mockRes: &stay.ExtendStayResult{
				Status:                "ok",
				BookingID:             "bk-123",
				PreviousCheckOut:      "2026-10-05",
				NewCheckOut:           "2026-10-07",
				AdditionalNights:      2,
				AdditionalAmountMinor: 1500000,
				NewTotalPriceMinor:    3000000,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid additional nights returns 400 Bad Request",
			bookingID:      "bk-123",
			body:           `{"additional_nights":0}`,
			mockErr:        stay.ErrInvalidAdditionalNights,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_ADDITIONAL_NIGHTS",
		},
		{
			name:           "Booking not found returns 404 Not Found",
			bookingID:      "bk-not-found",
			body:           `{"additional_nights":1}`,
			mockErr:        stay.ErrBookingNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "BOOKING_NOT_FOUND",
		},
		{
			name:           "No availability for extension returns 409 Conflict",
			bookingID:      "bk-123",
			body:           `{"additional_nights":1}`,
			mockErr:        stay.ErrNoAvailabilityForExtension,
			expectedStatus: http.StatusConflict,
			expectedCode:   "NO_AVAILABILITY_FOR_EXTENSION",
		},
		{
			name:           "Room physical overlap returns 409 Conflict",
			bookingID:      "bk-123",
			body:           `{"additional_nights":1}`,
			mockErr:        stay.ErrRoomPhysicalOverlap,
			expectedStatus: http.StatusConflict,
			expectedCode:   "ROOM_PHYSICAL_OVERLAP",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockStayService{
				extendStayFunc: func(ctx context.Context, input stay.ExtendStayInput) (*stay.ExtendStayResult, error) {
					if tc.mockErr != nil {
						return nil, tc.mockErr
					}
					return tc.mockRes, nil
				},
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/bookings/:id/extend-stay", handleExtendStay(Deps{StaySvc: svc}))

			req := httptest.NewRequest(http.MethodPost, "/bookings/"+tc.bookingID+"/extend-stay", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tc.expectedStatus, w.Code, w.Body.String())
			}

			if tc.expectedCode != "" {
				var errResp map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &errResp)
				if errResp["code"] != tc.expectedCode {
					t.Errorf("expected error code %s, got %v", tc.expectedCode, errResp["code"])
				}
			}
		})
	}
}

func TestStayAPI_ListRoomMoves(t *testing.T) {
	svc := &mockStayService{
		listRoomMovesFunc: func(ctx context.Context, bookingID string) ([]stay.RoomMoveLog, error) {
			if bookingID == "bk-123" {
				return []stay.RoomMoveLog{
					{
						ID:             "move-1",
						BookingID:      "bk-123",
						FromRoomNumber: "101",
						ToRoomNumber:   "305",
						MoveDate:       "2026-10-03",
						ReasonCategory: "maintenance_defect",
						Notes:          "AC broken",
						CreatedAt:      time.Now().UTC(),
					},
				}, nil
			}
			return nil, stay.ErrBookingNotFound
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/bookings/:id/room-moves", handleListRoomMoves(Deps{StaySvc: svc}))

	// Positive test
	req := httptest.NewRequest(http.MethodGet, "/bookings/bk-123/room-moves", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// Negative test: 404
	req404 := httptest.NewRequest(http.MethodGet, "/bookings/bk-999/room-moves", nil)
	w404 := httptest.NewRecorder()
	r.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w404.Code)
	}
}
