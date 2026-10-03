package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/rates"
)

func TestXenditWebhook_TableTest(t *testing.T) {
	const webhookSecretToken = "test_xendit_webhook_secret_123"

	// Helper setup
	setupRouter := func(status booking.Status, holdPast bool, withGateway bool, withAttempts bool) (http.Handler, *booking.Booking) {
		exp := time.Now().UTC().Add(30 * time.Minute)
		if holdPast {
			exp = time.Now().UTC().Add(-10 * time.Minute)
		}
		b := &booking.Booking{
			ID:              "01900000-0000-7000-8000-000000000099",
			RoomTypeID:      "01900000-0000-7000-8000-000000000001",
			CheckIn:         time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour),
			CheckOut:        time.Now().UTC().Truncate(24 * time.Hour).Add(48 * time.Hour),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Tamu Uji Webhook",
			GuestEmail:      "webhook@test.com",
			Status:          status,
			TotalPriceMinor: 1100000,
			Currency:        "IDR",
			ExpiresAt:       &exp,
		}

		tx := &webhookMockTx{
			bookings: map[string]*booking.Booking{b.ID: b},
		}
		reader := &webhookMockReader{bookings: map[string]*booking.Booking{b.ID: b}}
		svc := booking.NewService(tx, nil, nil, nil, nil, reader, 30*time.Minute, nil)

		if withAttempts {
			svc.SetPaymentAttemptStore(&webhookMockAttemptStore{
				attempts: map[string][]booking.PaymentAttempt{
					b.ID: {
						{
							ID:                "att-1",
							BookingID:         b.ID,
							Provider:          "xendit",
							ProviderReference: "inv_wh_test_123",
							AmountMinor:       1100000,
							Currency:          "IDR",
							Status:            "pending",
						},
					},
				},
			})
		}

		var gw *payment.XenditGateway
		if withGateway {
			gw = payment.NewXendit("https://api.xendit.co", "test_sec", webhookSecretToken, "http://localhost:3000", nil)
		}

		r := NewRouter(Deps{
			StaffAuth:     TestStaffVerifier(),
			BookingSvc:    svc,
			XenditGateway: gw,
		})
		return r, b
	}

	tests := []struct {
		name              string
		initialStatus     booking.Status
		holdPast          bool
		withGateway       bool
		withAttempts      bool
		headerToken       string
		payloadID         string
		payloadExtID      string
		payloadStatus     string
		payloadAmount     int64
		payloadCurrency   string
		expectCode        int
		expectFinalStatus booking.Status
	}{
		{
			name:              "Valid webhook PAID confirms pending booking (200 OK)",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusOK,
			expectFinalStatus: booking.StatusConfirmed,
		},
		{
			name:              "Idempotent replay on already confirmed booking returns 200 OK",
			initialStatus:     booking.StatusConfirmed,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusOK,
			expectFinalStatus: booking.StatusConfirmed,
		},
		{
			name:              "Amount mismatch rejected with 422 PAYMENT_AMOUNT_MISMATCH (BE-R14)",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     500000, // Underpayment
			payloadCurrency:   "IDR",
			expectCode:        http.StatusUnprocessableEntity,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Currency mismatch rejected with 422 PAYMENT_CURRENCY_MISMATCH (BE-R14)",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "USD", // Wrong currency
			expectCode:        http.StatusUnprocessableEntity,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Invoice ID mismatch rejected with 422 INVOICE_ID_MISMATCH (BE-R14)",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_forged_999", // Does not match recorded attempt
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusUnprocessableEntity,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Out-of-order EXPIRED on confirmed booking ignored safely (200 OK, BE-R14)",
			initialStatus:     booking.StatusConfirmed,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "EXPIRED",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusOK,
			expectFinalStatus: booking.StatusConfirmed, // Not cancelled!
		},
		{
			name:              "Payment on expired hold returns 409 HOLD_EXPIRED",
			initialStatus:     booking.StatusPending,
			holdPast:          true,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusConflict,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Webhook EXPIRED cancels pending booking (200 OK)",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "EXPIRED",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusOK,
			expectFinalStatus: booking.StatusCancelled,
		},
		{
			name:              "Invalid webhook token rejected with 401 Unauthorized",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      true,
			headerToken:       "wrong_token",
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusUnauthorized,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Gateway not configured returns 501 Not Implemented",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       false,
			withAttempts:      false,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusNotImplemented,
			expectFinalStatus: booking.StatusPending,
		},
		{
			name:              "Booking not found returns 404 BOOKING_NOT_FOUND",
			initialStatus:     booking.StatusPending,
			holdPast:          false,
			withGateway:       true,
			withAttempts:      false,
			headerToken:       webhookSecretToken,
			payloadID:         "inv_wh_test_123",
			payloadExtID:      "00000000-0000-0000-0000-000000000000", // Non-existent
			payloadStatus:     "PAID",
			payloadAmount:     1100000,
			payloadCurrency:   "IDR",
			expectCode:        http.StatusNotFound,
			expectFinalStatus: booking.StatusPending,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router, b := setupRouter(tc.initialStatus, tc.holdPast, tc.withGateway, tc.withAttempts)

			extID := b.ID
			if tc.payloadExtID != "" {
				extID = tc.payloadExtID
			}

			body, _ := json.Marshal(map[string]any{
				"id":             tc.payloadID,
				"external_id":    extID,
				"status":         tc.payloadStatus,
				"amount":         tc.payloadAmount,
				"currency":       tc.payloadCurrency,
				"payment_method": "QRIS",
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/xendit", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.headerToken != "" {
				req.Header.Set("x-callback-token", tc.headerToken)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("expected HTTP status %d, got %d. Body: %s", tc.expectCode, w.Code, w.Body.String())
			}

			if tc.payloadExtID == "" && b.Status != tc.expectFinalStatus {
				t.Errorf("expected booking status %s, got %s", tc.expectFinalStatus, b.Status)
			}
		})
	}
}

// Minimal webhookMockTx for router webhook testing
type webhookMockTx struct {
	bookings map[string]*booking.Booking
}

func (m *webhookMockTx) InTx(_ context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	return fn(m, m)
}
func (m *webhookMockTx) LockAndDecrement(_ context.Context, _ string, _, _ time.Time, _ int) error {
	return nil
}
func (m *webhookMockTx) Increment(_ context.Context, _ string, _, _ time.Time, _ int) error {
	return nil
}
func (m *webhookMockTx) InsertBookingWithHold(_ context.Context, b *booking.Booking, _ []rates.Quote, _ time.Time) error {
	m.bookings[b.ID] = b
	return nil
}
func (m *webhookMockTx) UpdateStatus(_ context.Context, id string, to booking.Status) error {
	if b, ok := m.bookings[id]; ok {
		b.Status = to
	}
	return nil
}
func (m *webhookMockTx) GetForUpdate(_ context.Context, id string) (booking.Booking, error) {
	if b, ok := m.bookings[id]; ok {
		return *b, nil
	}
	return booking.Booking{}, booking.ErrNotFound
}
func (m *webhookMockTx) PickAndAssignRooms(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	return []string{"101"}, nil
}
func (m *webhookMockTx) GetRoomAssignments(_ context.Context, _ string) ([]string, error) {
	return []string{"101"}, nil
}
func (m *webhookMockTx) PublishTx(_ context.Context, _ string, _ []byte) error { return nil }

type webhookMockReader struct {
	bookings map[string]*booking.Booking
}

func (r *webhookMockReader) Get(_ context.Context, id string) (booking.Booking, error) {
	if b, ok := r.bookings[id]; ok {
		return *b, nil
	}
	return booking.Booking{}, booking.ErrNotFound
}

type webhookMockAttemptStore struct {
	attempts map[string][]booking.PaymentAttempt
}

func (s *webhookMockAttemptStore) RecordAttempt(_ context.Context, _ booking.PaymentAttempt) error {
	return nil
}
func (s *webhookMockAttemptStore) UpdateAttemptStatus(_ context.Context, _ string, _ string) error {
	return nil
}
func (s *webhookMockAttemptStore) UpdateAttemptByID(_ context.Context, _ string, _ string, _ string, _ map[string]any) error {
	return nil
}
func (s *webhookMockAttemptStore) GetAttemptsByBookingID(_ context.Context, bookingID string) ([]booking.PaymentAttempt, error) {
	return s.attempts[bookingID], nil
}
