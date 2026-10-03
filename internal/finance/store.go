package finance

import (
	"context"
	"errors"
)

var (
	ErrInvalidAmount       = errors.New("finance: invalid refund amount")
	ErrOverRefund          = errors.New("finance: refund amount exceeds refundable balance")
	ErrBookingNotPaid      = errors.New("finance: booking is not in refundable status")
	ErrBookingNotFound     = errors.New("finance: booking not found")
	ErrCaseNotFound        = errors.New("finance: payment case not found")
	ErrCaseAlreadyResolved = errors.New("finance: payment case is already resolved")
	ErrRefundNotFound      = errors.New("finance: refund not found")
	ErrReasonRequired      = errors.New("finance: refund reason is required (min 5 chars)")
	ErrGatewayFailed       = errors.New("finance: payment gateway refund execution failed")
)

// Store mendefinisikan kontrak persistensi database untuk modul finance.
type Store interface {
	// GetRefundableBalance membaca sisa dana yang dapat di-refund dengan lock FOR UPDATE
	GetRefundableBalance(ctx context.Context, bookingID string) (capturedMinor, refundedMinor, remainingMinor int64, bookingStatus string, invoiceID string, err error)
	// CreateRefund mencatat refund baru di database
	CreateRefund(ctx context.Context, refund *PaymentRefund) error
	// UpdateRefundStatus memperbarui status refund setelah callback/response gateway
	UpdateRefundStatus(ctx context.Context, refundID, status, providerRefundID string) error
	// ListRefundsByBookingID mengambil seluruh riwayat refund untuk satu pemesanan
	ListRefundsByBookingID(ctx context.Context, bookingID string) ([]PaymentRefund, error)

	// CreatePaymentCase mencatat kasus anomali pembayaran baru
	CreatePaymentCase(ctx context.Context, pc *PaymentCase) error
	// GetPaymentCaseByID mengambil satu kasus berdasarkan ID
	GetPaymentCaseByID(ctx context.Context, caseID string) (*PaymentCase, error)
	// ListPaymentCases mengambil daftar kasus pembayaran dengan filter status
	ListPaymentCases(ctx context.Context, status string, limit int) ([]PaymentCase, error)
	// ResolvePaymentCase menandai kasus selesai dengan aksi dan catatan resolusi
	ResolvePaymentCase(ctx context.Context, caseID, action, notes, resolvedBy string) error

	// GetReconciliationSummary menghitung metrik agregasi kas
	GetReconciliationSummary(ctx context.Context) (*ReconciliationSummary, error)
	// VerifyBookingOwnership memvalidasi apakah booking ID milik email tamu tertentu
	VerifyBookingOwnership(ctx context.Context, bookingID, email string) (bool, error)
}
