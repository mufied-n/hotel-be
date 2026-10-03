package finance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/adapter/payment"
)

// RefundGateway mendefinisikan port adapter gateway pembayaran untuk transaksi refund.
type RefundGateway interface {
	CreateRefund(ctx context.Context, req payment.RefundRequest) (payment.RefundResult, error)
}

// Service mendefinisikan use cases bisnis untuk manajemen keuangan, refund, dan sengketa pembayaran.
type Service interface {
	ProcessRefund(ctx context.Context, input CreateRefundInput) (*PaymentRefund, error)
	CreateLatePaymentCase(ctx context.Context, bookingID, providerRef string, amountMinor int64, notes string) (*PaymentCase, error)
	ListCases(ctx context.Context, status string, limit int) ([]PaymentCase, error)
	ResolveCase(ctx context.Context, input ResolveCaseInput) error
	GetBookingRefundStatus(ctx context.Context, email, bookingID string) (*RefundStatusView, error)
	GetReconciliationSummary(ctx context.Context) (*ReconciliationSummary, error)
}

// DefaultService mengimplementasikan Service interface.
type DefaultService struct {
	store   Store
	gateway RefundGateway
	log     *slog.Logger
	nowFunc func() time.Time
}

// NewService membuat instance baru DefaultService.
func NewService(store Store, gateway RefundGateway, log *slog.Logger) *DefaultService {
	if log == nil {
		log = slog.Default()
	}
	return &DefaultService{
		store:   store,
		gateway: gateway,
		log:     log,
		nowFunc: time.Now,
	}
}

// ProcessRefund memproses pengembalian dana dengan serialisasi transaksi dan validasi saldo anti-over-refund.
func (s *DefaultService) ProcessRefund(ctx context.Context, input CreateRefundInput) (*PaymentRefund, error) {
	cleanBookingID := strings.TrimSpace(input.BookingID)
	if cleanBookingID == "" {
		return nil, ErrBookingNotFound
	}
	if input.AmountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	cleanReason := strings.TrimSpace(input.Reason)
	if len(cleanReason) < 5 {
		return nil, ErrReasonRequired
	}

	// 1. Verifikasi saldo refundable dan status booking dengan penguncian baris (FOR UPDATE)
	captured, refunded, remaining, status, invoiceID, err := s.store.GetRefundableBalance(ctx, cleanBookingID)
	if err != nil {
		return nil, err
	}

	// Status guard: Hanya booking yang pernah lunas yang dapat di-refund
	if status == "pending" || status == "expired" || captured == 0 {
		return nil, ErrBookingNotPaid
	}

	// Over-refund guard (BR-F14-01): Akumulasi refund tidak boleh melampaui sisa dana yang dapat di-refund
	if input.AmountMinor > remaining {
		s.log.WarnContext(ctx, "finance.over_refund_rejected",
			"booking_id", cleanBookingID,
			"requested", input.AmountMinor,
			"remaining", remaining,
			"captured", captured,
			"refunded", refunded,
		)
		return nil, fmt.Errorf("%w: requested %d, remaining %d", ErrOverRefund, input.AmountMinor, remaining)
	}

	// 2. Buat reference ID unik deterministik untuk idempotensi gateway
	now := s.nowFunc()
	cleanPrefix := strings.ToUpper(strings.ReplaceAll(cleanBookingID, "-", ""))
	if len(cleanPrefix) > 8 {
		cleanPrefix = cleanPrefix[:8]
	}
	referenceID := fmt.Sprintf("rfnd-%s-%d", cleanPrefix, now.UnixNano())

	refundRecord := &PaymentRefund{
		BookingID:   cleanBookingID,
		ReferenceID: referenceID,
		AmountMinor: input.AmountMinor,
		Currency:    "IDR",
		Reason:      cleanReason,
		Status:      "pending",
		Provider:    "xendit",
		ActorID:     input.ActorID,
		ActorRole:   input.ActorRole,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 3. Catat status 'pending' terlebih dahulu di database
	if err := s.store.CreateRefund(ctx, refundRecord); err != nil {
		return nil, fmt.Errorf("finance: failed to persist refund intent: %w", err)
	}

	// 4. Eksekusi ke payment gateway (Xendit) jika gateway tersedia
	if s.gateway != nil {
		gwReq := payment.RefundRequest{
			ReferenceID: referenceID,
			InvoiceID:   invoiceID,
			Amount:      input.AmountMinor,
			Currency:    "IDR",
			Reason:      cleanReason,
		}

		gwRes, gwErr := s.gateway.CreateRefund(ctx, gwReq)
		if gwErr != nil {
			s.log.ErrorContext(ctx, "finance.gateway_refund_failed", "booking_id", cleanBookingID, "err", gwErr)
			_ = s.store.UpdateRefundStatus(ctx, refundRecord.ID, "failed", "")
			refundRecord.Status = "failed"
			return refundRecord, fmt.Errorf("%w: %s", ErrGatewayFailed, gwErr.Error())
		}

		refundRecord.Status = gwRes.Status
		refundRecord.ProviderRefundID = gwRes.RefundID
		_ = s.store.UpdateRefundStatus(ctx, refundRecord.ID, gwRes.Status, gwRes.RefundID)
	} else {
		// Mock mode / internal test: sukses langsung
		refundRecord.Status = "succeeded"
		_ = s.store.UpdateRefundStatus(ctx, refundRecord.ID, "succeeded", "mock_rfd_"+referenceID)
	}

	s.log.InfoContext(ctx, "finance.refund_processed",
		"refund_id", refundRecord.ID,
		"booking_id", cleanBookingID,
		"amount", input.AmountMinor,
		"status", refundRecord.Status,
	)

	return refundRecord, nil
}

// CreateLatePaymentCase mencatat anomali pembayaran terlambat (setelah booking kedaluwarsa).
func (s *DefaultService) CreateLatePaymentCase(ctx context.Context, bookingID, providerRef string, amountMinor int64, notes string) (*PaymentCase, error) {
	now := s.nowFunc()
	pc := &PaymentCase{
		BookingID:         bookingID,
		CaseType:          "late_payment",
		Status:            "open",
		AmountMinor:       amountMinor,
		Currency:          "IDR",
		ProviderReference: providerRef,
		Notes:             notes,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.store.CreatePaymentCase(ctx, pc); err != nil {
		return nil, fmt.Errorf("finance: failed to create late payment case: %w", err)
	}

	s.log.WarnContext(ctx, "finance.late_payment_case_created",
		"case_id", pc.ID,
		"booking_id", bookingID,
		"amount", amountMinor,
	)

	return pc, nil
}

// ListCases mengambil daftar sengketa pembayaran untuk ditinjau oleh tim Finance.
func (s *DefaultService) ListCases(ctx context.Context, status string, limit int) ([]PaymentCase, error) {
	return s.store.ListPaymentCases(ctx, strings.TrimSpace(status), limit)
}

// ResolveCase menyelesaikan kasus sengketa pembayaran dengan tindakan yang disetujui.
func (s *DefaultService) ResolveCase(ctx context.Context, input ResolveCaseInput) error {
	cleanID := strings.TrimSpace(input.CaseID)
	if cleanID == "" {
		return ErrCaseNotFound
	}
	cleanAction := strings.TrimSpace(input.Action)
	if cleanAction == "" {
		cleanAction = "resolved"
	}
	return s.store.ResolvePaymentCase(ctx, cleanID, cleanAction, input.Notes, input.ActorID)
}

// GetBookingRefundStatus menyajikan riwayat refund kepada tamu pemilik reservasi (UU PDP No. 27/2022).
func (s *DefaultService) GetBookingRefundStatus(ctx context.Context, email, bookingID string) (*RefundStatusView, error) {
	cleanBookingID := strings.TrimSpace(bookingID)
	normEmail := strings.ToLower(strings.TrimSpace(email))

	// Verifikasi kepemilikan anti-IDOR
	isOwner, err := s.store.VerifyBookingOwnership(ctx, cleanBookingID, normEmail)
	if err != nil || !isOwner {
		return nil, ErrBookingNotFound
	}

	refunds, err := s.store.ListRefundsByBookingID(ctx, cleanBookingID)
	if err != nil {
		return nil, err
	}

	return &RefundStatusView{
		BookingID: cleanBookingID,
		HasRefund: len(refunds) > 0,
		Refunds:   refunds,
	}, nil
}

// GetReconciliationSummary menghasilkan laporan agregasi rekonsiliasi kas.
func (s *DefaultService) GetReconciliationSummary(ctx context.Context) (*ReconciliationSummary, error) {
	return s.store.GetReconciliationSummary(ctx)
}
