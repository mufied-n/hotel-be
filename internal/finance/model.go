package finance

import (
	"time"
)

// PaymentRefund mencatat transaksi pengembalian dana resmi ke tamu (PCI-DSS SAQ A).
type PaymentRefund struct {
	ID                 string    `json:"id"`
	BookingID          string    `json:"booking_id"`
	PaymentAttemptID   *string   `json:"payment_attempt_id,omitempty"`
	ReferenceID        string    `json:"reference_id"`
	AmountMinor        int64     `json:"amount_minor"`
	Currency           string    `json:"currency"`
	Reason             string    `json:"reason"`
	Status             string    `json:"status"` // pending, succeeded, failed
	Provider           string    `json:"provider"`
	ProviderRefundID   string    `json:"provider_refund_id,omitempty"`
	ActorID            string    `json:"actor_id"`
	ActorRole          string    `json:"actor_role"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// PaymentCase mencatat anomali rekonsiliasi pembayaran (late payment, mismatch).
type PaymentCase struct {
	ID                string     `json:"id"`
	BookingID         string     `json:"booking_id"`
	CaseType          string     `json:"case_type"` // late_payment, amount_mismatch, duplicate_payment
	Status            string     `json:"status"`    // open, investigating, resolved, dismissed
	AmountMinor       int64      `json:"amount_minor"`
	Currency          string     `json:"currency"`
	ProviderReference string     `json:"provider_reference"`
	Notes             string     `json:"notes"`
	ResolvedBy        string     `json:"resolved_by,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	ResolutionAction  string     `json:"resolution_action,omitempty"` // refunded, reallocated_room, manual_adjustment
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CreateRefundInput adalah parameter untuk mengajukan refund dana.
type CreateRefundInput struct {
	BookingID   string `json:"booking_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
	Notes       string `json:"notes,omitempty"`
	ActorID     string `json:"actor_id"`
	ActorRole   string `json:"actor_role"`
}

// ResolveCaseInput adalah parameter untuk menyelesaikan sengketa/kasus pembayaran.
type ResolveCaseInput struct {
	CaseID   string `json:"case_id"`
	Action   string `json:"action"` // refund, reallocate, dismiss
	Notes    string `json:"notes"`
	ActorID  string `json:"actor_id"`
}

// RefundStatusView menyajikan ringkasan refund kepada tamu terverifikasi (UU PDP No. 27/2022).
type RefundStatusView struct {
	BookingID string          `json:"booking_id"`
	HasRefund bool            `json:"has_refund"`
	Refunds   []PaymentRefund `json:"refunds"`
}

// ReconciliationSummary adalah ringkasan agregasi audit rekonsiliasi kas.
type ReconciliationSummary struct {
	TotalSettledMinor  int64 `json:"total_settled_minor"`
	TotalRefundedMinor int64 `json:"total_refunded_minor"`
	NetCapturedMinor   int64 `json:"net_captured_minor"`
	OpenCasesCount     int   `json:"open_cases_count"`
	TotalRefundsCount  int   `json:"total_refunds_count"`
}
