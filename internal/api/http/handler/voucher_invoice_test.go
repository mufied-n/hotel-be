package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/docgen"
	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/staffauth"
	"github.com/gin-gonic/gin"
)

type mockVoucherGuestService struct {
	guest.Service
	receiptFn func(ctx context.Context, email, id string) (*guest.ReceiptDTO, error)
}

func (m *mockVoucherGuestService) GetBookingReceipt(ctx context.Context, email, id string) (*guest.ReceiptDTO, error) {
	if m.receiptFn != nil {
		return m.receiptFn(ctx, email, id)
	}
	return nil, guest.ErrBookingNotFound
}

func (m *mockVoucherGuestService) ValidateSession(_ context.Context, token string) (*guest.GuestSession, error) {
	if token == "gst_sess_valid" {
		return &guest.GuestSession{GuestEmail: "guest@example.com"}, nil
	}
	return nil, fmt.Errorf("invalid session")
}

type mockVoucherStaffAuth struct{}

func (mockVoucherStaffAuth) VerifyStaffToken(_ context.Context, token string) (staffauth.Principal, error) {
	if token == "stf_receptionist" {
		return staffauth.Principal{Username: "rec1", Role: "receptionist"}, nil
	}
	if token == "stf_finance" {
		return staffauth.Principal{Username: "fin1", Role: "finance"}, nil
	}
	if token == "stf_gm" {
		return staffauth.Principal{Username: "gm1", Role: "gm_admin"}, nil
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}

func (mockVoucherStaffAuth) Login(context.Context, string, string) (string, time.Time, staffauth.Principal, error) {
	return "", time.Time{}, staffauth.Principal{}, nil
}
func (mockVoucherStaffAuth) Logout(context.Context, string) error { return nil }

func sampleReceiptDTO() *guest.ReceiptDTO {
	return &guest.ReceiptDTO{
		InvoiceNumber:    "INV/PKU/202610/01923456",
		InvoiceDate:      "2026-10-04T10:00:00Z",
		BookingID:        "01923456-789a-bcde-f012-3456789abcde",
		BookingReference: "PKU-20261004-01923456",
		Status:           "confirmed",
		StayDetails: guest.StayDetails{
			CheckInDate:  "2026-10-10",
			CheckInTime:  "14:00 WIB",
			CheckOutDate: "2026-10-12",
			CheckOutTime: "12:00 WIB",
			TotalNights:  2,
			Timezone:     "Asia/Jakarta",
		},
		GuestDetails: guest.GuestDetails{
			Name:      "Budi Santoso",
			Email:     "guest@example.com",
			Phone:     "+628123456789",
			NumRooms:  1,
			NumGuests: 2,
		},
		RoomItem: guest.RoomItemReceipt{
			RoomTypeID:       "rt-deluxe",
			RoomTypeName:     "Deluxe King",
			RatePlanCode:     "ROOM_BREAKFAST",
			MealPlan:         "Sarapan Termasuk (Breakfast Included)",
			NumRooms:         1,
			TotalNights:      2,
			NightlyRateMinor: 1000000,
			SubtotalMinor:    2000000,
		},
		PricingBreakdown: guest.PricingBreakdown{
			Currency:        "IDR",
			TotalPriceMinor: 2420000,
		},
		PaymentSummary: guest.PaymentSummary{
			Status:   "PAID",
			Provider: "BCA Virtual Account",
			PaidAt:   "2026-10-04T10:05:00Z",
		},
	}
}

func TestGuestBookingVoucherPDF_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		hasSession   bool
		nilSvc       bool
		bookingID    string
		receiptFn    func(ctx context.Context, email, id string) (*guest.ReceiptDTO, error)
		wantStatus   int
		wantContains string
	}{
		{
			name:       "Unauthorized without guest session",
			hasSession: false,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Not implemented when GuestSvc is nil",
			hasSession: true,
			nilSvc:     true,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "Booking not found returns 404",
			hasSession: true,
			bookingID:  "not-found-id",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return nil, guest.ErrBookingNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Booking unpaid returns 400 RECEIPT_NOT_AVAILABLE",
			hasSession: true,
			bookingID:  "pending-id",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return nil, guest.ErrReceiptNotAvailable
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Confirmed booking returns 200 and PDF byte stream",
			hasSession: true,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return sampleReceiptDTO(), nil
			},
			wantStatus:   http.StatusOK,
			wantContains: "%PDF-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			var guestSvc guest.Service
			if !tt.nilSvc {
				guestSvc = &mockVoucherGuestService{receiptFn: tt.receiptFn}
			}
			d := Deps{GuestSvc: guestSvc}

			r.Use(middleware.RequireGuestSession(guestSvc))
			r.GET("/api/v1/guest/bookings/:id/voucher.pdf", GuestBookingVoucherPDF(d))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/"+tt.bookingID+"/voucher.pdf", nil)
			if tt.hasSession {
				req.Header.Set("Authorization", "Bearer gst_sess_valid")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantContains != "" && !strings.Contains(w.Body.String(), tt.wantContains) {
				t.Fatalf("expected body to contain %q, but was not found", tt.wantContains)
			}

			if tt.wantStatus == http.StatusOK {
				disp := w.Header().Get("Content-Disposition")
				if !strings.Contains(disp, "voucher-PKU-20261004-01923456.pdf") {
					t.Errorf("unexpected Content-Disposition header: %s", disp)
				}
				if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
					t.Errorf("expected Content-Type application/pdf, got %s", ct)
				}
			}
		})
	}
}

func TestGuestBookingInvoicePDF_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		hasSession   bool
		nilSvc       bool
		bookingID    string
		receiptFn    func(ctx context.Context, email, id string) (*guest.ReceiptDTO, error)
		wantStatus   int
		wantContains string
	}{
		{
			name:       "Unauthorized without guest session",
			hasSession: false,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Not implemented when GuestSvc is nil",
			hasSession: true,
			nilSvc:     true,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "Booking not found returns 404",
			hasSession: true,
			bookingID:  "not-found-id",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return nil, guest.ErrBookingNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Booking unpaid returns 400 RECEIPT_NOT_AVAILABLE",
			hasSession: true,
			bookingID:  "pending-id",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return nil, guest.ErrReceiptNotAvailable
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Confirmed booking returns 200 and PDF byte stream",
			hasSession: true,
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			receiptFn: func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) {
				return sampleReceiptDTO(), nil
			},
			wantStatus:   http.StatusOK,
			wantContains: "%PDF-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			var guestSvc guest.Service
			if !tt.nilSvc {
				guestSvc = &mockVoucherGuestService{receiptFn: tt.receiptFn}
			}
			d := Deps{GuestSvc: guestSvc}

			r.Use(middleware.RequireGuestSession(guestSvc))
			r.GET("/api/v1/guest/bookings/:id/invoice.pdf", GuestBookingInvoicePDF(d))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/bookings/"+tt.bookingID+"/invoice.pdf", nil)
			if tt.hasSession {
				req.Header.Set("Authorization", "Bearer gst_sess_valid")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantContains != "" && !strings.Contains(w.Body.String(), tt.wantContains) {
				t.Fatalf("expected body to contain %q, but was not found", tt.wantContains)
			}

			if tt.wantStatus == http.StatusOK {
				disp := w.Header().Get("Content-Disposition")
				if !strings.Contains(disp, "faktur-pbjt-PKU-20261004-01923456.pdf") {
					t.Errorf("unexpected Content-Disposition header: %s", disp)
				}
				if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
					t.Errorf("expected Content-Type application/pdf, got %s", ct)
				}
			}
		})
	}
}

func TestGetBookingVoucherAndInvoicePDF_StaffAndGuest_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		authHeader string
		guestToken string
		bookingID  string
		endpoint   string
		receiptFn  func(ctx context.Context, email, id string) (*guest.ReceiptDTO, error)
		wantStatus int
	}{
		{
			name:       "Staff receptionist can download voucher PDF",
			authHeader: "Bearer stf_receptionist",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/voucher.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sampleReceiptDTO(), nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "Staff finance can download invoice PDF",
			authHeader: "Bearer stf_finance",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/invoice.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sampleReceiptDTO(), nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "Guest with valid token downloads voucher PDF",
			guestToken: "gst_token_abc",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/voucher.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sampleReceiptDTO(), nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "Guest without matching token receives 404",
			guestToken: "wrong_token",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/voucher.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sampleReceiptDTO(), nil },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Staff downloads voucher when receipt is not available (unpaid)",
			authHeader: "Bearer stf_receptionist",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/voucher.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return nil, guest.ErrReceiptNotAvailable },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Staff downloads invoice when booking not found in receipt store",
			authHeader: "Bearer stf_finance",
			bookingID:  "01923456-789a-bcde-f012-3456789abcde",
			endpoint:   "/api/v1/bookings/01923456-789a-bcde-f012-3456789abcde/invoice.pdf",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return nil, guest.ErrBookingNotFound },
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			mockTx := &mockBookingTx{
				booking: booking.Booking{
					ID:         "01923456-789a-bcde-f012-3456789abcde",
					GuestEmail: "guest@example.com",
					GuestToken: "gst_token_abc",
					Status:     booking.StatusConfirmed,
				},
			}
			runner := &mockTxRunner{tx: mockTx}
			reader := &mockBookingReader{tx: mockTx}
			bkgSvc := booking.NewService(runner, &mockInvStore{}, &mockRates{}, mockPaymentGateway{}, mockNotifier{}, reader, 30*time.Minute, nil)

			guestSvc := &mockVoucherGuestService{receiptFn: tt.receiptFn}
			staffAuth := mockVoucherStaffAuth{}
			d := Deps{
				BookingSvc: bkgSvc,
				GuestSvc:   guestSvc,
				StaffAuth:  staffAuth,
			}

			r.Use(middleware.IdentifySubject(staffAuth))
			r.GET("/api/v1/bookings/:id/voucher.pdf", GetBookingVoucherPDF(d))
			r.GET("/api/v1/bookings/:id/invoice.pdf", GetBookingInvoicePDF(d))

			req := httptest.NewRequest(http.MethodGet, tt.endpoint, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.guestToken != "" {
				req.Header.Set("X-Guest-Token", tt.guestToken)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestVerifyVoucher_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sample := sampleReceiptDTO()
	validToken := docgen.SignVoucherToken("", sample.BookingReference, sample.BookingID, sample.StayDetails.CheckInDate)

	tests := []struct {
		name       string
		ref        string
		token      string
		id         string
		receiptFn  func(ctx context.Context, email, id string) (*guest.ReceiptDTO, error)
		wantStatus int
		wantValid  bool
	}{
		{
			name:       "Missing ref or token returns 400",
			ref:        "",
			token:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Booking not found returns 404",
			ref:        "PKU-20261004-99999999",
			token:      validToken,
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return nil, guest.ErrBookingNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Tampered token returns 400 INVALID_QR_SIGNATURE",
			ref:        sample.BookingReference,
			token:      "tampered_token_signature_12345",
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sample, nil },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Nil GuestSvc returns 501",
			ref:        sample.BookingReference,
			token:      validToken,
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "Valid token with explicit ID returns 200 and SIGNATURE_VERIFIED",
			ref:        sample.BookingReference,
			id:         sample.BookingID,
			token:      validToken,
			receiptFn:  func(_ context.Context, _, _ string) (*guest.ReceiptDTO, error) { return sample, nil },
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			var guestSvc guest.Service
			if tt.wantStatus != http.StatusNotImplemented {
				guestSvc = &mockVoucherGuestService{receiptFn: tt.receiptFn}
			}
			d := Deps{GuestSvc: guestSvc}

			r.GET("/api/v1/front-desk/verify-voucher", VerifyVoucher(d))

			url := fmt.Sprintf("/api/v1/front-desk/verify-voucher?ref=%s&token=%s", tt.ref, tt.token)
			if tt.id != "" {
				url += "&id=" + tt.id
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantValid {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode JSON response: %v", err)
				}
				if resp["verification_status"] != "SIGNATURE_VERIFIED" {
					t.Errorf("expected verification_status SIGNATURE_VERIFIED, got %v", resp["verification_status"])
				}
				if resp["guest_name"] != sample.GuestDetails.Name {
					t.Errorf("expected guest_name %q, got %v", sample.GuestDetails.Name, resp["guest_name"])
				}
			}
		})
	}
}
