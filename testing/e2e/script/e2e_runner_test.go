package script

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/notifier"
	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/rates"
)

type e2eIncrementCall struct {
	RoomTypeID string
	From       time.Time
	To         time.Time
	NumRooms   int
}

type e2eTxMock struct {
	booking       booking.Booking
	rooms         []string
	increments    []e2eIncrementCall
	otpNotifier   *e2eOTPNotifier
	guestStore    *e2eGuestStore
	finStore      *e2eFinanceStore
}

func (m *e2eTxMock) LockAndDecrement(_ context.Context, _ string, _, _ time.Time, _ int) error {
	return nil
}
func (m *e2eTxMock) Increment(_ context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	m.increments = append(m.increments, e2eIncrementCall{
		RoomTypeID: roomTypeID,
		From:       from,
		To:         to,
		NumRooms:   numRooms,
	})
	return nil
}
func (m *e2eTxMock) InsertBookingWithHold(_ context.Context, b *booking.Booking, _ []rates.Quote, holdExpiresAt time.Time) error {
	b.ID = "bk-e2e-001"
	b.Status = booking.StatusPending
	b.ExpiresAt = &holdExpiresAt
	m.booking = *b
	return nil
}
func (m *e2eTxMock) GetForUpdate(_ context.Context, id string) (booking.Booking, error) {
	return m.booking, nil
}
func (m *e2eTxMock) UpdateStatus(_ context.Context, _ string, to booking.Status) error {
	m.booking.Status = to
	return nil
}
func (m *e2eTxMock) PickAndAssignRooms(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	return m.rooms, nil
}
func (m *e2eTxMock) GetRoomAssignments(_ context.Context, _ string) ([]string, error) {
	return m.rooms, nil
}
func (m *e2eTxMock) PublishTx(_ context.Context, _ string, _ []byte) error { return nil }

type e2eTxRunner struct{ tx *e2eTxMock }

func (r *e2eTxRunner) InTx(_ context.Context, fn func(booking.InventoryTx, booking.EventPublisher) error) error {
	return fn(r.tx, r.tx)
}

type e2eReaderMock struct{ tx *e2eTxMock }

func (m *e2eReaderMock) Get(_ context.Context, _ string) (booking.Booking, error) {
	return m.tx.booking, nil
}

type e2ePayMock struct{}

func (m *e2ePayMock) CreateCharge(_ context.Context, _ booking.Booking, _ int64, _ string) (booking.ChargeResult, error) {
	return booking.ChargeResult{PaymentURL: "http://pay.hotel.test/charge/123", Reference: "ref-e2e-001"}, nil
}

type e2eNotifierMock struct{}

func (m *e2eNotifierMock) SendBookingConfirmed(_ context.Context, _ booking.Booking) error {
	return nil
}

type e2eGuestStore struct {
	challenges map[string]*guest.Challenge
	sessions   map[string]*guest.GuestSession
	tx         *e2eTxMock
}

func newE2EGuestStore(tx *e2eTxMock) *e2eGuestStore {
	return &e2eGuestStore{
		challenges: make(map[string]*guest.Challenge),
		sessions:   make(map[string]*guest.GuestSession),
		tx:         tx,
	}
}

func (s *e2eGuestStore) CreateChallenge(ctx context.Context, c *guest.Challenge) error {
	c.ID = "ch-e2e-001"
	s.challenges[c.Email] = c
	return nil
}

func (s *e2eGuestStore) GetLatestActiveChallenge(ctx context.Context, email string) (*guest.Challenge, error) {
	return s.challenges[email], nil
}

func (s *e2eGuestStore) UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error {
	for _, c := range s.challenges {
		if c.ID == id {
			c.Attempts = attempts
		}
	}
	return nil
}

func (s *e2eGuestStore) MarkChallengeVerified(ctx context.Context, id string, verifiedAt time.Time) error {
	for _, c := range s.challenges {
		if c.ID == id {
			c.VerifiedAt = &verifiedAt
		}
	}
	return nil
}

func (s *e2eGuestStore) CreateSession(ctx context.Context, sess *guest.GuestSession) error {
	sess.ID = "sess-e2e-001"
	s.sessions[sess.TokenHash] = sess
	return nil
}

func (s *e2eGuestStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*guest.GuestSession, error) {
	return s.sessions[tokenHash], nil
}

func (s *e2eGuestStore) TouchSession(ctx context.Context, id string, lastActiveAt, expiresAt time.Time) error {
	for _, sess := range s.sessions {
		if sess.ID == id {
			sess.LastActiveAt = lastActiveAt
			sess.ExpiresAt = expiresAt
		}
	}
	return nil
}

func (s *e2eGuestStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	delete(s.sessions, tokenHash)
	return nil
}

func (s *e2eGuestStore) CountActiveBookingsByEmail(ctx context.Context, email string) (int, error) {
	if s.tx.booking.GuestEmail == email {
		return 1, nil
	}
	return 0, nil
}

func (s *e2eGuestStore) ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error) {
	if s.tx.booking.GuestEmail == email {
		return []guest.BookingSummary{
			{
				ID:              s.tx.booking.ID,
				RoomTypeID:      s.tx.booking.RoomTypeID,
				RoomTypeName:    "Deluxe Premier",
				CheckIn:         s.tx.booking.CheckIn.Format("2006-01-02"),
				CheckOut:        s.tx.booking.CheckOut.Format("2006-01-02"),
				NumRooms:        s.tx.booking.NumRooms,
				NumGuests:       s.tx.booking.NumGuests,
				Status:          string(s.tx.booking.Status),
				TotalPriceMinor: s.tx.booking.TotalPriceMinor,
				Currency:        s.tx.booking.Currency,
				CreatedAt:       s.tx.booking.CreatedAt,
			},
		}, nil
	}
	return []guest.BookingSummary{}, nil
}

func (s *e2eGuestStore) GetBookingDetailByEmail(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error) {
	if s.tx.booking.GuestEmail == email && s.tx.booking.ID == bookingID {
		return &guest.BookingDetail{
			ID:              s.tx.booking.ID,
			RoomTypeID:      s.tx.booking.RoomTypeID,
			RoomTypeName:    "Deluxe Premier",
			CheckIn:         s.tx.booking.CheckIn.Format("2006-01-02"),
			CheckOut:        s.tx.booking.CheckOut.Format("2006-01-02"),
			NumRooms:        s.tx.booking.NumRooms,
			NumGuests:       s.tx.booking.NumGuests,
			Status:          string(s.tx.booking.Status),
			TotalPriceMinor: s.tx.booking.TotalPriceMinor,
			Currency:        s.tx.booking.Currency,
			GuestName:       s.tx.booking.GuestName,
			GuestEmail:      s.tx.booking.GuestEmail,
			GuestPhone:      s.tx.booking.GuestPhone,
			CreatedAt:       s.tx.booking.CreatedAt,
		}, nil
	}
	return nil, nil
}

func (s *e2eGuestStore) GetBookingReceiptData(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error) {
	if s.tx.booking.GuestEmail == email && s.tx.booking.ID == bookingID {
		return &guest.ReceiptDTO{
			InvoiceNumber:    "INV/PKU/202610/BKE2E001",
			InvoiceDate:      s.tx.booking.CreatedAt.Format(time.RFC3339),
			BookingID:        s.tx.booking.ID,
			BookingReference: "PKU-20261003-BKE2E001",
			Status:           string(s.tx.booking.Status),
			HotelInfo: guest.HotelInfo{
				Name:    "Pulang ke Uttara",
				Address: "Jl. Kaliurang Km 5.6 No. 1, Yogyakarta",
				Phone:   "+62 274 5022888",
			},
			StayDetails: guest.StayDetails{
				CheckInDate:  s.tx.booking.CheckIn.Format("2006-01-02"),
				CheckInTime:  "14:00 WIB",
				CheckOutDate: s.tx.booking.CheckOut.Format("2006-01-02"),
				CheckOutTime: "12:00 WIB",
				TotalNights:  2,
				Timezone:     "Asia/Jakarta",
			},
			GuestDetails: guest.GuestDetails{
				Name:      s.tx.booking.GuestName,
				Email:     s.tx.booking.GuestEmail,
				Phone:     s.tx.booking.GuestPhone,
				NumRooms:  s.tx.booking.NumRooms,
				NumGuests: s.tx.booking.NumGuests,
			},
			RoomItem: guest.RoomItemReceipt{
				RoomTypeID:       s.tx.booking.RoomTypeID,
				RoomTypeName:     "Deluxe Premier",
				RatePlanCode:     "BB",
				MealPlan:         "Sarapan Termasuk (Breakfast Included)",
				NumRooms:         s.tx.booking.NumRooms,
				TotalNights:      2,
				SubtotalMinor:    s.tx.booking.TotalPriceMinor,
				NightlyRateMinor: s.tx.booking.TotalPriceMinor / 2,
			},
			PricingBreakdown: guest.PricingBreakdown{
				Currency:        s.tx.booking.Currency,
				TotalPriceMinor: s.tx.booking.TotalPriceMinor,
			},
			PaymentSummary: guest.PaymentSummary{
				Status:   "PAID",
				Provider: "Xendit",
			},
			Policies: guest.PoliciesReceipt{
				CheckInPolicy:      "Wajib KTP/Paspor saat check-in.",
				CancellationPolicy: "Fleksibel sebelum H-1 14:00 WIB.",
			},
			QRPayload: "https://pulangkeuttara.id/verify/booking/" + s.tx.booking.ID,
		}, nil
	}
	return nil, nil
}


type e2eOTPNotifier struct {
	lastOTP string
}

func (n *e2eOTPNotifier) SendGuestOTP(ctx context.Context, email, otpCode string) error {
	n.lastOTP = otpCode
	return nil
}


type e2eFinanceStore struct {
	tx        *e2eTxMock
	refunds   map[string][]finance.PaymentRefund
	cases     map[string]*finance.PaymentCase
	refundSeq int
	caseSeq   int
}

func newE2EFinanceStore(tx *e2eTxMock) *e2eFinanceStore {
	return &e2eFinanceStore{
		tx:      tx,
		refunds: make(map[string][]finance.PaymentRefund),
		cases:   make(map[string]*finance.PaymentCase),
	}
}

func (s *e2eFinanceStore) GetRefundableBalance(ctx context.Context, bookingID string) (capturedMinor, refundedMinor, remainingMinor int64, bookingStatus string, invoiceID string, err error) {
	if s.tx.booking.ID != bookingID {
		return 0, 0, 0, "", "", finance.ErrBookingNotFound
	}
	var totalRefunded int64
	for _, r := range s.refunds[bookingID] {
		if r.Status == "succeeded" || r.Status == "pending" {
			totalRefunded += r.AmountMinor
		}
	}
	remaining := s.tx.booking.TotalPriceMinor - totalRefunded
	if remaining < 0 {
		remaining = 0
	}
	return s.tx.booking.TotalPriceMinor, totalRefunded, remaining, string(s.tx.booking.Status), "inv_e2e_12345", nil
}

func (s *e2eFinanceStore) CreateRefund(ctx context.Context, r *finance.PaymentRefund) error {
	s.refundSeq++
	r.ID = fmt.Sprintf("rfnd-e2e-%03d", s.refundSeq)
	s.refunds[r.BookingID] = append(s.refunds[r.BookingID], *r)
	return nil
}

func (s *e2eFinanceStore) UpdateRefundStatus(ctx context.Context, refundID, status, providerRefundID string) error {
	for bID, list := range s.refunds {
		for i, r := range list {
			if r.ID == refundID {
				list[i].Status = status
				list[i].ProviderRefundID = providerRefundID
				s.refunds[bID] = list
				return nil
			}
		}
	}
	return finance.ErrRefundNotFound
}

func (s *e2eFinanceStore) ListRefundsByBookingID(ctx context.Context, bookingID string) ([]finance.PaymentRefund, error) {
	return s.refunds[bookingID], nil
}

func (s *e2eFinanceStore) CreatePaymentCase(ctx context.Context, pc *finance.PaymentCase) error {
	s.caseSeq++
	pc.ID = fmt.Sprintf("case-e2e-%03d", s.caseSeq)
	s.cases[pc.ID] = pc
	return nil
}

func (s *e2eFinanceStore) GetPaymentCaseByID(ctx context.Context, caseID string) (*finance.PaymentCase, error) {
	pc, ok := s.cases[caseID]
	if !ok {
		return nil, finance.ErrCaseNotFound
	}
	return pc, nil
}

func (s *e2eFinanceStore) ListPaymentCases(ctx context.Context, status string, limit int) ([]finance.PaymentCase, error) {
	var results []finance.PaymentCase
	for _, pc := range s.cases {
		if status == "" || pc.Status == status {
			results = append(results, *pc)
		}
	}
	return results, nil
}

func (s *e2eFinanceStore) ResolvePaymentCase(ctx context.Context, caseID, action, notes, resolvedBy string) error {
	pc, ok := s.cases[caseID]
	if !ok {
		return finance.ErrCaseNotFound
	}
	if pc.Status == "resolved" {
		return finance.ErrCaseAlreadyResolved
	}
	pc.Status = "resolved"
	pc.ResolutionAction = action
	pc.Notes = pc.Notes + " | " + notes
	pc.ResolvedBy = resolvedBy
	now := time.Now()
	pc.ResolvedAt = &now
	return nil
}

func (s *e2eFinanceStore) GetReconciliationSummary(ctx context.Context) (*finance.ReconciliationSummary, error) {
	var totalRefunded int64
	var countRefunds int
	for _, list := range s.refunds {
		for _, r := range list {
			if r.Status == "succeeded" {
				totalRefunded += r.AmountMinor
				countRefunds++
			}
		}
	}
	var openCases int
	for _, pc := range s.cases {
		if pc.Status == "open" {
			openCases++
		}
	}
	return &finance.ReconciliationSummary{
		TotalSettledMinor:  150_000_000,
		TotalRefundedMinor: totalRefunded,
		NetCapturedMinor:   150_000_000 - totalRefunded,
		OpenCasesCount:     openCases,
		TotalRefundsCount:  countRefunds,
	}, nil
}

func (s *e2eFinanceStore) VerifyBookingOwnership(ctx context.Context, bookingID, email string) (bool, error) {
	return s.tx.booking.ID == bookingID && s.tx.booking.GuestEmail == email, nil
}

func setupE2ETestServer(t *testing.T) (*httptest.Server, *e2eTxMock) {
	t.Helper()

	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "guest", "/api/v1/catalog/rooms", "GET"},
		{"p", "guest", "/api/v1/catalog/rooms/:id", "GET"},
		{"p", "guest", "/api/v1/search", "GET"},
		{"p", "guest", "/api/v1/quotes", "POST"},
		{"p", "guest", "/api/v1/bookings", "POST"},
		{"p", "guest", "/api/v1/bookings/:id", "GET"},
		{"p", "guest", "/api/v1/bookings/:id/cancel", "POST"},
		{"p", "guest", "/fake-pay/:ref", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/no-show", "POST"},
		{"p", "housekeeping", "/api/v1/rooms/housekeeping", "GET"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms", "POST"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms/:id", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "finance", "/api/v1/finance/refunds", "POST"},
		{"p", "finance", "/api/v1/finance/cases", "GET"},
		{"p", "finance", "/api/v1/finance/cases/:id/resolve", "POST"},
		{"p", "finance", "/api/v1/finance/reconciliations", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
		{"g", "revenue_mgr", "guest"},
	}

	enforcer, err := auth.NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	now := time.Now()
	tx := &e2eTxMock{
		booking: booking.Booking{
			ID:              "bk-e2e-001",
			Status:          booking.StatusPending,
			RoomTypeID:      "01900000-0000-7000-8000-000000000001",
			CheckIn:         now,
			CheckOut:        now.Add(48 * time.Hour),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Budi Santoso",
			GuestEmail:      "budi@example.com",
			GuestToken:      "gst_e2e_secret_token_123",
			TotalPriceMinor: 1_100_000,
		},
		rooms: []string{"301"},
	}

	runner := &e2eTxRunner{tx: tx}
	inv := &mockInventoryStore{
		avail: []inventory.Availability{
			{Date: now, TotalRooms: 20, AvailableRooms: 10},
			{Date: now.Add(24 * time.Hour), TotalRooms: 20, AvailableRooms: 10},
		},
	}
	ratesSvc := &mockRateProvider{
		quotes: []rates.Quote{
			{Date: now, RateMinor: 550_000},
			{Date: now.Add(24 * time.Hour), RateMinor: 550_000},
		},
	}

	bkSvc := booking.NewService(
		runner,
		inv,
		ratesSvc,
		&e2ePayMock{},
		&e2eNotifierMock{},
		&e2eReaderMock{tx: tx},
		30*time.Minute,
		nil,
	)

	rateEngine := rates.NewEngine(map[string]int64{
		"01900000-0000-7000-8000-000000000001": 550_000,
	}, 1.25)
	quoteStore := rateEngine.QuoteStore()
	bkSvc.SetQuoteStore(quoteStore)

	xenditGw := payment.NewXendit("https://api.xendit.co", "test_xendit_sec", "test_e2e_xendit_webhook_token", "http://localhost:3000", nil)

	guestStore := newE2EGuestStore(tx)
	guestNotifier := &e2eOTPNotifier{}
	tx.otpNotifier = guestNotifier
	tx.guestStore = guestStore
	guestSvc := guest.NewService(guestStore, guestNotifier, nil)

	finStore := newE2EFinanceStore(tx)
	tx.finStore = finStore
	finSvc := finance.NewService(finStore, nil, nil)

	handler := api.NewRouter(api.Deps{
		BookingSvc:    bkSvc,
		InvStore:      inv,
		RateSvc:       ratesSvc,
		RateEngine:    rateEngine,
		QuoteStore:    quoteStore,
		Enforcer:      enforcer,
		IsDevelopment: true,
		XenditGateway: xenditGw,
		GuestSvc:      guestSvc,
		FinanceSvc:    finSvc,
		ReadyCheck:    func(ctx context.Context) error { return nil },
		FakePay: func(w http.ResponseWriter, r *http.Request) {
			bID := r.URL.Query().Get("booking_id")
			if bID == "" {
				bID = "bk-e2e-001"
			}
			if err := bkSvc.Confirm(r.Context(), bID); err != nil {
				if errors.Is(err, booking.ErrHoldExpired) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"error":"hold has expired, room availability was released","code":"HOLD_EXPIRED"}`))
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"confirmed"}`))
		},
	})

	srv := httptest.NewServer(handler)
	return srv, tx
}

type mockInventoryStore struct {
	avail []inventory.Availability
}

func (m *mockInventoryStore) GetByDate(_ context.Context, _ string, _, _ time.Time) ([]inventory.Availability, error) {
	return m.avail, nil
}

type mockRateProvider struct {
	quotes []rates.Quote
}

func (m *mockRateProvider) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	return m.quotes, nil
}

func TestEndToEndHotelBookingRBACLifecycle(t *testing.T) {
	srv, tx := setupE2ETestServer(t)
	defer srv.Close()

	client := srv.Client()
	var createdGuestToken string

	// 1. Healthz probe
	t.Run("E2E-01: Health check", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("healthz status = %d, want 200", res.StatusCode)
		}
	})

	// 2. Public Availability Search
	t.Run("E2E-02: Public search availability", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/availability?room_type_id=01900000-0000-7000-8000-000000000001&check_in=2026-10-10&check_out=2026-10-12")
		if err != nil {
			t.Fatalf("availability request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("availability status = %d, want 200", res.StatusCode)
		}
	})

	// 2A. Public Catalog Room Discovery (BE-G01)
	t.Run("E2E-02A: Public catalog room discovery (7 sellable variants, 95 rooms)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/catalog/rooms")
		if err != nil {
			t.Fatalf("catalog request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("catalog status = %d, want 200", res.StatusCode)
		}
		var catalogResp struct {
			Total int `json:"total"`
			Rooms []struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"rooms"`
		}
		if err := json.NewDecoder(res.Body).Decode(&catalogResp); err != nil {
			t.Fatalf("catalog decode failed: %v", err)
		}
		if catalogResp.Total != 7 || len(catalogResp.Rooms) != 7 {
			t.Errorf("catalog total = %d, want 7 sellable variants", catalogResp.Total)
		}
	})

	// 2B. Multi-night Cross-Variant Search Engine (BE-G02, BE-G03)
	t.Run("E2E-02B: Multi-night cross-variant search", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("search status = %d, want 200", res.StatusCode)
		}
		var searchResp struct {
			TotalVariants  int `json:"total_variants"`
			AvailableCount int `json:"available_count"`
			Results        []struct {
				Available       bool  `json:"available"`
				AvailableRooms  int   `json:"available_rooms"`
				TotalPriceMinor int64 `json:"total_price_minor"`
			} `json:"results"`
		}
		if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil {
			t.Fatalf("search decode failed: %v", err)
		}
		if searchResp.TotalVariants != 7 {
			t.Errorf("total_variants = %d, want 7", searchResp.TotalVariants)
		}
		if searchResp.AvailableCount == 0 {
			t.Errorf("available_count = 0, want > 0")
		}
	})

	// 2C. Search Validation Reject Exceeding Stay (BE-G03: LOS > 30 nights)
	t.Run("E2E-02C: Search validation reject stay > 30 nights (400 Bad Request)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-11-20&adults=2&rooms=1")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("search validation status = %d, want 400", res.StatusCode)
		}
	})

	// 2D. Search Validation Reject Invalid Child Age (BE-G03: Age > 17)
	t.Run("E2E-02D: Search validation reject child age > 17 (400 Bad Request)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&child_ages=19")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("search child age status = %d, want 400", res.StatusCode)
		}
	})

	var createdVariantID string

	// 2E. Revenue Manager creates a new room variant (POST /api/v1/catalog/rooms)
	t.Run("E2E-02E: Revenue Manager creates room variant (201 Created)", func(t *testing.T) {
		payload := []byte(`{
			"code": "villa-garden",
			"name": "Garden Villa",
			"family_name": "Villa",
			"bed_type": "1 King Bed",
			"room_size_sqm": 85,
			"max_capacity": 4,
			"max_adults": 2,
			"max_children": 2,
			"base_price_minor": 2500000,
			"description": "Private villa with lush tropical garden view.",
			"amenities": ["Private Pool", "Free Wi-Fi"],
			"photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
		}`)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/catalog/rooms", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("create variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create variant status = %d, want 201", res.StatusCode)
		}
		var created struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		}
		if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
			t.Fatalf("decode created variant failed: %v", err)
		}
		if created.ID == "" || created.Code != "villa-garden" {
			t.Fatalf("unexpected created variant: %+v", created)
		}
		createdVariantID = created.ID
	})

	// 2F. Revenue Manager updates room variant (PUT /api/v1/catalog/rooms/:id)
	t.Run("E2E-02F: Revenue Manager updates room variant (200 OK)", func(t *testing.T) {
		payload := []byte(`{
			"code": "villa-garden",
			"name": "Garden Villa Deluxe",
			"family_name": "Villa",
			"bed_type": "1 King Bed",
			"room_size_sqm": 85,
			"max_capacity": 4,
			"max_adults": 2,
			"max_children": 2,
			"base_price_minor": 2750000,
			"description": "Private villa with lush tropical garden view and floating breakfast.",
			"amenities": ["Private Pool", "Free Wi-Fi", "Floating Breakfast"],
			"photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
		}`)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("update variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("update variant status = %d, want 200", res.StatusCode)
		}
		var updated struct {
			Name           string `json:"name"`
			BasePriceMinor int64  `json:"base_price_minor"`
		}
		if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
			t.Fatalf("decode updated variant failed: %v", err)
		}
		if updated.Name != "Garden Villa Deluxe" || updated.BasePriceMinor != 2750000 {
			t.Errorf("unexpected updated variant: %+v", updated)
		}
	})

	// 2G. Public Guest views single room variant (GET /api/v1/catalog/rooms/:id)
	t.Run("E2E-02G: Public Guest views single room variant (200 OK)", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/catalog/rooms/" + createdVariantID)
		if err != nil {
			t.Fatalf("get single variant failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("get single variant status = %d, want 200", res.StatusCode)
		}
		var v struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			BasePriceMinor int64  `json:"base_price_minor"`
		}
		if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
			t.Fatalf("decode single variant failed: %v", err)
		}
		if v.ID != createdVariantID || v.BasePriceMinor != 2750000 {
			t.Errorf("expected updated price 2750000, got %+v", v)
		}
	})

	// 2H. Catalog RBAC Negative Tests (Guest cannot POST/PUT/DELETE, Revenue Mgr cannot DELETE)
	t.Run("E2E-02H: Catalog RBAC Negative Tests (403 Forbidden)", func(t *testing.T) {
		// Guest cannot POST
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/catalog/rooms", bytes.NewReader([]byte(`{"code":"hack"}`)))
		req.Header.Set("Content-Type", "application/json")
		res, _ := client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest post catalog status = %d, want 403", res.StatusCode)
		}

		// Guest cannot PUT
		req, _ = http.NewRequest(http.MethodPut, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, bytes.NewReader([]byte(`{"name":"hack"}`)))
		req.Header.Set("Content-Type", "application/json")
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest put catalog status = %d, want 403", res.StatusCode)
		}

		// Guest cannot DELETE
		req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest delete catalog status = %d, want 403", res.StatusCode)
		}

		// Revenue Manager cannot DELETE
		req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		req.Header.Set("X-User-Role", "revenue_mgr")
		res, _ = client.Do(req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("revenue_mgr delete catalog status = %d, want 403", res.StatusCode)
		}
	})

	// 2I. GM Admin deletes room variant (DELETE /api/v1/catalog/rooms/:id)
	t.Run("E2E-02I: GM Admin deletes room variant (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/catalog/rooms/"+createdVariantID, nil)
		req.Header.Set("Authorization", "Bearer gm_admin")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("gm_admin delete variant request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("delete variant status = %d, want 200", res.StatusCode)
		}

		// Verify subsequent GET returns 404 Not Found
		res, err = client.Get(srv.URL + "/api/v1/catalog/rooms/" + createdVariantID)
		if err != nil {
			t.Fatalf("subsequent get variant failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("deleted variant status = %d, want 404", res.StatusCode)
		}
	})

	// 3. Public Create Booking
	t.Run("E2E-03: Public create booking hold", func(t *testing.T) {
		payload := []byte(`{
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "2026-10-10",
			"check_out": "2026-10-12",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Budi Santoso",
			"guest_email": "budi@example.com"
		}`)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("booking request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Errorf("create booking status = %d, want 201", res.StatusCode)
		}
		var resp struct {
			GuestAccessToken string `json:"guest_access_token"`
		}
		_ = json.NewDecoder(res.Body).Decode(&resp)
		createdGuestToken = resp.GuestAccessToken
	})

	// 4. RBAC Negative Test: Public Guest CANNOT check-in
	t.Run("E2E-04: Guest cannot check-in (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("check-in request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("guest check-in status = %d, want 403 Forbidden", res.StatusCode)
		}
	})

	// 5. Payment Confirmation Simulation
	t.Run("E2E-05: Payment confirmation", func(t *testing.T) {
		res, err := client.Post(srv.URL+"/fake-pay/ref-e2e", "application/json", nil)
		if err != nil {
			t.Fatalf("payment request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("payment status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusConfirmed {
			t.Errorf("booking status after payment = %s, want confirmed", tx.booking.Status)
		}
	})

	// 6. Receptionist Check-In
	t.Run("E2E-06: Receptionist check-in (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		req.Header.Set("X-User-Role", "receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("receptionist check-in request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("receptionist check-in status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusCheckedIn {
			t.Errorf("booking status = %s, want checked_in", tx.booking.Status)
		}
	})

	// 7. Housekeeping cannot check-out
	t.Run("E2E-07: Housekeeping cannot check-out (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		req.Header.Set("X-User-Role", "housekeeping")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("housekeeping check-out request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("housekeeping check-out status = %d, want 403 Forbidden", res.StatusCode)
		}
	})

	// 8. Receptionist Check-Out via Bearer Token
	t.Run("E2E-08: Receptionist check-out via Bearer token (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("receptionist check-out request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("receptionist check-out status = %d, want 200", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusCheckedOut {
			t.Errorf("booking status = %s, want checked_out", tx.booking.Status)
		}
	})

	// 9. GM Admin Wildcard Inspection
	t.Run("E2E-09: GM Admin inspect booking (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		req.Header.Set("X-User-Role", "gm_admin")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("gm_admin request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("gm_admin status = %d, want 200", res.StatusCode)
		}

		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["id"] != "bk-e2e-001" {
			t.Errorf("expected booking ID bk-e2e-001, got %v", body["id"])
		}
	})

	// 10. BE-G13: Public guest receives masked PublicDTO (no PII leakage)
	t.Run("E2E-10: Public guest receives masked PublicDTO (no PII)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if _, exists := body["guest_name"]; exists {
			t.Errorf("PII leaked! guest_name found: %v", body["guest_name"])
		}
		if _, exists := body["guest_email"]; exists {
			t.Errorf("PII leaked! guest_email found: %v", body["guest_email"])
		}
		if _, exists := body["guest_token"]; exists {
			t.Errorf("PII leaked! guest_token found: %v", body["guest_token"])
		}
		if body["id"] != "bk-e2e-001" {
			t.Errorf("expected id bk-e2e-001, got %v", body["id"])
		}
	})

	// 11. BE-G13: Guest with valid X-Guest-Token receives full PII
	t.Run("E2E-11: Guest with X-Guest-Token receives full PII", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001", nil)
		req.Header.Set("X-Guest-Token", createdGuestToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["guest_name"] != "Budi Santoso" {
			t.Errorf("expected guest_name 'Budi Santoso', got %v", body["guest_name"])
		}
		if body["guest_email"] != "budi@example.com" {
			t.Errorf("expected guest_email 'budi@example.com', got %v", body["guest_email"])
		}
	})

	// 12. BE-G13: Guest with invalid token cannot cancel booking (403 Forbidden)
	t.Run("E2E-12: Guest with invalid token cannot cancel booking (403 Forbidden)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/cancel", nil)
		req.Header.Set("X-Guest-Token", "invalid_token_xyz")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("cancel status = %d, want 403 Forbidden", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "FORBIDDEN_OWNERSHIP" {
			t.Errorf("expected error code FORBIDDEN_OWNERSHIP, got %s", pd.Code)
		}
	})

	// 13. BE-G10: Production mode gates /fake-pay (404 Not Found)
	t.Run("E2E-13: Production mode gates /fake-pay (404 Not Found)", func(t *testing.T) {
		prodHandler := api.NewRouter(api.Deps{
			Enforcer:      auth.DefaultTestEnforcer(),
			IsDevelopment: false,
			FakePay: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		})
		prodSrv := httptest.NewServer(prodHandler)
		defer prodSrv.Close()

		res, err := client.Post(prodSrv.URL+"/fake-pay/ref-e2e", "application/json", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("production fake-pay status = %d, want 404", res.StatusCode)
		}
	})

	checkIn := "2026-10-10"
	checkOut := "2026-10-12"
	var lockedPromoQuote rates.LockedQuote

	// 14. BE-G04, BE-G05, BE-G06: Guest requests locked quote with BB and OCTOBREAK promo
	t.Run("E2E-14: Guest requests locked quote with BB and OCTOBREAK promo", func(t *testing.T) {
		payload := `{
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"rate_plan_code": "bed_and_breakfast",
			"check_in": "` + checkIn + `",
			"check_out": "` + checkOut + `",
			"num_rooms": 1,
			"num_guests": 2,
			"promo_code": "OCTOBREAK"
		}`
		res, err := client.Post(srv.URL+"/api/v1/quotes", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&lockedPromoQuote); err != nil {
			t.Fatalf("decode quote failed: %v", err)
		}
		if lockedPromoQuote.CancellationCode != rates.PolicyNonRefundable {
			t.Errorf("expected non_refundable policy for promo quote, got %s", lockedPromoQuote.CancellationCode)
		}
		if lockedPromoQuote.Pricing.DiscountMinor <= 0 {
			t.Errorf("expected discount > 0, got %d", lockedPromoQuote.Pricing.DiscountMinor)
		}
		if lockedPromoQuote.Pricing.BreakfastChargeMinor != 400_000 {
			t.Errorf("expected breakfast charge 400_000, got %d", lockedPromoQuote.Pricing.BreakfastChargeMinor)
		}
	})

	// 15. BE-G19: Guest attempts to book with quote but omits consent (400 CONSENT_REQUIRED)
	t.Run("E2E-15: Guest attempts to book without consent (400 CONSENT_REQUIRED)", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"quote_id": "%s",
			"terms_accepted": false,
			"privacy_accepted": true,
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "%s",
			"check_out": "%s",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Siti Rahma",
			"guest_email": "siti@example.com"
		}`, lockedPromoQuote.ID, checkIn, checkOut)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "CONSENT_REQUIRED" {
			t.Errorf("expected code CONSENT_REQUIRED, got %s", pd.Code)
		}
	})

	var promoBookingID string
	var promoGuestToken string

	// 16. BE-G06, BE-G19: Guest creates booking with locked quote snapshot and consent (201 Created)
	t.Run("E2E-16: Guest creates booking with locked quote snapshot and consent", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"quote_id": "%s",
			"terms_accepted": true,
			"privacy_accepted": true,
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "%s",
			"check_out": "%s",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Siti Rahma",
			"guest_email": "siti@example.com"
		}`, lockedPromoQuote.ID, checkIn, checkOut)
		res, err := client.Post(srv.URL+"/api/v1/bookings", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		bMap := body["booking"].(map[string]any)
		promoBookingID = bMap["id"].(string)
		promoGuestToken = body["guest_access_token"].(string)
		if int64(bMap["total_price_minor"].(float64)) != lockedPromoQuote.Pricing.TotalPriceMinor {
			t.Errorf("locked price mismatch: got %v, want %d", bMap["total_price_minor"], lockedPromoQuote.Pricing.TotalPriceMinor)
		}
	})

	// 17. BE-G08: Confirmed promo booking cannot be cancelled by guest (409 NON_REFUNDABLE_BOOKING)
	t.Run("E2E-17: Confirmed promo booking cannot be cancelled by guest (409 Conflict)", func(t *testing.T) {
		// Konfirmasi booking promo terlebih dahulu
		payRes, err := client.Post(srv.URL+"/fake-pay/ref-promo?booking_id="+promoBookingID, "application/json", nil)
		if err != nil {
			t.Fatalf("fake-pay failed: %v", err)
		}
		if payRes.StatusCode != http.StatusOK {
			t.Fatalf("fake-pay status = %d, want 200", payRes.StatusCode)
		}

		// Update status mock booking di tx agar confirmed
		tx.booking.Status = booking.StatusConfirmed

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/"+promoBookingID+"/cancel", nil)
		req.Header.Set("X-Guest-Token", promoGuestToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("cancel request failed: %v", err)
		}
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409 Conflict", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "NON_REFUNDABLE_BOOKING" {
			t.Errorf("expected code NON_REFUNDABLE_BOOKING, got %s", pd.Code)
		}
	})

	var batchDBookingID string
	var batchDGuestToken string
	checkoutPayload := `{
		"room_type_id": "01900000-0000-7000-8000-000000000001",
		"check_in": "` + checkIn + `",
		"check_out": "` + checkOut + `",
		"num_rooms": 1,
		"num_guests": 2,
		"guest_name": "Rian Kusuma",
		"guest_email": "rian@example.com",
		"guest_phone": "+6281298765432",
		"estimated_arrival_time": "14:30",
		"special_requests": "High floor, non-smoking, quiet room"
	}`

	// 18. BE-G07, BE-G12: Guest checkout with complete profile (E.164, arrival time, special requests, server_time)
	t.Run("E2E-18: Complete guest profile checkout returns 201 with expires_at & server_time", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings", strings.NewReader(checkoutPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "ik-e2e-rian-001")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("checkout request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("checkout status = %d, want 201 Created", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["expires_at"] == nil {
			t.Errorf("expected expires_at in response, got nil")
		}
		if body["server_time"] == nil {
			t.Errorf("expected server_time in response, got nil")
		}
		bMap := body["booking"].(map[string]any)
		batchDBookingID = bMap["id"].(string)
		batchDGuestToken = body["guest_access_token"].(string)
		if bMap["estimated_arrival_time"] != "14:30" {
			t.Errorf("expected arrival time 14:30, got %v", bMap["estimated_arrival_time"])
		}
		if bMap["special_requests"] != "High floor, non-smoking, quiet room" {
			t.Errorf("expected special requests match, got %v", bMap["special_requests"])
		}
	})

	// 19. BE-G09: Idempotency-Key network replay returns 201 with Idempotency-Replayed: true
	t.Run("E2E-19: Idempotency-Key network retry returns 201 with Idempotency-Replayed: true", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings", strings.NewReader(checkoutPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "ik-e2e-rian-001")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("replay request failed: %v", err)
		}
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("replay status = %d, want 201", res.StatusCode)
		}
		if res.Header.Get("Idempotency-Replayed") != "true" {
			t.Errorf("expected Idempotency-Replayed header = true, got %q", res.Header.Get("Idempotency-Replayed"))
		}
	})

	// 20. BE-G09: Idempotency-Key conflict with payload mismatch returns 409 IDEMPOTENCY_CONFLICT
	t.Run("E2E-20: Idempotency-Key conflict with payload mismatch returns 409 IDEMPOTENCY_CONFLICT", func(t *testing.T) {
		mismatchedPayload := `{
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "` + checkIn + `",
			"check_out": "` + checkOut + `",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Totally Different Person",
			"guest_email": "different@example.com"
		}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings", strings.NewReader(mismatchedPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "ik-e2e-rian-001")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("conflict request failed: %v", err)
		}
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409 Conflict", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "IDEMPOTENCY_CONFLICT" {
			t.Errorf("expected code IDEMPOTENCY_CONFLICT, got %s", pd.Code)
		}
	})

	// 21. BE-G12: Late payment arrival after hold expires returns 409 HOLD_EXPIRED
	t.Run("E2E-21: Late payment arrival after hold expires returns 409 HOLD_EXPIRED", func(t *testing.T) {
		// Simulasikan hold expired dengan menggeser expires_at ke masa lalu
		pastExpiry := time.Now().Add(-10 * time.Minute)
		tx.booking.ExpiresAt = &pastExpiry
		tx.booking.Status = booking.StatusPending

		payRes, err := client.Post(srv.URL+"/fake-pay/ref-late?booking_id="+batchDBookingID, "application/json", nil)
		if err != nil {
			t.Fatalf("late payment request failed: %v", err)
		}
		if payRes.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for expired hold payment, got %d", payRes.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(payRes.Body).Decode(&pd)
		if pd.Code != "HOLD_EXPIRED" {
			t.Errorf("expected code HOLD_EXPIRED, got %s", pd.Code)
		}
	})

	// 22. BE-G13, UU PDP No. 27/2022: Public booking query omits guest phone, email, name; auth with token includes them
	t.Run("E2E-22: UU PDP privacy enforcement masks guest phone and email in public view", func(t *testing.T) {
		tx.booking.GuestName = "Rian Kusuma"
		tx.booking.GuestEmail = "rian@example.com"
		tx.booking.GuestPhone = "+6281298765432"
		tx.booking.GuestToken = batchDGuestToken
		tx.booking.Status = booking.StatusConfirmed

		// 1. Unauthenticated public query -> PII is masked
		pubReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/"+batchDBookingID, nil)
		pubRes, err := client.Do(pubReq)
		if err != nil {
			t.Fatalf("public query failed: %v", err)
		}
		if pubRes.StatusCode != http.StatusOK {
			t.Fatalf("public query status = %d, want 200", pubRes.StatusCode)
		}
		var pubBody map[string]any
		_ = json.NewDecoder(pubRes.Body).Decode(&pubBody)
		if _, exists := pubBody["guest_phone"]; exists {
			t.Errorf("guest_phone MUST NOT be returned to unauthenticated caller")
		}
		if _, exists := pubBody["guest_email"]; exists {
			t.Errorf("guest_email MUST NOT be returned to unauthenticated caller")
		}
		if _, exists := pubBody["guest_name"]; exists {
			t.Errorf("guest_name MUST NOT be returned to unauthenticated caller")
		}

		// 2. Query with valid X-Guest-Token -> PII returned to legitimate owner
		authReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/"+batchDBookingID, nil)
		authReq.Header.Set("X-Guest-Token", batchDGuestToken)
		authRes, err := client.Do(authReq)
		if err != nil {
			t.Fatalf("auth query failed: %v", err)
		}
		if authRes.StatusCode != http.StatusOK {
			t.Fatalf("auth query status = %d, want 200", authRes.StatusCode)
		}
		var authBody map[string]any
		_ = json.NewDecoder(authRes.Body).Decode(&authBody)
		if authBody["guest_phone"] != "+6281298765432" {
			t.Errorf("expected guest_phone = +6281298765432, got %v", authBody["guest_phone"])
		}
		if authBody["guest_email"] != "rian@example.com" {
			t.Errorf("expected guest_email = rian@example.com, got %v", authBody["guest_email"])
		}
	})

	// 23. BE-G22: Early check-out restitutes remaining inventory nights
	t.Run("E2E-23: Early check-out restitutes remaining inventory nights (200 OK)", func(t *testing.T) {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		tx.booking.Status = booking.StatusCheckedIn
		tx.booking.CheckIn = today.Add(-24 * time.Hour) // stayed yesterday night
		tx.booking.CheckOut = today.Add(48 * time.Hour)  // scheduled 2 nights ahead
		tx.increments = nil

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("checkout request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 OK", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusCheckedOut {
			t.Errorf("booking status = %s, want checked_out", tx.booking.Status)
		}
		if len(tx.increments) != 1 {
			t.Fatalf("expected 1 increment call for remaining nights, got %d", len(tx.increments))
		}
		inc := tx.increments[0]
		if !inc.From.Equal(today) {
			t.Errorf("expected increment from %v, got %v", today, inc.From)
		}
		if !inc.To.Equal(tx.booking.CheckOut) {
			t.Errorf("expected increment to %v, got %v", tx.booking.CheckOut, inc.To)
		}
	})

	// 24. BE-G22: Rejection of no-show before check-in date (400 NO_SHOW_TOO_EARLY)
	t.Run("E2E-24: Rejection of no-show before check-in date (400 NO_SHOW_TOO_EARLY)", func(t *testing.T) {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		tx.booking.Status = booking.StatusConfirmed
		tx.booking.CheckIn = today.Add(24 * time.Hour) // arrival is tomorrow
		tx.booking.CheckOut = today.Add(72 * time.Hour)
		tx.increments = nil

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/no-show", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("no-show request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 Bad Request", res.StatusCode)
		}
		var pd api.ProblemDetails
		_ = json.NewDecoder(res.Body).Decode(&pd)
		if pd.Code != "NO_SHOW_TOO_EARLY" {
			t.Errorf("expected error code NO_SHOW_TOO_EARLY, got %s", pd.Code)
		}
		if tx.booking.Status != booking.StatusConfirmed {
			t.Errorf("expected status to remain confirmed, got %s", tx.booking.Status)
		}
		if len(tx.increments) != 0 {
			t.Errorf("expected 0 inventory increments on rejected no-show, got %d", len(tx.increments))
		}
	})

	// 25. BE-G22: Acceptance of no-show on check-in date releases remaining nights (200 OK)
	t.Run("E2E-25: Acceptance of no-show on check-in date releases remaining nights (200 OK)", func(t *testing.T) {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		tx.booking.Status = booking.StatusConfirmed
		tx.booking.CheckIn = today // arrival is today
		tx.booking.CheckOut = today.Add(48 * time.Hour)
		tx.increments = nil

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/no-show", nil)
		req.Header.Set("Authorization", "Bearer receptionist")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("no-show request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 OK", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusNoShow {
			t.Errorf("booking status = %s, want no_show", tx.booking.Status)
		}
		if len(tx.increments) != 1 {
			t.Fatalf("expected 1 increment call for remaining nights, got %d", len(tx.increments))
		}
		inc := tx.increments[0]
		if !inc.From.Equal(today) {
			t.Errorf("expected increment from %v, got %v", today, inc.From)
		}
		if !inc.To.Equal(tx.booking.CheckOut) {
			t.Errorf("expected increment to %v, got %v", tx.booking.CheckOut, inc.To)
		}
	})

	// 26. Xendit Webhook: Rejection of invalid callback token (401 Unauthorized)
	t.Run("E2E-26: Xendit Webhook rejects invalid callback token (401 Unauthorized)", func(t *testing.T) {
		payload := `{"id": "inv_test_wh_1", "external_id": "bk-e2e-001", "status": "PAID"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/webhooks/xendit", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-callback-token", "invalid_attacker_token")

		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("webhook request failed: %v", err)
		}
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 Unauthorized", res.StatusCode)
		}
	})

	// 27. Xendit Webhook: Valid callback token with PAID confirms booking (200 OK)
	t.Run("E2E-27: Xendit Webhook with PAID confirms booking idempotently (200 OK)", func(t *testing.T) {
		tx.booking.Status = booking.StatusPending
		futureExp := time.Now().UTC().Add(30 * time.Minute)
		tx.booking.ExpiresAt = &futureExp
		payload := `{"id": "inv_test_wh_2", "external_id": "bk-e2e-001", "status": "PAID", "amount": 1100000, "payment_method": "QRIS"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/webhooks/xendit", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-callback-token", "test_e2e_xendit_webhook_token")

		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("webhook request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 OK", res.StatusCode)
		}
		if tx.booking.Status != booking.StatusConfirmed {
			t.Errorf("booking status = %s, want confirmed", tx.booking.Status)
		}

		// Replay webhook (idempotent 200 OK)
		req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/webhooks/xendit", strings.NewReader(payload))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("x-callback-token", "test_e2e_xendit_webhook_token")

		res2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("replay webhook failed: %v", err)
		}
		if res2.StatusCode != http.StatusOK {
			t.Fatalf("replay status = %d, want 200 OK", res2.StatusCode)
		}
	})

	// 28. Resend Notifier: Email confirmation dispatch with Idempotency-Key
	t.Run("E2E-28: Resend Outbox email dispatch on confirmed booking", func(t *testing.T) {
		var receivedAuth string
		var receivedIdemp string
		var receivedTo []string
		var receivedSubject string

		resendMockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			receivedIdemp = r.Header.Get("Idempotency-Key")

			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if toList, ok := body["to"].([]any); ok {
				for _, to := range toList {
					receivedTo = append(receivedTo, to.(string))
				}
			}
			if subj, ok := body["subject"].(string); ok {
				receivedSubject = subj
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "re_msg_12345"})
		}))
		defer resendMockServer.Close()

		resendClient := notifier.NewResend(resendMockServer.URL, "re_e2e_secret_key", "Pulang ke Uttara <reservations@pulangkeuttara.com>", nil)

		b := tx.booking
		b.Status = booking.StatusConfirmed
		err := resendClient.SendBookingConfirmed(context.Background(), b)
		if err != nil {
			t.Fatalf("resend send failed: %v", err)
		}

		if receivedAuth != "Bearer re_e2e_secret_key" {
			t.Errorf("Authorization = %s, want Bearer re_e2e_secret_key", receivedAuth)
		}
		expectedIdemp := fmt.Sprintf("email-confirmed-%s", b.ID)
		if receivedIdemp != expectedIdemp {
			t.Errorf("Idempotency-Key = %s, want %s", receivedIdemp, expectedIdemp)
		}
		if len(receivedTo) != 1 || receivedTo[0] != b.GuestEmail {
			t.Errorf("to = %v, want [%s]", receivedTo, b.GuestEmail)
		}
		if !strings.Contains(receivedSubject, b.ID) {
			t.Errorf("subject %s does not contain booking ID %s", receivedSubject, b.ID)
		}
	})

	var guestSessionToken string

	// 29. Guest Request OTP Challenge
	t.Run("E2E-29: Guest requests OTP challenge (200 OK)", func(t *testing.T) {
		body := map[string]string{"email": "rian@example.com"}
		bBytes, _ := json.Marshal(body)
		res, err := client.Post(srv.URL+"/api/v1/auth/guest/challenge", "application/json", bytes.NewReader(bBytes))
		if err != nil {
			t.Fatalf("challenge request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("challenge status = %d, want 200", res.StatusCode)
		}
		var resp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&resp)
		if resp["status"] != "ok" {
			t.Errorf("status = %v, want ok", resp["status"])
		}
		if tx.otpNotifier.lastOTP == "" || len(tx.otpNotifier.lastOTP) != 6 {
			t.Errorf("expected 6-digit OTP captured, got %s", tx.otpNotifier.lastOTP)
		}
	})

	// 30. Guest Verifies OTP and receives Session Token
	t.Run("E2E-30: Guest verifies OTP and receives session token (200 OK)", func(t *testing.T) {
		body := map[string]string{
			"email": "rian@example.com",
			"code":  tx.otpNotifier.lastOTP,
		}
		bBytes, _ := json.Marshal(body)
		res, err := client.Post(srv.URL+"/api/v1/auth/guest/verify", "application/json", bytes.NewReader(bBytes))
		if err != nil {
			t.Fatalf("verify request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("verify status = %d, want 200", res.StatusCode)
		}
		var resp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&resp)
		token, ok := resp["token"].(string)
		if !ok || !strings.HasPrefix(token, "gst_sess_") {
			t.Fatalf("expected token starting with gst_sess_, got %v", resp["token"])
		}
		guestSessionToken = token

		cookies := res.Cookies()
		foundCookie := false
		for _, c := range cookies {
			if c.Name == "guest_session" && c.Value == token {
				foundCookie = true
				break
			}
		}
		if !foundCookie {
			t.Errorf("expected guest_session cookie in response")
		}
	})

	// 31. Guest Accesses Profile via Session
	t.Run("E2E-31: Guest inspects profile via /auth/guest/me (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/guest/me", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("me request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("me status = %d, want 200", res.StatusCode)
		}
		var resp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&resp)
		if resp["email"] != "rian@example.com" {
			t.Errorf("email = %v, want rian@example.com", resp["email"])
		}
	})

	// 32. Guest Accesses My Bookings List
	t.Run("E2E-32: Guest lists own reservations via /guest/bookings (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings?status=all", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("list bookings request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list bookings status = %d, want 200", res.StatusCode)
		}
		var resp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&resp)
		data, ok := resp["data"].([]any)
		if !ok || len(data) == 0 {
			t.Fatalf("expected bookings in data, got %v", resp)
		}
		first := data[0].(map[string]any)
		if first["id"] != "bk-e2e-001" {
			t.Errorf("booking id = %v, want bk-e2e-001", first["id"])
		}
	})

	// 33. Guest Accesses Booking Detail with Allowed Actions
	t.Run("E2E-33: Guest views booking detail with allowed actions (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("detail request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("detail status = %d, want 200", res.StatusCode)
		}
		var resp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&resp)
		bk, ok := resp["booking"].(map[string]any)
		if !ok || bk["id"] != "bk-e2e-001" {
			t.Errorf("booking id = %v, want bk-e2e-001", bk["id"])
		}
		if bk["guest_email"] != "rian@example.com" {
			t.Errorf("guest_email = %v, want rian@example.com", bk["guest_email"])
		}
		actions, ok := resp["allowed_actions"].(map[string]any)
		if !ok {
			t.Fatalf("expected allowed_actions map in response")
		}
		if actions["can_download_receipt"] != true {
			t.Errorf("expected can_download_receipt true, got %v", actions["can_download_receipt"])
		}
	})

	// 34. IDOR Defense: Accessing Another Guest's Booking returns 404 Not Found
	t.Run("E2E-34: IDOR defense returns 404 for another guest's booking", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-other-user", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("IDOR status = %d, want 404 Not Found", res.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body["error"] != "BOOKING_NOT_FOUND" {
			t.Errorf("error = %v, want BOOKING_NOT_FOUND", body["error"])
		}
	})

	// 35. Guest Downloads Printable Invoice / Receipt DTO (F04)
	t.Run("E2E-35: Guest downloads Printable Invoice Receipt DTO (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/receipt", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("receipt request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("receipt status = %d, want 200 OK", res.StatusCode)
		}
		if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("expected Cache-Control to contain no-store, got %s", cc)
		}

		var receipt map[string]any
		_ = json.NewDecoder(res.Body).Decode(&receipt)

		invNum, _ := receipt["invoice_number"].(string)
		if !strings.HasPrefix(invNum, "INV/PKU/") {
			t.Errorf("invoice_number = %v, want prefix INV/PKU/", invNum)
		}
		if receipt["status"] != "confirmed" {
			t.Errorf("status = %v, want confirmed", receipt["status"])
		}

		hotelInfo, ok := receipt["hotel_info"].(map[string]any)
		if !ok || hotelInfo["name"] != "Pulang ke Uttara" {
			t.Errorf("hotel_info name = %v, want Pulang ke Uttara", hotelInfo["name"])
		}

		stayDetails, ok := receipt["stay_details"].(map[string]any)
		if !ok || stayDetails["check_in_time"] != "14:00 WIB" {
			t.Errorf("check_in_time = %v, want 14:00 WIB", stayDetails["check_in_time"])
		}

		pricing, ok := receipt["pricing_breakdown"].(map[string]any)
		if !ok || pricing["total_price_minor"] == nil {
			t.Errorf("expected pricing_breakdown with total_price_minor")
		}

		qrPayload, _ := receipt["qr_payload"].(string)
		if !strings.Contains(qrPayload, "bk-e2e-001") {
			t.Errorf("qr_payload = %v, want to contain bk-e2e-001", qrPayload)
		}
	})

	// 36. Guest Downloads RFC 5545 iCalendar stream (F04)
	t.Run("E2E-36: Guest downloads RFC 5545 iCalendar stream (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/calendar.ics", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("calendar request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("calendar status = %d, want 200 OK", res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/calendar") {
			t.Errorf("Content-Type = %v, want text/calendar", ct)
		}
		if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "pulang-booking-bk-e2e-001.ics") {
			t.Errorf("Content-Disposition = %v, want attachment with filename", cd)
		}

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(res.Body)
		icsContent := buf.String()

		expectedTokens := []string{
			"BEGIN:VCALENDAR",
			"VERSION:2.0",
			"PRODID:-//Pulang ke Uttara",
			"TZID:Asia/Jakarta",
			"BEGIN:VEVENT",
			"UID:booking-bk-e2e-001@pulangkeuttara.id",
			"SUMMARY:Menginap di Pulang ke Uttara",
			"STATUS:CONFIRMED",
			"BEGIN:VALARM",
			"TRIGGER:-P1D",
			"END:VALARM",
			"END:VEVENT",
			"END:VCALENDAR",
		}
		for _, tok := range expectedTokens {
			if !strings.Contains(icsContent, tok) {
				t.Errorf("iCalendar output missing token: %q", tok)
			}
		}
	})

	// 37. Receipt status guard: Non-confirmed booking rejected with 400 Bad Request
	t.Run("E2E-37: Receipt and calendar rejected for non-confirmed booking (400 RECEIPT_NOT_AVAILABLE)", func(t *testing.T) {
		originalStatus := tx.booking.Status
		tx.booking.Status = booking.StatusPending
		defer func() { tx.booking.Status = originalStatus }()

		// 1. Receipt attempt on pending booking
		reqR, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/receipt", nil)
		reqR.Header.Set("Authorization", "Bearer "+guestSessionToken)
		resR, err := client.Do(reqR)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resR.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 Bad Request", resR.StatusCode)
		}
		var errBody map[string]any
		_ = json.NewDecoder(resR.Body).Decode(&errBody)
		if errBody["error"] != "RECEIPT_NOT_AVAILABLE" {
			t.Errorf("error = %v, want RECEIPT_NOT_AVAILABLE", errBody["error"])
		}

		// 2. Calendar attempt on pending booking
		reqC, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/calendar.ics", nil)
		reqC.Header.Set("Authorization", "Bearer "+guestSessionToken)
		resC, err := client.Do(reqC)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resC.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 Bad Request", resC.StatusCode)
		}
	})

	// 38. IDOR Defense: Receipt and calendar rejected for other guest's booking
	t.Run("E2E-38: IDOR defense returns 404 for receipt and calendar of another guest", func(t *testing.T) {
		// 1. Receipt on foreign booking
		reqR, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-other-user/receipt", nil)
		reqR.Header.Set("Authorization", "Bearer "+guestSessionToken)
		resR, err := client.Do(reqR)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resR.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404 Not Found", resR.StatusCode)
		}

		// 2. Calendar on foreign booking
		reqC, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-other-user/calendar.ics", nil)
		reqC.Header.Set("Authorization", "Bearer "+guestSessionToken)
		resC, err := client.Do(reqC)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resC.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404 Not Found", resC.StatusCode)
		}
	})

	// 39. Guest Logout and Revocation
	t.Run("E2E-39: Guest logout revokes session (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/guest/logout", nil)
		req.Header.Set("Authorization", "Bearer "+guestSessionToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("logout request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("logout status = %d, want 200", res.StatusCode)
		}

		// Permintaan berikutnya ke /me wajib gagal 401 Unauthorized
		reqMe, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/guest/me", nil)
		reqMe.Header.Set("Authorization", "Bearer "+guestSessionToken)
		resMe, err := client.Do(reqMe)
		if err != nil {
			t.Fatalf("me request failed: %v", err)
		}
		if resMe.StatusCode != http.StatusUnauthorized {
			t.Errorf("me status after logout = %d, want 401 Unauthorized", resMe.StatusCode)
		}

		// Permintaan ke receipt tanpa sesi wajib gagal 401 Unauthorized
		reqRc, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/receipt", nil)
		resRc, _ := client.Do(reqRc)
		if resRc.StatusCode != http.StatusUnauthorized {
			t.Errorf("receipt without session = %d, want 401 Unauthorized", resRc.StatusCode)
		}
	})

	// 40. Resend Notifier: Guest OTP email dispatch
	t.Run("E2E-40: Resend dispatch guest OTP email", func(t *testing.T) {
		var receivedSubject string
		var receivedTo []string
		var receivedIdemp string

		resendMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedIdemp = r.Header.Get("Idempotency-Key")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if toList, ok := body["to"].([]any); ok {
				for _, to := range toList {
					receivedTo = append(receivedTo, to.(string))
				}
			}
			if s, ok := body["subject"].(string); ok {
				receivedSubject = s
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "re_otp_9999"})
		}))
		defer resendMock.Close()

		resendClient := notifier.NewResend(resendMock.URL, "re_e2e_otp_key", "Pulang ke Uttara <reservations@pulangkeuttara.com>", nil)
		err := resendClient.SendGuestOTP(context.Background(), "rian@example.com", "987123")
		if err != nil {
			t.Fatalf("SendGuestOTP failed: %v", err)
		}
		if len(receivedTo) != 1 || receivedTo[0] != "rian@example.com" {
			t.Errorf("to = %v, want [rian@example.com]", receivedTo)
		}
		if !strings.Contains(receivedSubject, "Kode Verifikasi") {
			t.Errorf("subject = %s, want to contain 'Kode Verifikasi'", receivedSubject)
		}
		if !strings.HasPrefix(receivedIdemp, "otp-rian@example.com") {
			t.Errorf("Idempotency-Key = %s, want prefix 'otp-rian@example.com'", receivedIdemp)
		}
	})

	// 41. Finance Refund: Valid refund by finance officer (201 Created) & RBAC rejection for receptionist (403 Forbidden)
	t.Run("E2E-41: Finance refund processing & RBAC authorization", func(t *testing.T) {
		// 1. Receptionist mencoba inisiasi refund -> 403 Forbidden
		refundBody := `{"booking_id":"bk-e2e-001","amount_minor":500000,"reason":"Guest requested partial cancellation"}`
		reqRec, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/finance/refunds", strings.NewReader(refundBody))
		reqRec.Header.Set("Authorization", "Bearer receptionist")
		reqRec.Header.Set("Content-Type", "application/json")
		resRec, err := client.Do(reqRec)
		if err != nil {
			t.Fatalf("receptionist refund request failed: %v", err)
		}
		if resRec.StatusCode != http.StatusForbidden {
			t.Errorf("receptionist refund status = %d, want 403 Forbidden", resRec.StatusCode)
		}

		// 2. Finance officer memproses refund -> 201 Created
		reqFin, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/finance/refunds", strings.NewReader(refundBody))
		reqFin.Header.Set("Authorization", "Bearer finance")
		reqFin.Header.Set("Content-Type", "application/json")
		resFin, err := client.Do(reqFin)
		if err != nil {
			t.Fatalf("finance refund request failed: %v", err)
		}
		if resFin.StatusCode != http.StatusCreated {
			t.Fatalf("finance refund status = %d, want 201 Created", resFin.StatusCode)
		}

		var resp map[string]any
		_ = json.NewDecoder(resFin.Body).Decode(&resp)
		refundData, ok := resp["refund"].(map[string]any)
		if !ok {
			t.Fatalf("expected refund object in response, got %v", resp)
		}
		if refundData["status"] != "succeeded" {
			t.Errorf("refund status = %v, want succeeded", refundData["status"])
		}
		if refundData["amount_minor"].(float64) != 500000 {
			t.Errorf("refund amount = %v, want 500000", refundData["amount_minor"])
		}
	})

	// 42. Finance Anti-Over-Refund Guard (409 Conflict)
	t.Run("E2E-42: Anti-over-refund guard strictly rejects excessive amount (409 Conflict)", func(t *testing.T) {
		// Total booking 1.100.000, sudah di-refund 500.000 di E2E-41. Sisa saldo: 600.000.
		// Permintaan refund 700.000 wajib ditolak dengan 409 Conflict.
		overBody := `{"booking_id":"bk-e2e-001","amount_minor":700000,"reason":"Excessive refund attempt"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/finance/refunds", strings.NewReader(overBody))
		req.Header.Set("Authorization", "Bearer finance")
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("over-refund status = %d, want 409 Conflict", res.StatusCode)
		}

		var errResp map[string]any
		_ = json.NewDecoder(res.Body).Decode(&errResp)
		if errResp["code"] != "OVER_REFUND_EXCEEDED" {
			t.Errorf("error code = %v, want OVER_REFUND_EXCEEDED", errResp["code"])
		}
	})

	// 43. Late Payment Case & Resolution Workflow
	t.Run("E2E-43: Late payment case listing and resolution workflow (200 OK)", func(t *testing.T) {
		// Simulasikan payment case baru
		_ = tx.finStore.CreatePaymentCase(context.Background(), &finance.PaymentCase{
			BookingID:         "bk-e2e-001",
			CaseType:          "late_payment",
			Status:            "open",
			AmountMinor:       1100000,
			Currency:          "IDR",
			ProviderReference: "inv_late_e2e_999",
			Notes:             "Payment arrived after hold expired",
			CreatedAt:         time.Now(),
		})

		// 1. Finance officer melihat daftar open cases
		reqList, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/finance/cases?status=open", nil)
		reqList.Header.Set("Authorization", "Bearer finance")
		resList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("list cases request failed: %v", err)
		}
		if resList.StatusCode != http.StatusOK {
			t.Fatalf("list cases status = %d, want 200 OK", resList.StatusCode)
		}

		// 2. Resolve payment case
		resolveBody := `{"action":"refund","notes":"Manual refund dispatched to guest bank account"}`
		reqRes, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/finance/cases/case-e2e-001/resolve", strings.NewReader(resolveBody))
		reqRes.Header.Set("Authorization", "Bearer finance")
		reqRes.Header.Set("Content-Type", "application/json")
		resRes, err := client.Do(reqRes)
		if err != nil {
			t.Fatalf("resolve case request failed: %v", err)
		}
		if resRes.StatusCode != http.StatusOK {
			t.Fatalf("resolve case status = %d, want 200 OK", resRes.StatusCode)
		}

		// 3. Resolve ulang kasus yang sama wajib ditolak 409 Conflict
		reqDup, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/finance/cases/case-e2e-001/resolve", strings.NewReader(resolveBody))
		reqDup.Header.Set("Authorization", "Bearer finance")
		reqDup.Header.Set("Content-Type", "application/json")
		resDup, _ := client.Do(reqDup)
		if resDup.StatusCode != http.StatusConflict {
			t.Errorf("duplicate resolve status = %d, want 409 Conflict", resDup.StatusCode)
		}
	})

	// 44. Finance Reconciliation Summary
	t.Run("E2E-44: Finance reconciliation summary aggregation (200 OK)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/finance/reconciliations", nil)
		req.Header.Set("Authorization", "Bearer finance")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("summary request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("summary status = %d, want 200 OK", res.StatusCode)
		}

		var summary map[string]any
		_ = json.NewDecoder(res.Body).Decode(&summary)
		if summary["total_settled_minor"].(float64) <= 0 {
			t.Errorf("total_settled_minor = %v, want > 0", summary["total_settled_minor"])
		}
		if summary["net_captured_minor"].(float64) <= 0 {
			t.Errorf("net_captured_minor = %v, want > 0", summary["net_captured_minor"])
		}
	})

	// 45. Guest Refund Status Self-Service & Anti-IDOR Protection
	t.Run("E2E-45: Guest refund status inquiry & anti-IDOR verification", func(t *testing.T) {
		// Buat sesi tamu rian@example.com untuk pengujian (karena tx.booking.GuestEmail = "rian@example.com")
		sessRian := &guest.GuestSession{
			ID:           "sess-rian-refund",
			GuestEmail:   "rian@example.com",
			TokenHash:    guest.HashString("gst_sess_rian_refund_token"),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
			LastActiveAt: time.Now(),
			CreatedAt:    time.Now(),
		}
		_ = tx.guestStore.CreateSession(context.Background(), sessRian)

		// 1. Tamu Rian melihat status refund booking miliknya -> 200 OK
		reqRian, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-e2e-001/refund-status", nil)
		reqRian.Header.Set("Authorization", "Bearer gst_sess_rian_refund_token")
		resRian, err := client.Do(reqRian)
		if err != nil {
			t.Fatalf("guest refund status request failed: %v", err)
		}
		if resRian.StatusCode != http.StatusOK {
			t.Fatalf("guest refund status = %d, want 200 OK", resRian.StatusCode)
		}

		var view map[string]any
		_ = json.NewDecoder(resRian.Body).Decode(&view)
		if view["has_refund"] != true {
			t.Errorf("has_refund = %v, want true", view["has_refund"])
		}

		// 2. Anti-IDOR: Tamu mencoba mengakses booking orang lain -> 404 Not Found
		reqIDOR, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/bk-foreign-001/refund-status", nil)
		reqIDOR.Header.Set("Authorization", "Bearer gst_sess_rian_refund_token")
		resIDOR, err := client.Do(reqIDOR)
		if err != nil {
			t.Fatalf("idor request failed: %v", err)
		}
		if resIDOR.StatusCode != http.StatusNotFound {
			t.Errorf("idor status = %d, want 404 Not Found", resIDOR.StatusCode)
		}
	})
}




