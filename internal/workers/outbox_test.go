package workers

import (
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
