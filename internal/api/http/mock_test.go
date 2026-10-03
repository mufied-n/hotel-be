package http

import (
	"context"
	"time"

	"github.com/example/hotel-booking/internal/guest"
)

type mockGuestService struct {
	requestChallengeFunc    func(ctx context.Context, email string) (int, error)
	verifyChallengeFunc     func(ctx context.Context, email, code string) (string, *guest.GuestSession, error)
	validateSessionFunc     func(ctx context.Context, rawToken string) (*guest.GuestSession, error)
	revokeSessionFunc       func(ctx context.Context, rawToken string) error
	getProfileFunc          func(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error)
	listBookingsFunc        func(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error)
	getBookingDetailFunc    func(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error)
	getBookingReceiptFunc   func(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error)
	generateCalendarICSFunc func(receipt *guest.ReceiptDTO) ([]byte, error)
}

func (m *mockGuestService) RequestChallenge(ctx context.Context, email string) (int, error) {
	if m.requestChallengeFunc != nil {
		return m.requestChallengeFunc(ctx, email)
	}
	return 60, nil
}

func (m *mockGuestService) VerifyChallenge(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
	if m.verifyChallengeFunc != nil {
		return m.verifyChallengeFunc(ctx, email, code)
	}
	return "gst_sess_mock_token_123", &guest.GuestSession{
		ID:         "sess_01",
		GuestEmail: email,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

func (m *mockGuestService) ValidateSession(ctx context.Context, rawToken string) (*guest.GuestSession, error) {
	if m.validateSessionFunc != nil {
		return m.validateSessionFunc(ctx, rawToken)
	}
	if rawToken == "gst_sess_valid" {
		return &guest.GuestSession{
			ID:         "sess_valid",
			GuestEmail: "tamu@example.com",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}, nil
	}
	return nil, guest.ErrSessionNotFound
}

func (m *mockGuestService) RevokeSession(ctx context.Context, rawToken string) error {
	if m.revokeSessionFunc != nil {
		return m.revokeSessionFunc(ctx, rawToken)
	}
	return nil
}

func (m *mockGuestService) GetSessionProfile(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error) {
	if m.getProfileFunc != nil {
		return m.getProfileFunc(ctx, session)
	}
	return &guest.ProfileView{
		Email:               session.GuestEmail,
		ActiveBookingsCount: 1,
		LastActiveAt:        time.Now(),
		ExpiresAt:           session.ExpiresAt,
	}, nil
}

func (m *mockGuestService) ListBookings(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error) {
	if m.listBookingsFunc != nil {
		return m.listBookingsFunc(ctx, email, status, limit)
	}
	return []guest.BookingSummary{
		{
			ID:              "bk_001",
			RoomTypeName:    "Deluxe Premier",
			Status:          "confirmed",
			TotalPriceMinor: 1500000,
			Currency:        "IDR",
		},
	}, nil
}

func (m *mockGuestService) GetBookingDetail(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error) {
	if m.getBookingDetailFunc != nil {
		return m.getBookingDetailFunc(ctx, email, bookingID)
	}
	if bookingID == "bk_001" && email == "tamu@example.com" {
		return &guest.BookingDetail{
			ID:              "bk_001",
			GuestEmail:      email,
			GuestName:       "Budi Santoso",
			RoomTypeName:    "Deluxe Premier",
			Status:          "confirmed",
			TotalPriceMinor: 1500000,
			Currency:        "IDR",
			AllowedActions: guest.AllowedActions{
				CanCancel:          true,
				CanDownloadReceipt: true,
			},
		}, nil
	}
	return nil, guest.ErrBookingNotFound
}

func (m *mockGuestService) GetBookingReceipt(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error) {
	if m.getBookingReceiptFunc != nil {
		return m.getBookingReceiptFunc(ctx, email, bookingID)
	}
	if bookingID == "bk_001" && email == "tamu@example.com" {
		return &guest.ReceiptDTO{
			InvoiceNumber:    "INV/PKU/202610/BK001",
			BookingID:        "bk_001",
			BookingReference: "PKU-20261003-BK001",
			Status:           "confirmed",
		}, nil
	}
	return nil, guest.ErrBookingNotFound
}

func (m *mockGuestService) GenerateCalendarICS(receipt *guest.ReceiptDTO) ([]byte, error) {
	if m.generateCalendarICSFunc != nil {
		return m.generateCalendarICSFunc(receipt)
	}
	return []byte("BEGIN:VCALENDAR\nEND:VCALENDAR"), nil
}
