package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/assistance"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/platform/featureflag"
)

type mockAssistanceService struct {
	createGuestRequestFunc func(ctx context.Context, guestEmail string, input assistance.CreateRequestInput) (*assistance.SpecialRequest, error)
	listGuestRequestsFunc  func(ctx context.Context, guestEmail, bookingID string) ([]assistance.SpecialRequest, error)
	listStaffQueueFunc     func(ctx context.Context, filter assistance.ListFilter) ([]assistance.StaffQueueItem, error)
	updateStatusFunc       func(ctx context.Context, input assistance.UpdateStatusInput) (*assistance.SpecialRequest, error)
}

func (m *mockAssistanceService) CreateGuestRequest(ctx context.Context, guestEmail string, input assistance.CreateRequestInput) (*assistance.SpecialRequest, error) {
	if m.createGuestRequestFunc != nil {
		return m.createGuestRequestFunc(ctx, guestEmail, input)
	}
	return nil, nil
}

func (m *mockAssistanceService) ListGuestRequests(ctx context.Context, guestEmail, bookingID string) ([]assistance.SpecialRequest, error) {
	if m.listGuestRequestsFunc != nil {
		return m.listGuestRequestsFunc(ctx, guestEmail, bookingID)
	}
	return nil, nil
}

func (m *mockAssistanceService) ListStaffQueue(ctx context.Context, filter assistance.ListFilter) ([]assistance.StaffQueueItem, error) {
	if m.listStaffQueueFunc != nil {
		return m.listStaffQueueFunc(ctx, filter)
	}
	return nil, nil
}

func (m *mockAssistanceService) UpdateStatus(ctx context.Context, input assistance.UpdateStatusInput) (*assistance.SpecialRequest, error) {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, input)
	}
	return nil, nil
}

func TestAssistanceAPI_CreateGuestRequest_TableTest(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name           string
		session        *guest.GuestSession
		serviceNil     bool
		bookingID      string
		body           string
		mockRes        *assistance.SpecialRequest
		mockErr        error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:       "Success - 201 Created",
			session:    &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:  "01900000-0000-7000-8000-000000000001",
			body:       `{"category":"celebration_setup","description":"Anniversary dekorasi","target_time":"15:00"}`,
			mockRes:    &assistance.SpecialRequest{ID: "req-1", BookingID: "01900000-0000-7000-8000-000000000001", Category: assistance.CategoryCelebrationSetup, Status: assistance.StatusPending, CreatedAt: now},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Fail - Service Unavailable",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			serviceNil:     true,
			bookingID:      "bk-1",
			body:           `{"category":"celebration_setup"}`,
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "SERVICE_UNAVAILABLE",
		},
		{
			name:           "Fail - Unauthorized session missing",
			session:        nil,
			bookingID:      "bk-1",
			body:           `{"category":"celebration_setup"}`,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
		},
		{
			name:           "Fail - Invalid JSON body",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-1",
			body:           `{invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_JSON",
		},
		{
			name:           "Fail - Booking Not Found (Anti-IDOR)",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-unknown",
			body:           `{"category":"baby_crib","description":"Boks bayi"}`,
			mockErr:        assistance.ErrBookingNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "BOOKING_NOT_FOUND",
		},
		{
			name:           "Fail - Invalid Category",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-1",
			body:           `{"category":"bogus","description":"Boks bayi"}`,
			mockErr:        assistance.ErrInvalidCategory,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_CATEGORY",
		},
		{
			name:           "Fail - Empty Description",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-1",
			body:           `{"category":"baby_crib","description":""}`,
			mockErr:        assistance.ErrEmptyDescription,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "EMPTY_DESCRIPTION",
		},
		{
			name:           "Fail - Unexpected internal error",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-1",
			body:           `{"category":"baby_crib","description":"Boks bayi"}`,
			mockErr:        errors.New("db crash"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{}
			if !tt.serviceNil {
				deps.AssistanceSvc = &mockAssistanceService{
					createGuestRequestFunc: func(ctx context.Context, guestEmail string, input assistance.CreateRequestInput) (*assistance.SpecialRequest, error) {
						return tt.mockRes, tt.mockErr
					},
				}
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/api/v1/guest/bookings/:id/special-requests", handleCreateGuestSpecialRequest(deps))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/guest/bookings/"+tt.bookingID+"/special-requests", bytes.NewBufferString(tt.body))
			if tt.session != nil {
				ctx := context.WithValue(req.Context(), guestSessionContextKey, tt.session)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}

			if tt.expectedCode != "" {
				var errResp map[string]any
				_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
				if errResp["code"] != tt.expectedCode {
					t.Errorf("expected error code %s, got %v", tt.expectedCode, errResp["code"])
				}
			}
		})
	}
}

func TestAssistanceAPI_ListGuestRequests_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		session        *guest.GuestSession
		serviceNil     bool
		bookingID      string
		mockRes        []assistance.SpecialRequest
		mockErr        error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:       "Success - 200 OK",
			session:    &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:  "bk-123",
			mockRes:    []assistance.SpecialRequest{{ID: "req-1", Category: assistance.CategoryCelebrationSetup}},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Fail - Service Unavailable",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			serviceNil:     true,
			bookingID:      "bk-123",
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "SERVICE_UNAVAILABLE",
		},
		{
			name:           "Fail - Unauthorized session missing",
			session:        nil,
			bookingID:      "bk-123",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
		},
		{
			name:           "Fail - Booking Not Found (Anti-IDOR)",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-123",
			mockErr:        assistance.ErrBookingNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "BOOKING_NOT_FOUND",
		},
		{
			name:           "Fail - Internal Error",
			session:        &guest.GuestSession{GuestEmail: "guest@example.com"},
			bookingID:      "bk-123",
			mockErr:        errors.New("db error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{}
			if !tt.serviceNil {
				deps.AssistanceSvc = &mockAssistanceService{
					listGuestRequestsFunc: func(ctx context.Context, guestEmail, bookingID string) ([]assistance.SpecialRequest, error) {
						return tt.mockRes, tt.mockErr
					},
				}
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.GET("/api/v1/guest/bookings/:id/special-requests", handleListGuestSpecialRequests(deps))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/"+tt.bookingID+"/special-requests", nil)
			if tt.session != nil {
				ctx := context.WithValue(req.Context(), guestSessionContextKey, tt.session)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAssistanceAPI_ListStaffSpecialRequests_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		serviceNil     bool
		mockRes        []assistance.StaffQueueItem
		mockErr        error
		expectedStatus int
	}{
		{
			name: "Success - 200 OK",
			mockRes: []assistance.StaffQueueItem{
				{SpecialRequest: assistance.SpecialRequest{ID: "req-1"}, GuestName: "Budi"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Fail - Service Unavailable",
			serviceNil:     true,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "Fail - Internal Error",
			mockErr:        errors.New("query error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{}
			if !tt.serviceNil {
				deps.AssistanceSvc = &mockAssistanceService{
					listStaffQueueFunc: func(ctx context.Context, filter assistance.ListFilter) ([]assistance.StaffQueueItem, error) {
						return tt.mockRes, tt.mockErr
					},
				}
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.GET("/api/v1/front-desk/special-requests", handleListStaffSpecialRequests(deps))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/front-desk/special-requests?department=housekeeping", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

func TestAssistanceAPI_UpdateStaffSpecialRequestStatus_TableTest(t *testing.T) {
	tests := []struct {
		name           string
		serviceNil     bool
		reqID          string
		body           string
		authRole       string
		mockRes        *assistance.SpecialRequest
		mockErr        error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:       "Success - 200 OK",
			reqID:      "req-1",
			body:       `{"to_status":"fulfilled","staff_notes":"Selesai disiapkan di kamar"}`,
			authRole:   "housekeeping",
			mockRes:    &assistance.SpecialRequest{ID: "req-1", Status: assistance.StatusFulfilled},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Fail - Service Unavailable",
			serviceNil:     true,
			reqID:          "req-1",
			body:           `{"to_status":"fulfilled"}`,
			expectedStatus: http.StatusServiceUnavailable,
			expectedCode:   "SERVICE_UNAVAILABLE",
		},
		{
			name:           "Fail - Invalid JSON",
			reqID:          "req-1",
			body:           `{invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_JSON",
		},
		{
			name:           "Fail - Request Not Found",
			reqID:          "req-missing",
			body:           `{"to_status":"acknowledged"}`,
			mockErr:        assistance.ErrRequestNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "REQUEST_NOT_FOUND",
		},
		{
			name:           "Fail - Invalid Status Transition",
			reqID:          "req-1",
			body:           `{"to_status":"pending"}`,
			mockErr:        assistance.ErrInvalidStatusTransition,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_STATUS_TRANSITION",
		},
		{
			name:           "Fail - Staff Notes Required",
			reqID:          "req-1",
			body:           `{"to_status":"declined","staff_notes":""}`,
			mockErr:        assistance.ErrStaffNotesRequired,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "STAFF_NOTES_REQUIRED",
		},
		{
			name:           "Fail - Internal Error",
			reqID:          "req-1",
			body:           `{"to_status":"fulfilled"}`,
			mockErr:        errors.New("db error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{}
			if !tt.serviceNil {
				deps.AssistanceSvc = &mockAssistanceService{
					updateStatusFunc: func(ctx context.Context, input assistance.UpdateStatusInput) (*assistance.SpecialRequest, error) {
						return tt.mockRes, tt.mockErr
					},
				}
			}

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.PUT("/api/v1/front-desk/special-requests/:id/status", handleUpdateStaffSpecialRequestStatus(deps))

			req := httptest.NewRequest(http.MethodPut, "/api/v1/front-desk/special-requests/"+tt.reqID+"/status", bytes.NewBufferString(tt.body))
			if tt.authRole != "" {
				ctx := context.WithValue(req.Context(), RoleKey, tt.authRole)
				ctx = context.WithValue(ctx, SubjectKey, "staff_1")
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAssistanceAPI_FeatureFlag_Guard(t *testing.T) {
	// Feature Flag disabled
	ff := featureflag.NewMemoryManager(map[string]featureflag.Flag{
		"ff_guest_special_requests": {
			Key:     "ff_guest_special_requests",
			Enabled: false,
		},
	})

	mockGuest := &mockGuestService{
		validateSessionFunc: func(ctx context.Context, rawToken string) (*guest.GuestSession, error) {
			return &guest.GuestSession{GuestEmail: "budi@example.com"}, nil
		},
	}

	deps := Deps{
		AssistanceSvc: &mockAssistanceService{},
		GuestSvc:      mockGuest,
		FeatureFlag:   ff,
	}

	r := NewRouter(deps)

	// Test Guest Endpoint with feature disabled
	reqGuest := httptest.NewRequest(http.MethodPost, "/api/v1/guest/bookings/bk-1/special-requests", bytes.NewBufferString(`{}`))
	reqGuest.Header.Set("Authorization", "Bearer gst_sess_123")
	recGuest := httptest.NewRecorder()
	r.ServeHTTP(recGuest, reqGuest)

	if recGuest.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for disabled feature on guest endpoint, got %d", recGuest.Code)
	}

	// Test Staff Endpoint with feature disabled
	reqStaff := httptest.NewRequest(http.MethodGet, "/api/v1/front-desk/special-requests", nil)
	reqStaff.Header.Set("Authorization", "Bearer receptionist")
	recStaff := httptest.NewRecorder()
	r.ServeHTTP(recStaff, reqStaff)

	if recStaff.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for disabled feature on staff endpoint, got %d", recStaff.Code)
	}
}
