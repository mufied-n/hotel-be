package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/gin-gonic/gin"
)

func setupRBACTestRouter(t *testing.T, initialStatus booking.Status) http.Handler {
	t.Helper()

	if initialStatus == "" {
		initialStatus = booking.StatusConfirmed
	}

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "guest", "/api/v1/bookings", "POST"},
		{"p", "guest", "/api/v1/bookings/:id", "GET"},
		{"p", "guest", "/api/v1/bookings/:id/cancel", "POST"},
		{"p", "guest", "/fake-pay/:ref", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/no-show", "POST"},
		{"p", "housekeeping", "/api/v1/rooms/housekeeping", "GET"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create in-memory enforcer: %v", err)
	}

	now := time.Now()
	testBooking := booking.Booking{
		ID:         "01900000-0000-7000-8000-000000000001",
		Status:     initialStatus,
		RoomTypeID: "01900000-0000-7000-8000-000000000001",
		CheckIn:    now,
		CheckOut:   now.Add(24 * time.Hour),
		NumRooms:   1,
	}

	tx := &mockTx{booking: testBooking, rooms: []string{"301"}}
	txRunner := &testRunner{tx: tx}

	bkSvc := booking.NewService(
		txRunner,
		&mockInvStore{avail: []inventory.Availability{{Date: now, AvailableRooms: 5}}},
		&mockRates{quotes: []rates.Quote{{Date: now, RateMinor: 100_000}}},
		&mockPayment{},
		&mockNotifier{},
		&mockReader{booking: testBooking},
		30*time.Minute,
		nil,
	)

	return NewRouter(Deps{
		StaffAuth:  TestStaffVerifier(),
		BookingSvc: bkSvc,
		InvStore:   &mockInvStore{avail: []inventory.Availability{{Date: now, AvailableRooms: 5}}},
		RateSvc:    &mockRates{quotes: []rates.Quote{{Date: now, RateMinor: 100_000}}},
		Enforcer:   enforcer,
		ReadyCheck: func(ctx context.Context) error { return nil },
		FakePay: func(c *gin.Context) {
			c.Status(http.StatusOK)
		},
	})
}

func TestRBACRouteProtection(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		userRole       string
		bearerToken    string
		initialStatus  booking.Status
		expectedStatus int
	}{
		// Health & Ready endpoints are always public regardless of auth
		{
			name:           "Healthz endpoint is public without credentials",
			method:         http.MethodGet,
			path:           "/healthz",
			userRole:       "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Ready endpoint is public without credentials",
			method:         http.MethodGet,
			path:           "/ready",
			userRole:       "",
			expectedStatus: http.StatusOK,
		},

		// Public guest routes
		{
			name:           "Anonymous user can view availability (defaults to guest)",
			method:         http.MethodGet,
			path:           "/api/v1/availability?room_type_id=rt-1&check_in=2026-10-10&check_out=2026-10-12",
			userRole:       "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Guest role can view booking by ID",
			method:         http.MethodGet,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001",
			userRole:       "guest",
			expectedStatus: http.StatusOK,
		},

		// Unauthorized guest operations (FO endpoints)
		{
			name:           "Guest role is FORBIDDEN from check-in",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in",
			userRole:       "guest",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Anonymous is FORBIDDEN from check-in",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in",
			userRole:       "",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Guest role is FORBIDDEN from check-out",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-out",
			userRole:       "guest",
			initialStatus:  booking.StatusCheckedIn,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Guest role is FORBIDDEN from marking no-show",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/no-show",
			userRole:       "guest",
			expectedStatus: http.StatusForbidden,
		},

		// Receptionist authorized operations
		{
			name:           "Receptionist role can check-in via X-User-Role",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in",
			userRole:       "receptionist",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Receptionist role can check-out via Authorization Bearer token",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-out",
			bearerToken:    "receptionist",
			initialStatus:  booking.StatusCheckedIn,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Receptionist role can mark no-show",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/no-show",
			userRole:       "receptionist",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Receptionist inherits guest access to view booking",
			method:         http.MethodGet,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001",
			userRole:       "receptionist",
			expectedStatus: http.StatusOK,
		},

		// Housekeeping / other roles cannot perform FO tasks
		{
			name:           "Housekeeping is FORBIDDEN from check-in",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in",
			userRole:       "housekeeping",
			expectedStatus: http.StatusForbidden,
		},

		// Super Admin wildcard access
		{
			name:           "General Manager (gm_admin) can check-in",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in",
			userRole:       "gm_admin",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "General Manager (gm_admin) can check-out",
			method:         http.MethodPost,
			path:           "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-out",
			userRole:       "gm_admin",
			initialStatus:  booking.StatusCheckedIn,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRBACTestRouter(t, tt.initialStatus)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.userRole != "" {
				req.Header.Set("Authorization", "Bearer "+tt.userRole)
			}
			if tt.bearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+tt.bearerToken)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("%s %s (role: %s): expected status %d, got %d (body: %s)",
					tt.method, tt.path, tt.userRole, tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}
