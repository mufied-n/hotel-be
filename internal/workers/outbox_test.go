package workers

import (
	"context"
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

