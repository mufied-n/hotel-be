package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/example/hotel-booking/internal/platform/auth"
)

type mockFrontDeskService struct {
	getDailyRosterFunc func(ctx context.Context, targetDate time.Time) (*frontdesk.DailyRoster, error)
	recordHandoverFunc func(ctx context.Context, input frontdesk.RecordHandoverInput) (*frontdesk.HandoverNote, error)
	listHandoversFunc  func(ctx context.Context, limit, offset int) ([]frontdesk.HandoverNote, int, error)
}

func (m *mockFrontDeskService) GetDailyRoster(ctx context.Context, targetDate time.Time) (*frontdesk.DailyRoster, error) {
	if m.getDailyRosterFunc != nil {
		return m.getDailyRosterFunc(ctx, targetDate)
	}
	return &frontdesk.DailyRoster{
		Date: targetDate.Format("2006-01-02"),
		Metrics: frontdesk.RosterMetrics{
			TotalRooms:           95,
			SellableRooms:        94,
			OutOfOrderRooms:      1,
			OccupiedRooms:        65,
			VacantInspectedRooms: 20,
			VacantDirtyRooms:     6,
			CleaningRooms:        3,
			OccupancyRatePercent: 69.15,
		},
		ExpectedArrivals: []frontdesk.ExpectedArrivalItem{
			{
				BookingID:            "bk-arr-001",
				GuestName:            "Rian Ardianto",
				GuestPhone:           "+6281234567890",
				RoomTypeID:           "01900000-0000-7000-8000-000000000001",
				RoomTypeName:         "Superior King",
				NumRooms:             1,
				NumGuests:            2,
				EstimatedArrivalTime: "14:00",
				TotalPriceMinor:      1100000,
			},
		},
		ExpectedDepartures: []frontdesk.ExpectedDepartureItem{
			{
				BookingID:    "bk-dep-001",
				GuestName:    "Siti Nurhaliza",
				RoomNumbers:  []string{"204"},
				CheckInDate:  "2026-10-01",
				CheckOutDate: "2026-10-03",
			},
		},
		InHouseCount: 65,
	}, nil
}

func (m *mockFrontDeskService) RecordHandover(ctx context.Context, input frontdesk.RecordHandoverInput) (*frontdesk.HandoverNote, error) {
	if m.recordHandoverFunc != nil {
		return m.recordHandoverFunc(ctx, input)
	}
	return &frontdesk.HandoverNote{
		ID:             "note-uuid-001",
		Shift:          input.Shift,
		CashFloatMinor: input.CashFloatMinor,
		PendingIssues:  input.PendingIssues,
		VIPGuestNotes:  input.VIPGuestNotes,
		ActorID:        input.ActorID,
		ActorRole:      input.ActorRole,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (m *mockFrontDeskService) ListHandovers(ctx context.Context, limit, offset int) ([]frontdesk.HandoverNote, int, error) {
	if m.listHandoversFunc != nil {
		return m.listHandoversFunc(ctx, limit, offset)
	}
	return []frontdesk.HandoverNote{
		{
			ID:             "note-uuid-001",
			Shift:          frontdesk.ShiftMorning,
			CashFloatMinor: 1500000,
			PendingIssues:  "None",
			ActorID:        "staff:receptionist_01",
			ActorRole:      "receptionist",
			CreatedAt:      time.Now().UTC(),
		},
	}, 1, nil
}

var _ frontdesk.Service = (*mockFrontDeskService)(nil)

func setupFrontDeskTestRouter(t *testing.T, fdSvc frontdesk.Service) http.Handler {
	t.Helper()

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "receptionist", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "receptionist", "/api/v1/front-desk/handover-notes", "GET"},
		{"p", "receptionist", "/api/v1/front-desk/handover-notes", "POST"},
		{"p", "housekeeping", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "revenue_mgr", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "gm_admin", "/api/v1/front-desk/*", "*"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create in-memory enforcer: %v", err)
	}

	return NewRouter(Deps{
		StaffAuth:    TestStaffVerifier(),
		Enforcer:     enforcer,
		FrontDeskSvc: fdSvc,
	})
}

func TestFrontDeskAPI_GetDailyRoster_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		url        string
		setupMock  func(m *mockFrontDeskService)
		expectCode int
		expectJSON string
	}{
		{
			name:       "Receptionist accesses daily roster",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/front-desk/daily-roster",
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"total_rooms":95`,
		},
		{
			name:       "Specific valid date query",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/front-desk/daily-roster?date=2026-10-05",
			setupMock: func(m *mockFrontDeskService) {
				m.getDailyRosterFunc = func(ctx context.Context, targetDate time.Time) (*frontdesk.DailyRoster, error) {
					if targetDate.Format("2006-01-02") != "2026-10-05" {
						t.Errorf("expected 2026-10-05, got %s", targetDate.Format("2006-01-02"))
					}
					return &frontdesk.DailyRoster{Date: "2026-10-05"}, nil
				}
			},
			expectCode: http.StatusOK,
			expectJSON: `"date":"2026-10-05"`,
		},
		{
			name:       "Invalid date query returns 400 Bad Request",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/front-desk/daily-roster?date=not-a-date",
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
			expectJSON: "INVALID_DATE_FORMAT",
		},
		{
			name:       "Housekeeping role can view daily roster",
			authHeader: "Bearer housekeeping",
			url:        "/api/v1/front-desk/daily-roster",
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"in_house_count":65`,
		},
		{
			name:       "Revenue manager role can view daily roster",
			authHeader: "Bearer revenue_mgr",
			url:        "/api/v1/front-desk/daily-roster",
			setupMock:  nil,
			expectCode: http.StatusOK,
			expectJSON: `"occupancy_rate_percent":69.15`,
		},
		{
			name:       "Guest role is rejected with 403 Forbidden",
			authHeader: "Bearer guest",
			url:        "/api/v1/front-desk/daily-roster",
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "Internal store error returns 500",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/front-desk/daily-roster",
			setupMock: func(m *mockFrontDeskService) {
				m.getDailyRosterFunc = func(ctx context.Context, targetDate time.Time) (*frontdesk.DailyRoster, error) {
					return nil, context.DeadlineExceeded
				}
			},
			expectCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockFrontDeskService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}
			router := setupFrontDeskTestRouter(t, mockSvc)

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

func TestFrontDeskAPI_RecordHandover_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		body       string
		setupMock  func(m *mockFrontDeskService)
		expectCode int
		expectJSON string
	}{
		{
			name:       "Receptionist records valid morning shift handover (201 Created)",
			authHeader: "Bearer receptionist",
			body:       `{"shift":"morning","cash_float_minor":1500000,"pending_issues":"None","vip_guest_notes":"VIP guest in 501"}`,
			setupMock:  nil,
			expectCode: http.StatusCreated,
			expectJSON: `"shift":"morning"`,
		},
		{
			name:       "Invalid shift string returns 400 Bad Request",
			authHeader: "Bearer receptionist",
			body:       `{"shift":"twilight","cash_float_minor":1500000}`,
			setupMock: func(m *mockFrontDeskService) {
				m.recordHandoverFunc = func(ctx context.Context, input frontdesk.RecordHandoverInput) (*frontdesk.HandoverNote, error) {
					return nil, frontdesk.ErrInvalidShift
				}
			},
			expectCode: http.StatusBadRequest,
			expectJSON: "INVALID_SHIFT",
		},
		{
			name:       "Malformed JSON payload returns 400 Bad Request",
			authHeader: "Bearer receptionist",
			body:       `{bad-json`,
			setupMock:  nil,
			expectCode: http.StatusBadRequest,
			expectJSON: "INVALID_JSON",
		},
		{
			name:       "Guest role is rejected from recording handover note (403 Forbidden)",
			authHeader: "Bearer guest",
			body:       `{"shift":"morning"}`,
			setupMock:  nil,
			expectCode: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockFrontDeskService{}
			if tc.setupMock != nil {
				tc.setupMock(mockSvc)
			}
			router := setupFrontDeskTestRouter(t, mockSvc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/front-desk/handover-notes", bytes.NewBufferString(tc.body))
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

func TestFrontDeskAPI_ListHandovers_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		url        string
		expectCode int
		expectJSON string
	}{
		{
			name:       "Receptionist lists handover notes (200 OK)",
			authHeader: "Bearer receptionist",
			url:        "/api/v1/front-desk/handover-notes?limit=10&offset=0",
			expectCode: http.StatusOK,
			expectJSON: `"total":1`,
		},
		{
			name:       "GM Admin lists handover notes (200 OK)",
			authHeader: "Bearer gm_admin",
			url:        "/api/v1/front-desk/handover-notes",
			expectCode: http.StatusOK,
			expectJSON: `"shift":"morning"`,
		},
		{
			name:       "Guest role is forbidden from viewing handover notes (403 Forbidden)",
			authHeader: "Bearer guest",
			url:        "/api/v1/front-desk/handover-notes",
			expectCode: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := setupFrontDeskTestRouter(t, &mockFrontDeskService{})

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
