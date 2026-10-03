package payment

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

func TestXenditGateway_CreateCharge_TableTest(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		respBody      any
		expiresAt     *time.Time
		expectErr     error
		expectURL     string
		expectRef     string
	}{
		{
			name:       "Successful invoice creation (HTTP 201)",
			statusCode: http.StatusCreated,
			respBody: map[string]any{
				"id":          "inv_test_12345",
				"external_id": "01900000-0000-7000-8000-000000000001",
				"invoice_url": "https://checkout.xendit.co/web/inv_test_12345",
				"status":      "PENDING",
				"amount":      1100000,
			},
			expectErr: nil,
			expectURL: "https://checkout.xendit.co/web/inv_test_12345",
			expectRef: "inv_test_12345",
		},
		{
			name:       "API Error from Xendit (HTTP 400 Bad Request)",
			statusCode: http.StatusBadRequest,
			respBody: map[string]any{
				"error_code": "INVALID_AMOUNT",
				"message":    "Amount must be at least 10000 IDR",
			},
			expectErr: ErrInvoiceCreationFailed,
		},
		{
			name:       "Missing invoice_url in 200 response",
			statusCode: http.StatusOK,
			respBody: map[string]any{
				"id":     "inv_test_missing_url",
				"status": "PENDING",
			},
			expectErr: ErrInvoiceCreationFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verifikasi endpoint & Basic Auth
				if r.URL.Path != "/v2/invoices" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				user, pass, ok := r.BasicAuth()
				if !ok || user != "test_secret_key" || pass != "" {
					t.Errorf("invalid basic auth: user=%s, pass=%s, ok=%v", user, pass, ok)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(tc.respBody)
			}))
			defer server.Close()

			gateway := NewXendit(server.URL, "test_secret_key", "test_webhook_token", "http://localhost:3000", slog.Default())

			now := time.Now().UTC()
			exp := now.Add(25 * time.Minute)
			b := booking.Booking{
				ID:              "01900000-0000-7000-8000-000000000001",
				GuestEmail:      "tamu@example.com",
				TotalPriceMinor: 1100000,
				Currency:        "IDR",
				ExpiresAt:       &exp,
			}

			res, err := gateway.CreateCharge(context.Background(), b, b.TotalPriceMinor, b.Currency)
			if tc.expectErr != nil {
				if err == nil || !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error wrapping %v, got %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.PaymentURL != tc.expectURL {
					t.Errorf("PaymentURL = %s, want %s", res.PaymentURL, tc.expectURL)
				}
				if res.Reference != tc.expectRef {
					t.Errorf("Reference = %s, want %s", res.Reference, tc.expectRef)
				}
			}
		})
	}
}

func TestXenditGateway_VerifyWebhook_TableTest(t *testing.T) {
	gateway := NewXendit("https://api.xendit.co", "test_sec", "valid_callback_secret_token", "http://localhost:3000", nil)

	validPayload := []byte(`{
		"id": "6724a8f9024f92d42e316abc",
		"external_id": "01900000-0000-7000-8000-000000000001",
		"status": "PAID",
		"amount": 1100000,
		"payment_method": "QRIS",
		"paid_at": "2026-10-03T10:05:00.000Z"
	}`)

	tests := []struct {
		name          string
		gw            *XenditGateway
		headerToken   string
		body          []byte
		expectErr     error
		expectExtID   string
		expectStatus  string
	}{
		{
			name:         "Valid webhook token and valid PAID payload",
			gw:           gateway,
			headerToken:  "valid_callback_secret_token",
			body:         validPayload,
			expectErr:    nil,
			expectExtID:  "01900000-0000-7000-8000-000000000001",
			expectStatus: "PAID",
		},
		{
			name:         "Invalid webhook token returns ErrInvalidWebhookToken (anti-spoof)",
			gw:           gateway,
			headerToken:  "wrong_attacker_token",
			body:         validPayload,
			expectErr:    ErrInvalidWebhookToken,
		},
		{
			name:         "Empty header token returns ErrInvalidWebhookToken",
			gw:           gateway,
			headerToken:  "",
			body:         validPayload,
			expectErr:    ErrInvalidWebhookToken,
		},
		{
			name:         "Unconfigured server webhook token returns error",
			gw:           NewXendit("https://api.xendit.co", "test_sec", "", "http://localhost:3000", nil),
			headerToken:  "any_token",
			body:         validPayload,
			expectErr:    errors.New("xendit: webhook token is not configured"),
		},
		{
			name:         "Malformed JSON body returns unmarshal error",
			gw:           gateway,
			headerToken:  "valid_callback_secret_token",
			body:         []byte("{invalid-json"),
			expectErr:    errors.New("xendit: parse webhook payload"),
		},
		{
			name:         "Missing external_id in payload returns error",
			gw:           gateway,
			headerToken:  "valid_callback_secret_token",
			body:         []byte(`{"id": "inv_123", "status": "PAID"}`),
			expectErr:    errors.New("xendit: webhook payload missing external_id"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := tc.gw.VerifyWebhook(tc.headerToken, tc.body)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error containing '%v', got nil", tc.expectErr)
				}
				if errors.Is(tc.expectErr, ErrInvalidWebhookToken) && !errors.Is(err, ErrInvalidWebhookToken) {
					t.Fatalf("expected ErrInvalidWebhookToken, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if payload.ExternalID != tc.expectExtID {
					t.Errorf("ExternalID = %s, want %s", payload.ExternalID, tc.expectExtID)
				}
				if payload.Status != tc.expectStatus {
					t.Errorf("Status = %s, want %s", payload.Status, tc.expectStatus)
				}
			}
		})
	}
}
