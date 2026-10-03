package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

func TestResendNotifier_SendBookingConfirmed_TableTest(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		statusCode    int
		respBody      any
		expectErr     error
	}{
		{
			name:       "Successful email dispatch via Resend (HTTP 200)",
			apiKey:     "re_test_api_key_123",
			statusCode: http.StatusOK,
			respBody: map[string]any{
				"id": "email_resend_98765",
			},
			expectErr: nil,
		},
		{
			name:       "Resend API Error (HTTP 422 Unprocessable)",
			apiKey:     "re_test_api_key_123",
			statusCode: http.StatusUnprocessableEntity,
			respBody: map[string]any{
				"statusCode": 422,
				"name":       "validation_error",
				"message":    "Invalid recipient email",
			},
			expectErr: ErrEmailDispatchFailed,
		},
		{
			name:      "Unconfigured API key returns error immediately",
			apiKey:    "",
			expectErr: errors.New("resend: API key is not configured"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := booking.Booking{
				ID:              "01900000-0000-7000-8000-000000000001",
				GuestName:       "Budi Santoso",
				GuestEmail:      "budi@example.com",
				CheckIn:         time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
				CheckOut:        time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
				NumRooms:        1,
				TotalPriceMinor: 1100000,
				Currency:        "IDR",
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/emails" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				auth := r.Header.Get("Authorization")
				if auth != "Bearer "+tc.apiKey {
					t.Errorf("unexpected auth header: %s", auth)
				}
				idempKey := r.Header.Get("Idempotency-Key")
				expectedIdemp := fmt.Sprintf("email-confirmed-%s", b.ID)
				if idempKey != expectedIdemp {
					t.Errorf("unexpected Idempotency-Key: %s, want %s", idempKey, expectedIdemp)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(tc.respBody)
			}))
			defer server.Close()

			n := NewResend(server.URL, tc.apiKey, "Pulang ke Uttara <reservations@pulangkeuttara.com>", slog.Default())

			err := n.SendBookingConfirmed(context.Background(), b)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error containing '%v', got nil", tc.expectErr)
				}
				if errors.Is(tc.expectErr, ErrEmailDispatchFailed) && !errors.Is(err, ErrEmailDispatchFailed) {
					t.Fatalf("expected ErrEmailDispatchFailed, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestResendNotifier_SendGuestOTP_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		email      string
		otp        string
		statusCode int
		respBody   any
		expectErr  error
	}{
		{
			name:       "Successful OTP email dispatch (HTTP 200)",
			apiKey:     "re_test_api_key_123",
			email:      "tamu@example.com",
			otp:        "847291",
			statusCode: http.StatusOK,
			respBody: map[string]any{
				"id": "email_resend_otp_111",
			},
			expectErr: nil,
		},
		{
			name:       "Resend API Error (HTTP 422)",
			apiKey:     "re_test_api_key_123",
			email:      "invalid-email",
			otp:        "847291",
			statusCode: http.StatusUnprocessableEntity,
			respBody: map[string]any{
				"message": "Invalid recipient",
			},
			expectErr: ErrEmailDispatchFailed,
		},
		{
			name:      "Unconfigured API key",
			apiKey:    "",
			email:     "tamu@example.com",
			otp:       "847291",
			expectErr: errors.New("resend: API key is not configured"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/emails" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(tc.respBody)
			}))
			defer server.Close()

			resend := NewResend(server.URL, tc.apiKey, "Pulang ke Uttara <test@hotel.com>", slog.Default())
			err := resend.SendGuestOTP(context.Background(), tc.email, tc.otp)

			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.expectErr)
				}
				if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("expected err %v, got %v", tc.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

type mockFeatureChecker struct {
	enabled bool
}

func (m *mockFeatureChecker) IsEnabled(_ context.Context, _ string) bool {
	return m.enabled
}

func TestResendNotifier_FeatureFlagDisabled(t *testing.T) {
	checker := &mockFeatureChecker{enabled: false}
	resend := NewResend("http://localhost:9999", "resend_key", "from@hotel.com", slog.Default())
	resend.SetFeatureFlag(checker)

	// Harusnya skip dan return nil tanpa memanggil HTTP request
	b := booking.Booking{ID: "bk-ff-test", GuestEmail: "guest@example.com"}
	if err := resend.SendBookingConfirmed(context.Background(), b); err != nil {
		t.Errorf("expected nil error when feature flag is disabled, got %v", err)
	}

	if err := resend.SendGuestOTP(context.Background(), "guest@example.com", "123456"); err != nil {
		t.Errorf("expected nil error for SendGuestOTP when flag is disabled, got %v", err)
	}
}

