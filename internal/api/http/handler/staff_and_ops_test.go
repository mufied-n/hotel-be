package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/assistance"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/housekeeping"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/example/hotel-booking/internal/staffauth"
	"github.com/example/hotel-booking/internal/stay"
	"github.com/gin-gonic/gin"
)

// Mock types for ops services
type mockStaffAuth struct{}

func (mockStaffAuth) VerifyStaffToken(_ context.Context, token string) (staffauth.Principal, error) {
	if token == "stf_good" {
		return staffauth.Principal{Username: "receptionist1", Role: "receptionist"}, nil
	}
	if token == "stf_gm" {
		return staffauth.Principal{Username: "gm1", Role: "gm_admin"}, nil
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}
func (mockStaffAuth) Login(_ context.Context, username, password string) (string, time.Time, staffauth.Principal, error) {
	if username == "admin" && password == "pass" {
		return "stf_good", time.Now().Add(time.Hour), staffauth.Principal{Username: "admin", Role: "gm_admin"}, nil
	}
	return "", time.Time{}, staffauth.Principal{}, staffauth.ErrInvalidCredentials
}
func (mockStaffAuth) Logout(context.Context, string) error { return nil }

type mockFinanceService struct {
	finance.Service
}

func (m *mockFinanceService) ProcessRefund(_ context.Context, in finance.CreateRefundInput) (*finance.PaymentRefund, error) {
	switch in.BookingID {
	case "not-found":
		return nil, finance.ErrBookingNotFound
	case "not-paid":
		return nil, finance.ErrBookingNotPaid
	case "over-refund":
		return nil, finance.ErrOverRefund
	case "gateway-failed":
		return nil, finance.ErrGatewayFailed
	default:
		return &finance.PaymentRefund{ID: "rf-1", AmountMinor: in.AmountMinor, Status: "succeeded"}, nil
	}
}
func (m *mockFinanceService) ListCases(context.Context, string, int) ([]finance.PaymentCase, error) {
	return []finance.PaymentCase{{ID: "c-1", Status: "open"}}, nil
}
func (m *mockFinanceService) ResolveCase(_ context.Context, in finance.ResolveCaseInput) error {
	switch in.CaseID {
	case "not-found":
		return finance.ErrCaseNotFound
	case "already-resolved":
		return finance.ErrCaseAlreadyResolved
	default:
		return nil
	}
}
func (m *mockFinanceService) GetReconciliationSummary(context.Context) (*finance.ReconciliationSummary, error) {
	return &finance.ReconciliationSummary{TotalSettledMinor: 1000000}, nil
}
func (m *mockFinanceService) GetBookingRefundStatus(_ context.Context, _, id string) (*finance.RefundStatusView, error) {
	if id == "not-found" {
		return nil, finance.ErrBookingNotFound
	}
	return &finance.RefundStatusView{BookingID: id}, nil
}

type mockHousekeepingService struct {
	housekeeping.Service
}

func (m *mockHousekeepingService) GetRoomBoard(context.Context, int, string, string) (*housekeeping.RoomBoardSummary, error) {
	return &housekeeping.RoomBoardSummary{TotalRooms: 95}, nil
}
func (m *mockHousekeepingService) UpdateStatus(_ context.Context, in housekeeping.UpdateStatusInput) error {
	if in.RoomNumber == "invalid-trans" {
		return housekeeping.ErrInvalidTransition
	}
	return nil
}
func (m *mockHousekeepingService) MarkRoomOutOfOrder(context.Context, string, time.Time, time.Time, string) error {
	return nil
}

type mockFrontDeskService struct {
	frontdesk.Service
}

func (m *mockFrontDeskService) GetDailyRoster(context.Context, time.Time) (*frontdesk.DailyRoster, error) {
	return &frontdesk.DailyRoster{Date: "2026-10-14"}, nil
}
func (m *mockFrontDeskService) RecordHandover(context.Context, frontdesk.RecordHandoverInput) (*frontdesk.HandoverNote, error) {
	return &frontdesk.HandoverNote{ID: "n-1", Shift: "morning", PendingIssues: "All good"}, nil
}
func (m *mockFrontDeskService) ListHandovers(context.Context, int, int) ([]frontdesk.HandoverNote, int, error) {
	return []frontdesk.HandoverNote{{ID: "n-1", PendingIssues: "All good"}}, 1, nil
}

type mockStayService struct {
	stay.Service
}

func (m *mockStayService) MoveRoom(_ context.Context, in stay.RoomMoveInput) (*stay.RoomMoveResult, error) {
	switch in.BookingID {
	case "not-found":
		return nil, stay.ErrBookingNotFound
	case "invalid-status":
		return nil, stay.ErrInvalidBookingStatus
	}
	switch in.TargetRoomNumber {
	case "same":
		return nil, stay.ErrSameRoomMove
	case "not-ready":
		return nil, stay.ErrTargetRoomNotReady
	case "overlap":
		return nil, stay.ErrRoomPhysicalOverlap
	}
	return &stay.RoomMoveResult{BookingID: in.BookingID, PreviousRoomNumber: "101", NewRoomNumber: in.TargetRoomNumber}, nil
}
func (m *mockStayService) ExtendStay(_ context.Context, in stay.ExtendStayInput) (*stay.ExtendStayResult, error) {
	switch in.BookingID {
	case "not-found":
		return nil, stay.ErrBookingNotFound
	case "invalid-status":
		return nil, stay.ErrInvalidBookingStatus
	default:
		return &stay.ExtendStayResult{BookingID: in.BookingID, AdditionalNights: in.AdditionalNights}, nil
	}
}
func (m *mockStayService) ListRoomMoves(_ context.Context, id string) ([]stay.RoomMoveLog, error) {
	if id == "not-found" {
		return nil, stay.ErrBookingNotFound
	}
	return []stay.RoomMoveLog{{ID: "rm-1"}}, nil
}

type mockAssistanceService struct {
	assistance.Service
}

func (m *mockAssistanceService) CreateGuestRequest(_ context.Context, _ string, in assistance.CreateRequestInput) (*assistance.SpecialRequest, error) {
	if in.BookingID == "not-found" {
		return nil, assistance.ErrBookingNotFound
	}
	if in.Category == "invalid" {
		return nil, assistance.ErrInvalidCategory
	}
	return &assistance.SpecialRequest{ID: "sr-1"}, nil
}
func (m *mockAssistanceService) ListGuestRequests(_ context.Context, _ string, bookingID string) ([]assistance.SpecialRequest, error) {
	if bookingID == "not-found" {
		return nil, assistance.ErrBookingNotFound
	}
	return []assistance.SpecialRequest{{ID: "sr-1"}}, nil
}
func (m *mockAssistanceService) ListStaffQueue(context.Context, assistance.ListFilter) ([]assistance.StaffQueueItem, error) {
	return []assistance.StaffQueueItem{{ID: "sr-1"}}, nil
}
func (m *mockAssistanceService) UpdateStatus(_ context.Context, in assistance.UpdateStatusInput) (*assistance.SpecialRequest, error) {
	if in.RequestID == "not-found" {
		return nil, assistance.ErrRequestNotFound
	}
	if in.RequestID == "invalid-trans" {
		return nil, assistance.ErrInvalidStatusTransition
	}
	return &assistance.SpecialRequest{ID: in.RequestID, Status: assistance.StatusFulfilled}, nil
}

func TestOpsHandlers_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	flagMgr := featureflag.NewMemoryManager(map[string]featureflag.Flag{
		"promo": {Key: "promo", Enabled: true},
	})

	guestSvc := &mockFullGuestService{}
	deps := Deps{
		StaffAuth:       mockStaffAuth{},
		FeatureFlag:     flagMgr,
		FinanceSvc:      &mockFinanceService{},
		HousekeepingSvc: &mockHousekeepingService{},
		FrontDeskSvc:    &mockFrontDeskService{},
		StaySvc:         &mockStayService{},
		AssistanceSvc:   &mockAssistanceService{},
		GuestSvc:        guestSvc,
	}

	r := gin.New()
	r.Use(middleware.IdentifySubject(mockStaffAuth{}))

	// Staff Auth
	r.POST("/api/v1/auth/staff/login", StaffLogin(deps))
	r.GET("/api/v1/auth/staff/me", StaffMe(deps))
	r.POST("/api/v1/auth/staff/logout", StaffLogout(deps))

	// Feature Flags
	r.GET("/api/v1/admin/feature-flags", AdminListFlags(deps))
	r.PUT("/api/v1/admin/feature-flags/:key", AdminUpdateFlag(deps))

	// Finance
	r.POST("/api/v1/finance/refunds", FinanceRefund(deps))
	r.GET("/api/v1/finance/cases", FinanceCases(deps))
	r.POST("/api/v1/finance/cases/:id/resolve", FinanceResolveCase(deps))
	r.GET("/api/v1/finance/reconciliations", FinanceSummary(deps))

	// Housekeeping
	r.GET("/api/v1/housekeeping/rooms", HousekeepingRooms(deps))
	r.PUT("/api/v1/housekeeping/rooms/:id/status", HousekeepingStatus(deps))
	r.POST("/api/v1/housekeeping/rooms/:id/out-of-order", HousekeepingOOO(deps))

	// Front Desk
	r.GET("/api/v1/front-desk/daily-roster", FrontDeskDailyRoster(deps))
	r.POST("/api/v1/front-desk/handover-notes", FrontDeskRecordHandover(deps))
	r.GET("/api/v1/front-desk/handover-notes", FrontDeskListHandovers(deps))

	// Stay
	r.POST("/api/v1/bookings/:id/room-move", HandleRoomMove(deps))
	r.POST("/api/v1/bookings/:id/extend-stay", HandleExtendStay(deps))
	r.GET("/api/v1/bookings/:id/room-moves", HandleListRoomMoves(deps))

	// Assistance & Guest Portal
	guestGroup := r.Group("")
	guestGroup.Use(middleware.RequireGuestSession(deps.GuestSvc))
	guestGroup.GET("/api/v1/guest/bookings/:id/refund-status", GuestRefundStatus(deps))
	guestGroup.POST("/api/v1/guest/bookings/:id/special-requests", CreateGuestSpecialRequest(deps))
	guestGroup.GET("/api/v1/guest/bookings/:id/special-requests", ListGuestSpecialRequests(deps))
	r.GET("/api/v1/front-desk/special-requests", ListStaffSpecialRequests(deps))
	r.PUT("/api/v1/front-desk/special-requests/:id/status", UpdateStaffSpecialRequestStatus(deps))

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		headers    map[string]string
		wantStatus int
	}{
		// Staff auth
		{
			name:       "staff login bad json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/auth/staff/login",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "staff login invalid creds returns 401",
			method:     http.MethodPost,
			url:        "/api/v1/auth/staff/login",
			body:       `{"username":"wrong","password":"bad"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "staff login success returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/auth/staff/login",
			body:       `{"username":"admin","password":"pass"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "staff me with token returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/auth/staff/me",
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "staff logout returns 204",
			method:     http.MethodPost,
			url:        "/api/v1/auth/staff/logout",
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNoContent,
		},

		// Flags
		{
			name:       "admin list flags returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/admin/feature-flags",
			wantStatus: http.StatusOK,
		},
		{
			name:       "admin update flag valid returns 200",
			method:     http.MethodPut,
			url:        "/api/v1/admin/feature-flags/promo",
			body:       `{"enabled":false}`,
			wantStatus: http.StatusOK,
		},

		// Finance
		{
			name:       "finance refund valid returns 201",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{"booking_id":"b1","amount_minor":500000,"reason":"guest request"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "finance cases returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/finance/cases",
			wantStatus: http.StatusOK,
		},
		{
			name:       "finance resolve case returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/finance/cases/c-1/resolve",
			body:       `{"action":"manual_confirm","notes":"done"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "finance summary returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/finance/reconciliations",
			wantStatus: http.StatusOK,
		},
		{
			name:       "guest refund status returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/refund-status",
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusOK,
		},

		// Housekeeping
		{
			name:       "housekeeping rooms returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/housekeeping/rooms",
			wantStatus: http.StatusOK,
		},
		{
			name:       "housekeeping update status returns 200",
			method:     http.MethodPut,
			url:        "/api/v1/housekeeping/rooms/101/status",
			body:       `{"to_status":"inspected"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "housekeeping ooo returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/housekeeping/rooms/101/out-of-order",
			body:       `{"start_date":"2026-10-14","end_date":"2026-10-16","reason":"AC repair"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_gm"},
			wantStatus: http.StatusOK,
		},

		// Front Desk
		{
			name:       "front desk daily roster returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/front-desk/daily-roster?date=2026-10-14",
			wantStatus: http.StatusOK,
		},
		{
			name:       "front desk record handover returns 201",
			method:     http.MethodPost,
			url:        "/api/v1/front-desk/handover-notes",
			body:       `{"shift":"morning","pending_issues":"Checked in 5 guests"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "front desk list handovers returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/front-desk/handover-notes",
			wantStatus: http.StatusOK,
		},

		// Stay
		{
			name:       "stay room move returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/room-move",
			body:       `{"target_room_number":"102","reason_category":"noise_complaint","notes":"Noise complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "stay extend returns 200",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/extend-stay",
			body:       `{"additional_nights":1,"payment_method":"cash"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "stay list room moves returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/bookings/b1/room-moves",
			wantStatus: http.StatusOK,
		},

		// Assistance
		{
			name:       "guest create special request returns 201",
			method:     http.MethodPost,
			url:        "/api/v1/guest/bookings/b1/special-requests",
			body:       `{"category":"bedding","description":"Extra pillows"}`,
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "guest list special requests returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/b1/special-requests",
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "staff list special requests returns 200",
			method:     http.MethodGet,
			url:        "/api/v1/front-desk/special-requests",
			wantStatus: http.StatusOK,
		},
		{
			name:       "staff update special request status returns 200",
			method:     http.MethodPut,
			url:        "/api/v1/front-desk/special-requests/sr-1/status",
			body:       `{"to_status":"fulfilled","staff_notes":"Delivered to room"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusOK,
		},

		// Staff auth error
		{
			name:       "staff login invalid credentials returns 401",
			method:     http.MethodPost,
			url:        "/api/v1/auth/staff/login",
			body:       `{"username":"admin","password":"wrong"}`,
			wantStatus: http.StatusUnauthorized,
		},

		// Feature Flag error
		{
			name:       "admin update flag invalid json returns 400",
			method:     http.MethodPut,
			url:        "/api/v1/admin/feature-flags/promo",
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},

		// Finance errors
		{
			name:       "finance refund invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "finance refund booking not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{"booking_id":"not-found","amount_minor":500000,"reason":"guest request"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "finance refund booking not paid returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{"booking_id":"not-paid","amount_minor":500000,"reason":"guest request"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "finance refund over refund returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{"booking_id":"over-refund","amount_minor":500000,"reason":"guest request"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "finance refund gateway failed returns 502",
			method:     http.MethodPost,
			url:        "/api/v1/finance/refunds",
			body:       `{"booking_id":"gateway-failed","amount_minor":500000,"reason":"guest request"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "finance resolve case invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/finance/cases/c-1/resolve",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "finance resolve case not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/finance/cases/not-found/resolve",
			body:       `{"action":"manual_confirm","notes":"done"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "finance resolve case already resolved returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/finance/cases/already-resolved/resolve",
			body:       `{"action":"manual_confirm","notes":"done"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "guest refund status not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/not-found/refund-status",
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusNotFound,
		},

		// Housekeeping errors
		{
			name:       "housekeeping update status invalid json returns 400",
			method:     http.MethodPut,
			url:        "/api/v1/housekeeping/rooms/101/status",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "housekeeping update status invalid transition returns 409",
			method:     http.MethodPut,
			url:        "/api/v1/housekeeping/rooms/invalid-trans/status",
			body:       `{"to_status":"inspected"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "housekeeping ooo invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/housekeeping/rooms/101/out-of-order",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_gm"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "housekeeping ooo invalid date returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/housekeeping/rooms/101/out-of-order",
			body:       `{"start_date":"invalid","end_date":"2026-10-16"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_gm"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "housekeeping ooo start after end returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/housekeeping/rooms/101/out-of-order",
			body:       `{"start_date":"2026-10-18","end_date":"2026-10-16"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_gm"},
			wantStatus: http.StatusBadRequest,
		},

		// Front Desk errors
		{
			name:       "front desk daily roster invalid date returns 400",
			method:     http.MethodGet,
			url:        "/api/v1/front-desk/daily-roster?date=invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "front desk record handover invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/front-desk/handover-notes",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},

		// Stay errors
		{
			name:       "stay room move invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/room-move",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "stay room move booking not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/not-found/room-move",
			body:       `{"target_room_number":"102","reason_category":"noise_complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "stay room move invalid status returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/invalid-status/room-move",
			body:       `{"target_room_number":"102","reason_category":"noise_complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "stay room move same room returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/room-move",
			body:       `{"target_room_number":"same","reason_category":"noise_complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "stay room move target room not ready returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/room-move",
			body:       `{"target_room_number":"not-ready","reason_category":"noise_complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "stay room move physical overlap returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/room-move",
			body:       `{"target_room_number":"overlap","reason_category":"noise_complaint"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "stay extend invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/b1/extend-stay",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "stay extend not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/not-found/extend-stay",
			body:       `{"additional_nights":1,"payment_method":"cash"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "stay extend invalid status returns 409",
			method:     http.MethodPost,
			url:        "/api/v1/bookings/invalid-status/extend-stay",
			body:       `{"additional_nights":1,"payment_method":"cash"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "stay list room moves not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/bookings/not-found/room-moves",
			wantStatus: http.StatusNotFound,
		},

		// Assistance errors
		{
			name:       "guest create special request invalid json returns 400",
			method:     http.MethodPost,
			url:        "/api/v1/guest/bookings/b1/special-requests",
			body:       `{bad`,
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "guest create special request not found returns 404",
			method:     http.MethodPost,
			url:        "/api/v1/guest/bookings/not-found/special-requests",
			body:       `{"category":"bedding","description":"Extra pillows"}`,
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "guest list special requests not found returns 404",
			method:     http.MethodGet,
			url:        "/api/v1/guest/bookings/not-found/special-requests",
			headers:    map[string]string{"X-Guest-Session": "gst_sess_123"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "staff update special request invalid json returns 400",
			method:     http.MethodPut,
			url:        "/api/v1/front-desk/special-requests/sr-1/status",
			body:       `{bad`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "staff update special request not found returns 404",
			method:     http.MethodPut,
			url:        "/api/v1/front-desk/special-requests/not-found/status",
			body:       `{"to_status":"fulfilled"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "staff update special request invalid transition returns 400",
			method:     http.MethodPut,
			url:        "/api/v1/front-desk/special-requests/invalid-trans/status",
			body:       `{"to_status":"fulfilled"}`,
			headers:    map[string]string{"Authorization": "Bearer stf_good"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Buffer
			if tc.body != "" {
				bodyReader = bytes.NewBufferString(tc.body)
			} else {
				bodyReader = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.url, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestOpsHandlers_NilDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := Deps{}

	tests := []struct {
		name       string
		handler    gin.HandlerFunc
		method     string
		url        string
		wantStatus int
	}{
		{"finance refund nil", FinanceRefund(deps), http.MethodPost, "/", http.StatusNotImplemented},
		{"finance cases nil", FinanceCases(deps), http.MethodGet, "/", http.StatusNotImplemented},
		{"finance resolve nil", FinanceResolveCase(deps), http.MethodPost, "/c1", http.StatusNotImplemented},
		{"finance summary nil", FinanceSummary(deps), http.MethodGet, "/", http.StatusNotImplemented},
		{"guest refund nil", GuestRefundStatus(deps), http.MethodGet, "/b1", http.StatusNotImplemented},
		{"hk rooms nil", HousekeepingRooms(deps), http.MethodGet, "/", http.StatusInternalServerError},
		{"hk status nil", HousekeepingStatus(deps), http.MethodPut, "/101", http.StatusInternalServerError},
		{"hk ooo nil", HousekeepingOOO(deps), http.MethodPost, "/101", http.StatusInternalServerError},
		{"fd roster nil", FrontDeskDailyRoster(deps), http.MethodGet, "/", http.StatusInternalServerError},
		{"fd handover nil", FrontDeskRecordHandover(deps), http.MethodPost, "/", http.StatusInternalServerError},
		{"fd list nil", FrontDeskListHandovers(deps), http.MethodGet, "/", http.StatusInternalServerError},
		{"stay move nil", HandleRoomMove(deps), http.MethodPost, "/b1", http.StatusInternalServerError},
		{"stay extend nil", HandleExtendStay(deps), http.MethodPost, "/b1", http.StatusInternalServerError},
		{"stay list nil", HandleListRoomMoves(deps), http.MethodGet, "/b1", http.StatusInternalServerError},
		{"assist create nil", CreateGuestSpecialRequest(deps), http.MethodPost, "/b1", http.StatusServiceUnavailable},
		{"assist list guest nil", ListGuestSpecialRequests(deps), http.MethodGet, "/b1", http.StatusServiceUnavailable},
		{"assist list staff nil", ListStaffSpecialRequests(deps), http.MethodGet, "/", http.StatusServiceUnavailable},
		{"assist update nil", UpdateStaffSpecialRequestStatus(deps), http.MethodPut, "/sr1", http.StatusServiceUnavailable},
		{"flag list nil", AdminListFlags(deps), http.MethodGet, "/", http.StatusNotImplemented},
		{"flag update nil", AdminUpdateFlag(deps), http.MethodPut, "/promo", http.StatusNotImplemented},
		{"staff login nil", StaffLogin(deps), http.MethodPost, "/", http.StatusServiceUnavailable},
		{"staff logout nil", StaffLogout(deps), http.MethodPost, "/", http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("auth_context", middleware.AuthContext{Role: "gm_admin", Subject: "admin"})
				c.Next()
			})
			r.Handle(tc.method, "/:id", tc.handler)
			r.Handle(tc.method, "/", tc.handler)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.url, bytes.NewBufferString(`{"username":"user","password":"pass"}`))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.GuestSessionContextKey, &guest.GuestSession{GuestEmail: "guest@example.com"})
			req = req.WithContext(ctx)
			r.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
