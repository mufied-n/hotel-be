package booking

import (
	"context"
	"time"
)

// PaymentAttempt mencatat percobaan pembayaran di gateway untuk rekonsiliasi finansial (BE-G11, PCI-DSS SAQ A).
type PaymentAttempt struct {
	ID                string         `json:"id"`
	BookingID         string         `json:"booking_id"`
	Provider          string         `json:"provider"`
	ProviderReference string         `json:"provider_reference"`
	AmountMinor       int64          `json:"amount_minor"`
	Currency          string         `json:"currency"`
	Status            string         `json:"status"` // initiated, success, failed, unknown_timeout, received_after_expiry
	Payload           map[string]any `json:"payload,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// PaymentAttemptStore adalah port repository untuk buku besar pembayaran.
type PaymentAttemptStore interface {
	RecordAttempt(ctx context.Context, attempt PaymentAttempt) error
	UpdateAttemptStatus(ctx context.Context, bookingID string, status string) error
	UpdateAttemptByID(ctx context.Context, attemptID string, status string, providerReference string, payload map[string]any) error
	GetAttemptsByBookingID(ctx context.Context, bookingID string) ([]PaymentAttempt, error)
}
