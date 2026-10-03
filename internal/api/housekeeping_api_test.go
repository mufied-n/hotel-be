package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/housekeeping"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
)

type mockHousekeepingService struct {
	getRoomBoardFunc           func(ctx context.Context, floor int, status string, roomTypeID string) (*housekeeping.RoomBoardSummary, error)
	updateStatusFunc           func(ctx context.Context, input housekeeping.UpdateStatusInput) error
	validateRoomForCheckInFunc func(ctx context.Context, roomNumber string) error
	markRoomDirtyOnCheckOutFunc func(ctx context.Context, roomNumber string) error
	markRoomOutOfOrderFunc     func(ctx context.Context, roomNumber string, startDate, endDate time.Time, reason string) error
}

func (m *mockHousekeepingService) GetRoomBoard(ctx context.Context, floor int, status string, roomTypeID string) (*housekeeping.RoomBoardSummary, error) {
	if m.getRoomBoardFunc != nil {
		return m.getRoomBoardFunc(ctx, floor, status, roomTypeID)
	}
	return &housekeeping.RoomBoardSummary{
		TotalRooms: 95,
		Summary: map[housekeeping.CleanlinessStatus]int{
			housekeeping.StatusVacantDirty:  5,
			housekeeping.StatusCleaning:     3,
			housekeeping.StatusVacantClean:  7,
			housekeeping.StatusInspected:    40,
			housekeeping.StatusOccupied:     38,
			housekeeping.StatusOutOfService: 1,
			housekeeping.StatusOutOfOrder:   1,
		},
		Rooms: []housekeeping.RoomOperationalView{
			{
				RoomNumber:        "201",
				RoomTypeID:        "01900000-0000-7000-8000-000000000001",
				RoomTypeName:      "Superior King",
				Floor:             2,
				CleanlinessStatus: housekeeping.StatusInspected,
				UpdatedAt:         time.Now().UTC(),
				UpdatedBy:         "system",
			},
		},
	}, nil
}

func (m *mockHousekeepingService) UpdateStatus(ctx context.Context, input housekeeping.UpdateStatusInput) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, input)
	}
	return nil
}

func (m *mockHousekeepingService) ValidateRoomForCheckIn(ctx context.Context, roomNumber string) error {
	if m.validateRoomForCheckInFunc != nil {
		return m.validateRoomForCheckInFunc(ctx, roomNumber)
	}
	return nil
}

func (m *mockHousekeepingService) MarkRoomDirtyOnCheckOut(ctx context.Context, roomNumber string) error {
	if m.markRoomDirtyOnCheckOutFunc != nil {
		return m.markRoomDirtyOnCheckOutFunc(ctx, roomNumber)
	}
	return nil
}

func (m *mockHousekeepingService) MarkRoomOutOfOrder(ctx context.Context, roomNumber string, startDate, endDate time.Time, reason string) error {
	if m.markRoomOutOfOrderFunc != nil {
		return m.markRoomOutOfOrderFunc(ctx, roomNumber, startDate, endDate, reason)
	}
	return nil
}

var _ housekeeping.Service = (*mockHousekeepingService)(nil)

func setupHousekeepingTestRouter(t *testing.T, hkSvc housekeeping.Service, bkSvc *booking.Service) http.Handler {
	t.Helper()

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "housekeeping", "/api/v1/housekeeping/rooms", "GET"},
		{"p", "housekeeping", "/api/v1/housekeeping/rooms/:id/status", "PUT"},
		{"p", "receptionist", "/api/v1/housekeeping/rooms", "GET"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "gm_admin", "/api/v1/housekeeping/*", "*"},
		{"p", "gm_admin", "/api/v1/bookings/*", "*"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create in-memory enforcer: %v", err)
	}

	return NewRouter(Deps{
		Enforcer:        enforcer,
		HousekeepingSvc: hkSvc,
		BookingSvc:      bkSvc,
	})
}

func TestHousekeepingAPI_GetRooms_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		url        string
		setupMock  func(m *mockHousekeepingService)
		expectCode int
		expectJSON string
	}{
		{
			name:       "Housekeeping role can view room board",
			authHeader: "Bearer housekeeping",
			url:        "/api/v1/housekeeping/rooms",
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"total_rooms":95`,
		},
		{
			name:       "Receptionist role can view room board",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/housekeeping/rooms?floor=2&status=inspected",
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"inspected":40`,
		},
		{
			name:       "Filter with synonym dirty",
			authHeader: "Bearer housekeeping",
			url:        "/api/v1/housekeeping/rooms?status=dirty",
			setupMock: func(m *mockHousekeepingService) {
				m.getRoomBoardFunc = func(ctx context.Context, floor int, status string, roomTypeID string) (*housekeeping.RoomBoardSummary, error) {
					if status != "vacant_dirty" {
						t.Errorf("expected normalized status 'vacant_dirty', got %q", status)
					}
					return &housekeeping.RoomBoardSummary{TotalRooms: 5}, nil
				}
			},
			expectCode: http.StatusOK,
			expectJSON: `"total_rooms":5`,
		},
		{
			name:       "Guest role is forbidden from viewing housekeeping board",
			authHeader: "Bearer guest",
			url:        "/api/v1/housekeeping/rooms",
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Internal error in store returns 500",
			authHeader: "Bearer housekeeping",
			url:        "/api/v1/housekeeping/rooms",
			setupMock: func(m *mockHousekeepingService) {
				m.getRoomBoardFunc = func(ctx context.Context, floor int, status string, roomTypeID string) (*housekeeping.RoomBoardSummary, error) {
					return nil, context.DeadlineExceeded
				}
			},
			expectCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockHousekeepingService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}
			router := setupHousekeepingTestRouter(t, mockSvc, nil)

			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d, got %d, body: %s", tc.expectCode, w.Code, w.Body.String())
			}
			if tc.expectJSON != "" && !bytes.Contains(w.Body.Bytes(), []byte(tc.expectJSON)) {
				t.Errorf("expected response to contain %q, got: %s", tc.expectJSON, w.Body.String())
			}
		})
	}
}

func TestHousekeepingAPI_UpdateStatus_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		roomNumber string
		body       string
		setupMock  func(m *mockHousekeepingService)
		expectCode int
		expectJSON string
	}{
		{
			name:       "Housekeeping update dirty to cleaning",
			authHeader: "Bearer housekeeping",
			roomNumber: "201",
			body:       `{"to_status":"cleaning","notes":"Started cleaning"}`,
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"cleanliness_status":"cleaning"`,
		},
		{
			name:       "Housekeeping update cleaning to clean (synonym)",
			authHeader: "Bearer housekeeping",
			roomNumber: "201",
			body:       `{"to_status":"clean","notes":"Linen changed"}`,
			setupMock: func(m *mockHousekeepingService) {
				m.updateStatusFunc = func(ctx context.Context, input housekeeping.UpdateStatusInput) error {
					if input.ToStatus != housekeeping.StatusVacantClean {
						t.Errorf("expected normalized status vacant_clean, got %s", input.ToStatus)
					}
					return nil
				}
			},
			expectCode: http.StatusOK,
			expectJSON: `"cleanliness_status":"vacant_clean"`,
		},
		{
			name:       "Illegal transition returns 409 Conflict",
			authHeader: "Bearer housekeeping",
			roomNumber: "201",
			body:       `{"to_status":"inspected","notes":"Bypassing clean"}`,
			setupMock: func(m *mockHousekeepingService) {
				m.updateStatusFunc = func(ctx context.Context, input housekeeping.UpdateStatusInput) error {
					return housekeeping.ErrInvalidTransition
				}
			},
			expectCode: http.StatusConflict,
			expectJSON: "INVALID_STATUS_TRANSITION",
		},
		{
			name:       "Room not found returns 404",
			authHeader: "Bearer housekeeping",
			roomNumber: "999",
			body:       `{"to_status":"cleaning"}`,
			setupMock: func(m *mockHousekeepingService) {
				m.updateStatusFunc = func(ctx context.Context, input housekeeping.UpdateStatusInput) error {
					return housekeeping.ErrRoomNotFound
				}
			},
			expectCode: http.StatusNotFound,
			expectJSON: "ROOM_NOT_FOUND",
		},
		{
			name:       "Housekeeping role setting out_of_order returns 403 Forbidden",
			authHeader: "Bearer housekeeping",
			roomNumber: "201",
			body:       `{"to_status":"out_of_order","notes":"Broken AC"}`,
			setupMock: func(m *mockHousekeepingService) {
				m.updateStatusFunc = func(ctx context.Context, input housekeeping.UpdateStatusInput) error {
					return housekeeping.ErrUnauthorizedTransition
				}
			},
			expectCode: http.StatusForbidden,
			expectJSON: "UNAUTHORIZED_TRANSITION",
		},
		{
			name:       "Invalid JSON payload returns 400 Bad Request",
			authHeader: "Bearer housekeeping",
			roomNumber: "201",
			body:       `{invalid-json`,
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
			expectJSON: "INVALID_JSON",
		},
		{
			name:       "Guest role is rejected with 403",
			authHeader: "Bearer guest",
			roomNumber: "201",
			body:       `{"to_status":"cleaning"}`,
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockHousekeepingService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}
			router := setupHousekeepingTestRouter(t, mockSvc, nil)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/housekeeping/rooms/"+tc.roomNumber+"/status", bytes.NewBufferString(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d, got %d, body: %s", tc.expectCode, w.Code, w.Body.String())
			}
			if tc.expectJSON != "" && !bytes.Contains(w.Body.Bytes(), []byte(tc.expectJSON)) {
				t.Errorf("expected response to contain %q, got: %s", tc.expectJSON, w.Body.String())
			}
		})
	}
}

func TestHousekeepingAPI_OutOfOrder_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		roomNumber string
		body       string
		setupMock  func(m *mockHousekeepingService)
		expectCode int
		expectJSON string
	}{
		{
			name:       "GM Admin successfully marks room out of order",
			authHeader: "Bearer gm_admin",
			roomNumber: "205",
			body:       `{"start_date":"2026-10-05","end_date":"2026-10-08","reason":"AC Compressor overhaul"}`,
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"inventory_deducted_dates":["2026-10-05","2026-10-06","2026-10-07"]`,
		},
		{
			name:       "Non-GM staff (receptionist) is forbidden from OOO endpoint",
			authHeader: "Bearer receptionist",
			roomNumber: "205",
			body:       `{"start_date":"2026-10-05","end_date":"2026-10-08","reason":"AC broken"}`,
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Invalid date range (start >= end) returns 400 Bad Request",
			authHeader: "Bearer gm_admin",
			roomNumber: "205",
			body:       `{"start_date":"2026-10-08","end_date":"2026-10-05","reason":"AC broken"}`,
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
			expectJSON: "INVALID_DATE_RANGE",
		},
		{
			name:       "Room not found returns 404 Not Found",
			authHeader: "Bearer gm_admin",
			roomNumber: "999",
			body:       `{"start_date":"2026-10-05","end_date":"2026-10-08","reason":"AC broken"}`,
			setupMock: func(m *mockHousekeepingService) {
				m.markRoomOutOfOrderFunc = func(ctx context.Context, roomNumber string, startDate, endDate time.Time, reason string) error {
					return housekeeping.ErrRoomNotFound
				}
			},
			expectCode: http.StatusNotFound,
			expectJSON: "ROOM_NOT_FOUND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockHousekeepingService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}
			router := setupHousekeepingTestRouter(t, mockSvc, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/housekeeping/rooms/"+tc.roomNumber+"/out-of-order", bytes.NewBufferString(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d, got %d, body: %s", tc.expectCode, w.Code, w.Body.String())
			}
			if tc.expectJSON != "" && !bytes.Contains(w.Body.Bytes(), []byte(tc.expectJSON)) {
				t.Errorf("expected response to contain %q, got: %s", tc.expectJSON, w.Body.String())
			}
		})
	}
}

func TestHousekeepingAPI_CheckInReadinessGuard_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		bookingID  string
		checkInErr error
		expectCode int
		expectJSON string
	}{
		{
			name:       "Check-in fails with ROOM_NOT_READY when room is not inspected",
			authHeader: "Bearer receptionist",
			bookingID:  "bk_not_ready_1",
			checkInErr: booking.ErrRoomNotReady,
			expectCode: http.StatusConflict,
			expectJSON: "ROOM_NOT_READY",
		},
		{
			name:       "Check-in fails with NO_ROOM_AVAILABLE when completely full",
			authHeader: "Bearer receptionist",
			bookingID:  "bk_no_room_2",
			checkInErr: booking.ErrNoRoomAvailable,
			expectCode: http.StatusConflict,
			expectJSON: "NO_ROOM_AVAILABLE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Buat runner mock yang mengembalikan checkInErr
			mockRunner := &mockBookingCheckInRunner{err: tc.checkInErr}
			bkSvc := booking.NewService(mockRunner, nil, nil, nil, nil, nil, 30*time.Minute, nil)

			router := setupHousekeepingTestRouter(t, &mockHousekeepingService{}, bkSvc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+tc.bookingID+"/check-in", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d, got %d, body: %s", tc.expectCode, w.Code, w.Body.String())
			}
			if tc.expectJSON != "" && !bytes.Contains(w.Body.Bytes(), []byte(tc.expectJSON)) {
				t.Errorf("expected response to contain %q, got: %s", tc.expectJSON, w.Body.String())
			}
		})
	}
}

type mockBookingCheckInRunner struct {
	err error
}

func (m *mockBookingCheckInRunner) InTx(ctx context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	fake := &mockCheckInTx{err: m.err}
	return fn(fake, fake)
}

type mockCheckInTx struct {
	err error
}

func (m *mockCheckInTx) LockAndDecrement(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	return nil
}
func (m *mockCheckInTx) Increment(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	return nil
}
func (m *mockCheckInTx) InsertBookingWithHold(ctx context.Context, b *booking.Booking, quotes []rates.Quote, holdExpiresAt time.Time) error {
	return nil
}
func (m *mockCheckInTx) GetForUpdate(ctx context.Context, id string) (booking.Booking, error) {
	return booking.Booking{ID: id, Status: booking.StatusConfirmed, CheckIn: time.Now(), CheckOut: time.Now().AddDate(0, 0, 1)}, nil
}
func (m *mockCheckInTx) UpdateStatus(ctx context.Context, id string, to booking.Status) error {
	return nil
}
func (m *mockCheckInTx) PickAndAssignRooms(ctx context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return []string{"201"}, nil
}
func (m *mockCheckInTx) GetRoomAssignments(ctx context.Context, bookingID string) ([]string, error) {
	return []string{"201"}, nil
}
func (m *mockCheckInTx) PublishTx(ctx context.Context, topic string, payload []byte) error {
	return nil
}
