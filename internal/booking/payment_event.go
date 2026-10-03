package booking

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PaymentOutcomeStatus adalah status hasil pemrosesan event pembayaran.
type PaymentOutcomeStatus string

const (
	PaymentOutcomeConfirmed        PaymentOutcomeStatus = "CONFIRMED"
	PaymentOutcomeAlreadyConfirmed PaymentOutcomeStatus = "ALREADY_CONFIRMED"
	PaymentOutcomeCancelled        PaymentOutcomeStatus = "CANCELLED"
	PaymentOutcomeStaleIgnored     PaymentOutcomeStatus = "STALE_IGNORED"
	PaymentOutcomeUnhandledStatus  PaymentOutcomeStatus = "UNHANDLED_STATUS"
)

// PaymentOutcome adalah nilai kembalian dari ApplyPaymentEvent.
type PaymentOutcome struct {
	Status  PaymentOutcomeStatus
	Message string
}

// ApplyPaymentEvent memproses notifikasi webhook pembayaran secara idempoten dan aman (BE-R14, FR-15).
func (s *Service) ApplyPaymentEvent(ctx context.Context, event PaymentEvent) (PaymentOutcome, error) {
	if event.ExternalID == "" {
		event.ExternalID = event.BookingID
	}
	if event.ID == "" {
		event.ID = event.Reference
	}
	b, err := s.reader.Get(ctx, event.ExternalID)
	if err != nil {
		return PaymentOutcome{}, err
	}

	switch event.Status {
	case "PAID", "SETTLED":
		// Idempotent replay: jika sudah confirmed, langsung 200 OK tanpa error atau efek samping
		if b.Status == StatusConfirmed {
			return PaymentOutcome{
				Status:  PaymentOutcomeAlreadyConfirmed,
				Message: "booking already confirmed (idempotent replay)",
			}, nil
		}

		// Validasi Amount (BE-R14: cegah underpayment)
		if event.Amount != b.TotalPriceMinor {
			return PaymentOutcome{}, fmt.Errorf("%w: expected %d, got %d", ErrPaymentAmountMismatch, b.TotalPriceMinor, event.Amount)
		}

		// Validasi Currency (BE-R14)
		if event.Currency != "" && !strings.EqualFold(event.Currency, b.Currency) {
			return PaymentOutcome{}, fmt.Errorf("%w: expected %s, got %s", ErrPaymentCurrencyMismatch, b.Currency, event.Currency)
		}

		// Validasi Invoice ID terhadap buku besar PaymentAttempt (BE-R14)
		if s.attempts != nil {
			attempts, err := s.attempts.GetAttemptsByBookingID(ctx, event.ExternalID)
			if err != nil {
				return PaymentOutcome{}, fmt.Errorf("failed to get payment attempts: %w", err)
			}
			if len(attempts) > 0 {
				var hasRef, matched bool
				for _, att := range attempts {
					if att.ProviderReference != "" {
						hasRef = true
						if att.ProviderReference == event.ID {
							matched = true
							break
						}
					}
				}
				if hasRef && !matched {
					return PaymentOutcome{}, ErrInvoiceMismatch
				}
			}
		}

		if err := s.Confirm(ctx, event.ExternalID); err != nil {
			if errors.Is(err, ErrHoldExpired) {
				if s.latePayment != nil {
					_ = s.latePayment.CreateLatePaymentCase(ctx, event.ExternalID, event.ID, event.Amount, "Hold expired before payment arrived")
				}
				return PaymentOutcome{}, ErrHoldExpired
			}
			return PaymentOutcome{}, fmt.Errorf("failed to confirm booking: %w", err)
		}

		return PaymentOutcome{
			Status:  PaymentOutcomeConfirmed,
			Message: "booking confirmed",
		}, nil

	case "EXPIRED":
		// Proteksi out-of-order expiry (BE-R14): jangan batalkan booking yang sudah berstatus confirmed
		if b.Status == StatusConfirmed {
			return PaymentOutcome{
				Status:  PaymentOutcomeStaleIgnored,
				Message: "booking already confirmed, stale expiry event ignored",
			}, nil
		}

		if err := s.Cancel(ctx, event.ExternalID); err != nil {
			return PaymentOutcome{}, fmt.Errorf("failed to cancel booking: %w", err)
		}

		return PaymentOutcome{
			Status:  PaymentOutcomeCancelled,
			Message: "booking cancelled due to invoice expiry",
		}, nil

	default:
		return PaymentOutcome{
			Status:  PaymentOutcomeUnhandledStatus,
			Message: "unhandled status: " + event.Status,
		}, nil
	}
}
