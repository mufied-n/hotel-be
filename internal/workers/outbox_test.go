package workers

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 1, expected: 5 * time.Second},
		{attempt: 2, expected: 10 * time.Second},
		{attempt: 3, expected: 20 * time.Second},
		{attempt: 4, expected: 40 * time.Second},
		{attempt: 5, expected: 80 * time.Second},
		{attempt: 10, expected: 5 * time.Minute}, // capped at 5 minutes
		{attempt: 20, expected: 5 * time.Minute},
	}

	for _, c := range cases {
		got := backoff(c.attempt)
		if got != c.expected {
			t.Errorf("backoff(%d) = %v, want %v", c.attempt, got, c.expected)
		}
	}
}

func TestParsePayloadBookingID(t *testing.T) {
	// Valid
	validPayload := []byte(`{"booking_id":"bk-999","event":"booking.created"}`)
	id, err := ParsePayloadBookingID(validPayload)
	if err != nil || id != "bk-999" {
		t.Errorf("ParsePayloadBookingID valid = (%s, %v), want (bk-999, nil)", id, err)
	}

	// Invalid JSON
	_, err = ParsePayloadBookingID([]byte(`invalid-json`))
	if err == nil {
		t.Error("ParsePayloadBookingID invalid json want error")
	}

	// Empty booking_id
	_, err = ParsePayloadBookingID([]byte(`{"booking_id":"","other":"foo"}`))
	if err == nil {
		t.Error("ParsePayloadBookingID empty booking_id want error")
	}
}

func TestOutboxRelay_Run_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	relay := &OutboxRelay{
		BatchSize:   10,
		MaxAttempts: 3,
		Log:         slog.Default(),
	}

	// Should return immediately without hanging or panic
	relay.Run(ctx, 0)
}

func TestEnqueuer_Nil(t *testing.T) {
	var e *Enqueuer
	if err := e.EnqueueReleaseHold(context.Background(), "bk-1", 1*time.Minute); err != nil {
		t.Errorf("expected nil error on nil enqueuer, got %v", err)
	}
	if err := e.EnqueueNotify(context.Background(), "bk-1"); err != nil {
		t.Errorf("expected nil error on nil enqueuer, got %v", err)
	}

	e2 := &Enqueuer{Client: nil}
	if err := e2.EnqueueReleaseHold(context.Background(), "bk-1", 1*time.Minute); err != nil {
		t.Errorf("expected nil error on enqueuer with nil client, got %v", err)
	}
	if err := e2.EnqueueNotify(context.Background(), "bk-1"); err != nil {
		t.Errorf("expected nil error on enqueuer with nil client, got %v", err)
	}
}

type mockWorkerOTPNotifier struct {
	sentEmail       string
	sentOTP         string
	sentChallengeID string
	err             error
	callCount       int
}

func (m *mockWorkerOTPNotifier) SendGuestOTP(ctx context.Context, email, otpCode string, challengeID ...string) error {
	m.callCount++
	m.sentEmail = email
	m.sentOTP = otpCode
	if len(challengeID) > 0 {
		m.sentChallengeID = challengeID[0]
	}
	return m.err
}

func TestNewGuestOTPDispatchHandler_TableDriven(t *testing.T) {
	futureTime := time.Now().UTC().Add(10 * time.Minute)
	pastTime := time.Now().UTC().Add(-5 * time.Minute)

	tests := []struct {
		name              string
		payload           []byte
		notifierErr       error
		expectErr         bool
		expectCalls       int
		expectEmail       string
		expectOTP         string
		expectChallengeID string
	}{
		{
			name: "Valid active challenge dispatches OTP successfully",
			payload: []byte(`{
				"challenge_id": "ch-12345",
				"email": "tamu@example.com",
				"otp_code": "839102",
				"expires_at": "` + futureTime.Format(time.RFC3339) + `"
			}`),
			notifierErr:       nil,
			expectErr:         false,
			expectCalls:       1,
			expectEmail:       "tamu@example.com",
			expectOTP:         "839102",
			expectChallengeID: "ch-12345",
		},
		{
			name:              "Malformed JSON payload is safely discarded",
			payload:           []byte(`invalid-json`),
			notifierErr:       nil,
			expectErr:         false,
			expectCalls:       0,
			expectEmail:       "",
			expectOTP:         "",
			expectChallengeID: "",
		},
		{
			name: "Expired challenge is discarded without sending email",
			payload: []byte(`{
				"challenge_id": "ch-expired",
				"email": "stale@example.com",
				"otp_code": "111222",
				"expires_at": "` + pastTime.Format(time.RFC3339) + `"
			}`),
			notifierErr:       nil,
			expectErr:         false,
			expectCalls:       0,
			expectEmail:       "",
			expectOTP:         "",
			expectChallengeID: "",
		},
		{
			name: "Notifier network error returns error to trigger durable exponential backoff",
			payload: []byte(`{
				"challenge_id": "ch-retry",
				"email": "retry@example.com",
				"otp_code": "999888",
				"expires_at": "` + futureTime.Format(time.RFC3339) + `"
			}`),
			notifierErr:       errors.New("resend 504 gateway timeout"),
			expectErr:         true,
			expectCalls:       1,
			expectEmail:       "retry@example.com",
			expectOTP:         "999888",
			expectChallengeID: "ch-retry",
		},
		{
			name: "Nil notifier safely discards without error",
			payload: []byte(`{
				"challenge_id": "ch-no-notif",
				"email": "nonotif@example.com",
				"otp_code": "555444",
				"expires_at": "` + futureTime.Format(time.RFC3339) + `"
			}`),
			notifierErr:       nil,
			expectErr:         false,
			expectCalls:       0,
			expectEmail:       "",
			expectOTP:         "",
			expectChallengeID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notifier *mockWorkerOTPNotifier
			var optNotifier OTPNotifier
			if tt.name != "Nil notifier safely discards without error" {
				notifier = &mockWorkerOTPNotifier{err: tt.notifierErr}
				optNotifier = notifier
			}
			handler := NewGuestOTPDispatchHandler(nil, optNotifier, nil)

			err := handler(context.Background(), tt.payload)
			if tt.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if notifier != nil {
				if notifier.callCount != tt.expectCalls {
					t.Errorf("callCount = %d, want %d", notifier.callCount, tt.expectCalls)
				}
				if notifier.sentEmail != tt.expectEmail {
					t.Errorf("sentEmail = %q, want %q", notifier.sentEmail, tt.expectEmail)
				}
				if notifier.sentOTP != tt.expectOTP {
					t.Errorf("sentOTP = %q, want %q", notifier.sentOTP, tt.expectOTP)
				}
				if notifier.sentChallengeID != tt.expectChallengeID {
					t.Errorf("sentChallengeID = %q, want %q", notifier.sentChallengeID, tt.expectChallengeID)
				}
			}
		})
	}
}

