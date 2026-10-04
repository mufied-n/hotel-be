package http

import (
	stdhttp "net/http"

	"github.com/example/hotel-booking/internal/api/http/handler"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/gin-gonic/gin"
)

// registerRoutes mendaftarkan seluruh route HTTP API secara terstruktur per domain (FR-03).
func registerRoutes(r *gin.Engine, d Deps) {
	ff := func(key string) gin.HandlerFunc {
		return middleware.RequireFeature(d.FeatureFlag, key)
	}

	// 1. Health & Readiness (probe publik)
	r.Match([]string{stdhttp.MethodGet, stdhttp.MethodHead}, "/healthz", handler.Healthz)
	r.Match([]string{stdhttp.MethodGet, stdhttp.MethodHead}, "/ready", handler.Ready(d))

	// 2. Webhooks
	r.POST("/api/v1/webhooks/xendit", ff("ff_xendit_payment_gateway"), handler.XenditWebhook(d))

	// 3. Public Auth (kredensial staf login & challenge/verify tamu) (FR-23)
	authLimit := func(c *gin.Context) { c.Next() }
	if d.AuthRateLimiter != nil {
		authLimit = d.AuthRateLimiter.Limit()
	}
	r.POST("/api/v1/auth/guest/challenge", authLimit, ff("ff_guest_portal_auth"), handler.GuestChallenge(d))
	r.POST("/api/v1/auth/guest/verify", authLimit, ff("ff_guest_portal_auth"), handler.GuestVerify(d))
	r.POST("/api/v1/auth/staff/login", authLimit, handler.StaffLogin(d))
	r.POST("/api/v1/channel-events", ff("ff_channel_sync_integration"), handler.HandleChannelWebhook(d))

	v1 := r.Group("/api/v1")

	// 4. Staff Auth Session (me & logout)
	staffAuth := v1.Group("/auth/staff")
	staffAuth.Use(middleware.IdentifySubject(d.StaffAuth), middleware.RequireStaffSession())
	{
		staffAuth.GET("/me", handler.StaffMe(d))
		staffAuth.POST("/logout", handler.StaffLogout(d))
	}

	// 5. Guest Session Group (memerlukan gst_sess_ token)
	guestGroup := v1.Group("")
	guestGroup.Use(middleware.RequireGuestSession(d.GuestSvc))
	registerGuestRoutes(guestGroup, d, ff)

	// 6. Protected API Group (RBAC Casbin fail-closed)
	apiGroup := v1.Group("")
	apiGroup.Use(middleware.IdentifySubject(d.StaffAuth))
	apiGroup.Use(middleware.Authorize(d.Enforcer))
	registerDomainAPIRoutes(apiGroup, d, ff)

	// 7. Dev-only mock gateway (gated)
	if d.IsDevelopment && d.FakePay != nil {
		devGroup := r.Group("")
		devGroup.Use(middleware.IdentifySubject(d.StaffAuth))
		devGroup.Use(middleware.Authorize(d.Enforcer))
		devGroup.POST("/fake-pay/:ref", d.FakePay)
	}
}

func registerGuestRoutes(g *gin.RouterGroup, d Deps, ff func(string) gin.HandlerFunc) {
	g.GET("/auth/guest/me", ff("ff_guest_portal_auth"), handler.GuestMe(d))
	g.POST("/auth/guest/logout", ff("ff_guest_portal_auth"), handler.GuestLogout(d))
	g.GET("/guest/bookings", ff("ff_guest_my_bookings"), handler.GuestBookings(d))
	g.GET("/guest/bookings/:id", ff("ff_guest_my_bookings"), handler.GuestBookingDetail(d))
	g.GET("/guest/bookings/:id/payment", ff("ff_guest_my_bookings"), handler.GuestBookingPayment(d))
	g.GET("/guest/bookings/:id/receipt", ff("ff_booking_artifacts_receipt"), handler.GuestBookingReceipt(d))
	g.GET("/guest/bookings/:id/calendar.ics", ff("ff_booking_artifacts_icalendar"), handler.GuestBookingCalendar(d))
	g.GET("/guest/bookings/:id/voucher.pdf", ff("ff_official_pdf_voucher"), handler.GuestBookingVoucherPDF(d))
	g.GET("/guest/bookings/:id/invoice.pdf", ff("ff_official_pdf_voucher"), handler.GuestBookingInvoicePDF(d))
	g.GET("/guest/bookings/:id/live-status", ff("ff_realtime_event_hub"), handler.GuestBookingLiveStatus(d))
	g.GET("/guest/bookings/:id/refund-status", ff("ff_guest_my_bookings"), handler.GuestRefundStatus(d))
	g.POST("/guest/bookings/:id/special-requests", ff("ff_guest_special_requests"), handler.CreateGuestSpecialRequest(d))
	g.GET("/guest/bookings/:id/special-requests", ff("ff_guest_special_requests"), handler.ListGuestSpecialRequests(d))
}

func registerDomainAPIRoutes(g *gin.RouterGroup, d Deps, ff func(string) gin.HandlerFunc) {
	// Catalog
	g.GET("/catalog/rooms", handler.GetCatalogRooms(d))
	g.GET("/catalog/rooms/:id", handler.GetCatalogRoom(d))
	g.POST("/catalog/rooms", ff("ff_catalog_write"), handler.CreateCatalogRoom(d))
	g.PUT("/catalog/rooms/:id", ff("ff_catalog_write"), handler.UpdateCatalogRoom(d))
	g.DELETE("/catalog/rooms/:id", ff("ff_catalog_write"), handler.DeleteCatalogRoom(d))

	// Search & Availability & Quotes
	g.GET("/search", ff("ff_multi_variant_search"), handler.SearchRooms(d))
	g.GET("/availability", handler.GetAvailability(d))
	g.POST("/quotes", ff("ff_quote_locking_engine"), handler.CalculateQuote(d))

	// Bookings Core
	g.POST("/bookings", handler.CreateBooking(d))
	g.GET("/bookings/:id", handler.GetBooking(d))
	g.GET("/bookings/:id/payment", handler.GetBookingPayment(d))
	g.GET("/bookings/:id/voucher.pdf", ff("ff_official_pdf_voucher"), handler.GetBookingVoucherPDF(d))
	g.GET("/bookings/:id/invoice.pdf", ff("ff_official_pdf_voucher"), handler.GetBookingInvoicePDF(d))
	g.POST("/bookings/:id/cancel", handler.CancelBooking(d))
	g.POST("/bookings/:id/check-in", handler.CheckIn(d))
	g.POST("/bookings/:id/check-out", handler.CheckOut(d))
	g.POST("/bookings/:id/no-show", handler.NoShow(d))

	// Finance Reconciliation & Refunds (F14)
	g.POST("/finance/refunds", ff("ff_gateway_automated_refund"), handler.FinanceRefund(d))
	g.GET("/finance/cases", ff("ff_finance_reconciliation"), handler.FinanceCases(d))
	g.POST("/finance/cases/:id/resolve", ff("ff_finance_reconciliation"), handler.FinanceResolveCase(d))
	g.GET("/finance/reconciliations", ff("ff_finance_reconciliation"), handler.FinanceSummary(d))

	// Housekeeping Room Status & Readiness Lifecycle (Proposed 01)
	g.GET("/housekeeping/rooms", ff("ff_housekeeping_board"), handler.HousekeepingRooms(d))
	g.PUT("/housekeeping/rooms/:id/status", ff("ff_housekeeping_board"), handler.HousekeepingStatus(d))
	g.POST("/housekeeping/rooms/:id/out-of-order", ff("ff_housekeeping_board"), handler.HousekeepingOOO(d))

	// Front Desk Operations & Handover (Proposed 02)
	g.GET("/front-desk/daily-roster", ff("ff_front_desk_operations"), handler.FrontDeskDailyRoster(d))
	g.GET("/front-desk/handover-notes", ff("ff_front_desk_operations"), handler.FrontDeskListHandovers(d))
	g.POST("/front-desk/handover-notes", ff("ff_front_desk_operations"), handler.FrontDeskRecordHandover(d))
	g.GET("/front-desk/verify-voucher", ff("ff_front_desk_operations"), handler.VerifyVoucher(d))
	g.GET("/front-desk/live-stream", ff("ff_realtime_event_hub"), handler.FrontDeskLiveStream(d))

	// Stay Modification: Room Move & Extension (Proposed 03)
	g.POST("/bookings/:id/room-move", ff("ff_stay_modification"), handler.HandleRoomMove(d))
	g.POST("/bookings/:id/extend-stay", ff("ff_stay_modification"), handler.HandleExtendStay(d))
	g.GET("/bookings/:id/room-moves", ff("ff_stay_modification"), handler.HandleListRoomMoves(d))

	// Guest Special Requests & Stay Assistance Desk (Proposed 04)
	g.GET("/front-desk/special-requests", ff("ff_guest_special_requests"), handler.ListStaffSpecialRequests(d))
	g.PUT("/front-desk/special-requests/:id/status", ff("ff_guest_special_requests"), handler.UpdateStaffSpecialRequestStatus(d))

	// Feature Flags Administration (FR-FF-04)
	g.GET("/admin/feature-flags", handler.AdminListFlags(d))
	g.PUT("/admin/feature-flags/:key", handler.AdminUpdateFlag(d))

	// Revenue Management (Proposed Candidate A: Dynamic Rates & Stop-Sell)
	g.GET("/revenue/calendar", ff("ff_dynamic_rates_calendar"), handler.GetRevenueCalendar(d))
	g.PUT("/revenue/calendar/bulk", ff("ff_dynamic_rates_calendar"), handler.BulkUpdateCalendar(d))
	g.GET("/revenue/promos", ff("ff_promotions_engine"), handler.ListPromos(d))
	g.POST("/revenue/promos", ff("ff_promotions_engine"), handler.CreatePromo(d))
	g.PUT("/revenue/promos/:id", ff("ff_promotions_engine"), handler.UpdatePromo(d))

	// Channel Management & OTA Integration (F10)
	g.GET("/staff/channel-sync-issues", ff("ff_channel_sync_integration"), handler.HandleListChannelSyncIssues(d))
	g.POST("/staff/channel-sync-issues/:id/resolve", ff("ff_last_room_safeguards"), handler.HandleResolveChannelSyncIssue(d))
	g.GET("/staff/channel-partners/:code", ff("ff_channel_sync_integration"), handler.HandleGetChannelPartner(d))
}
