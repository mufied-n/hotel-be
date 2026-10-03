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
