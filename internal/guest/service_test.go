package guest

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/rates"
)

type mockStore struct {
	mu         sync.Mutex
	challenges map[string]*Challenge
	sessions   map[string]*GuestSession
	bookings   []BookingDetail
	receipts   map[string]*ReceiptDTO
	createErr  error
	updateErr  error
	sessionErr error
	touchErr   error
	listErr    error
	detailErr  error
	receiptErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		challenges: make(map[string]*Challenge),
		sessions:   make(map[string]*GuestSession),
		receipts:   make(map[string]*ReceiptDTO),
	}
}

func (m *mockStore) CreateChallenge(ctx context.Context, c *Challenge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	c.ID = "ch_001"
	m.challenges[c.Email] = c
	return nil
}

func (m *mockStore) CreateChallengeWithCooldown(ctx context.Context, c *Challenge, cooldown time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	for _, ch := range m.challenges {
		if ch.Email == c.Email {
			diff := c.CreatedAt.Sub(ch.CreatedAt)
			if diff < cooldown {
				return ErrRateLimited
			}
		}
	}
	c.ID = "ch_001"
	m.challenges[c.Email] = c
	return nil
}

func (m *mockStore) GetLatestActiveChallenge(ctx context.Context, email string) (*Challenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.challenges[email]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (m *mockStore) UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	for _, c := range m.challenges {
		if c.ID == id {
			c.Attempts = attempts
		}
	}
	return nil
}

func (m *mockStore) MarkChallengeVerified(ctx context.Context, id string, verifiedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.challenges {
		if c.ID == id {
			c.VerifiedAt = &verifiedAt
		}
	}
	return nil
}

func (m *mockStore) VerifyAndConsumeChallenge(ctx context.Context, email, inputHash string, now time.Time, newSession *GuestSession) (*GuestSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.challenges[email]
	if !ok || ch == nil {
		return nil, ErrInvalidOrExpiredCode
	}
	if ch.VerifiedAt != nil {
		return nil, ErrInvalidOrExpiredCode
	}
	if now.After(ch.ExpiresAt) {
		return nil, ErrInvalidOrExpiredCode
	}
	if ch.Attempts >= ch.MaxAttempts {
		return nil, ErrMaxAttemptsExceeded
	}

	if subtle.ConstantTimeCompare([]byte(ch.CodeHash), []byte(inputHash)) != 1 {
		ch.Attempts++
		if ch.Attempts >= ch.MaxAttempts {
			return nil, ErrMaxAttemptsExceeded
		}
		return nil, ErrInvalidOrExpiredCode
	}

	ch.VerifiedAt = &now
	newSession.ID = "sess_001"
	m.sessions[newSession.TokenHash] = newSession
	return newSession, nil
}

func (m *mockStore) CreateSession(ctx context.Context, s *GuestSession) error {
	if m.sessionErr != nil {
		return m.sessionErr
	}
	s.ID = "sess_001"
	m.sessions[s.TokenHash] = s
	return nil
}

func (m *mockStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*GuestSession, error) {
	s, ok := m.sessions[tokenHash]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockStore) TouchSession(ctx context.Context, id string, lastActiveAt, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.touchErr != nil {
		return m.touchErr
	}
	for _, s := range m.sessions {
		if s.ID == id {
			s.LastActiveAt = lastActiveAt
			s.ExpiresAt = expiresAt
		}
	}
	return nil
}

func (m *mockStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	delete(m.sessions, tokenHash)
	return nil
}

func (m *mockStore) CountActiveBookingsByEmail(ctx context.Context, email string) (int, error) {
	count := 0
	for _, b := range m.bookings {
		if strings.EqualFold(strings.TrimSpace(b.GuestEmail), strings.TrimSpace(email)) && (b.Status == "pending" || b.Status == "confirmed") {
			count++
		}
	}
	return count, nil
}

func (m *mockStore) ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]BookingSummary, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []BookingSummary
	for _, b := range m.bookings {
		if strings.EqualFold(strings.TrimSpace(b.GuestEmail), strings.TrimSpace(email)) {
			if status == "upcoming" && b.Status != "confirmed" && b.Status != "pending" {
				continue
			}
			if status == "completed" && b.Status != "checked_out" {
				continue
			}
			if status == "cancelled" && b.Status != "cancelled" {
				continue
			}
			res = append(res, BookingSummary{
				ID:              b.ID,
				RoomTypeID:      b.RoomTypeID,
				RoomTypeName:    b.RoomTypeName,
				CheckIn:         b.CheckIn,
				CheckOut:        b.CheckOut,
				NumRooms:        b.NumRooms,
				NumGuests:       b.NumGuests,
				Status:          b.Status,
				TotalPriceMinor: b.TotalPriceMinor,
				Currency:        b.Currency,
				CreatedAt:       b.CreatedAt,
			})
		}
	}
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (m *mockStore) GetBookingDetailByEmail(ctx context.Context, email, bookingID string) (*BookingDetail, error) {
	if m.detailErr != nil {
		return nil, m.detailErr
	}
	for _, b := range m.bookings {
		if b.ID == bookingID && strings.EqualFold(strings.TrimSpace(b.GuestEmail), strings.TrimSpace(email)) {
			bCopy := b
			return &bCopy, nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetBookingReceiptData(ctx context.Context, email, bookingID string) (*ReceiptDTO, error) {
	if m.receiptErr != nil {
		return nil, m.receiptErr
	}
	for key, r := range m.receipts {
		if (r.BookingID == bookingID || key == bookingID) && strings.EqualFold(strings.TrimSpace(r.GuestDetails.Email), strings.TrimSpace(email)) {
			rCopy := *r
			return &rCopy, nil
		}
	}
	return nil, nil
}


type mockNotifier struct {
	sentEmail string
	sentCode  string
	err       error
}

func (n *mockNotifier) SendGuestOTP(ctx context.Context, email, otpCode string) error {
	n.sentEmail = email
	n.sentCode = otpCode
	return n.err
}

func TestGuestService_RequestChallenge_TableTest(t *testing.T) {
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		email        string
		setupStore   func(s *mockStore)
		expectErr    error
		expectCdSec  int
	}{
		{
			name:        "Valid email creates OTP challenge and returns 60s cooldown",
			email:       "tamu@example.com",
			setupStore:  func(s *mockStore) {},
			expectErr:   nil,
			expectCdSec: 60,
		},
		{
			name:        "Invalid email format returns ErrInvalidEmail",
			email:       "invalid-email",
			setupStore:  func(s *mockStore) {},
			expectErr:   ErrInvalidEmail,
			expectCdSec: 0,
		},
		{
			name:  "Requesting challenge within 60s cooldown returns ErrRateLimited",
			email: "tamu@example.com",
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:        "ch_active",
					Email:     "tamu@example.com",
					CreatedAt: now.Add(-30 * time.Second),
				}
			},
			expectErr:   ErrRateLimited,
			expectCdSec: 0,
		},
		{
			name:  "Requesting challenge after 60s cooldown succeeds",
			email: "tamu@example.com",
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:        "ch_old",
					Email:     "tamu@example.com",
					CreatedAt: now.Add(-65 * time.Second),
				}
			},
			expectErr:   nil,
			expectCdSec: 60,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			tc.setupStore(store)
			notifier := &mockNotifier{}
			svc := NewService(store, notifier, slog.Default())
			svc.nowFunc = func() time.Time { return now }

			cd, err := svc.RequestChallenge(context.Background(), tc.email)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cd != tc.expectCdSec {
				t.Errorf("expected cooldown %d, got %d", tc.expectCdSec, cd)
			}
		})
	}
}

func TestGuestService_VerifyChallenge_TableTest(t *testing.T) {
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	validCode := "654321"
	validHash := HashString(validCode)

	tests := []struct {
		name       string
		email      string
		code       string
		setupStore func(s *mockStore)
		expectErr  error
	}{
		{
			name:  "Valid code verifies successfully and returns session token",
			email: "tamu@example.com",
			code:  validCode,
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:          "ch_valid",
					Email:       "tamu@example.com",
					CodeHash:    validHash,
					MaxAttempts: 3,
					Attempts:    0,
					ExpiresAt:   now.Add(10 * time.Minute),
				}
			},
			expectErr: nil,
		},
		{
			name:       "Code with length not 6 returns ErrInvalidOrExpiredCode",
			email:      "tamu@example.com",
			code:       "123",
			setupStore: func(s *mockStore) {},
			expectErr:  ErrInvalidOrExpiredCode,
		},
		{
			name:       "No active challenge found returns ErrInvalidOrExpiredCode",
			email:      "unknown@example.com",
			code:       validCode,
			setupStore: func(s *mockStore) {},
			expectErr:  ErrInvalidOrExpiredCode,
		},
		{
			name:  "Expired challenge returns ErrInvalidOrExpiredCode",
			email: "tamu@example.com",
			code:  validCode,
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:          "ch_expired",
					Email:       "tamu@example.com",
					CodeHash:    validHash,
					ExpiresAt:   now.Add(-1 * time.Minute),
					MaxAttempts: 3,
				}
			},
			expectErr: ErrInvalidOrExpiredCode,
		},
		{
			name:  "Already verified challenge cannot be replayed",
			email: "tamu@example.com",
			code:  validCode,
			setupStore: func(s *mockStore) {
				verified := now.Add(-2 * time.Minute)
				s.challenges["tamu@example.com"] = &Challenge{
					ID:          "ch_used",
					Email:       "tamu@example.com",
					CodeHash:    validHash,
					VerifiedAt:  &verified,
					ExpiresAt:   now.Add(5 * time.Minute),
					MaxAttempts: 3,
				}
			},
			expectErr: ErrInvalidOrExpiredCode,
		},
		{
			name:  "Wrong code increments attempts and returns ErrInvalidOrExpiredCode",
			email: "tamu@example.com",
			code:  "999999",
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:          "ch_active",
					Email:       "tamu@example.com",
					CodeHash:    validHash,
					Attempts:    1,
					MaxAttempts: 3,
					ExpiresAt:   now.Add(5 * time.Minute),
				}
			},
			expectErr: ErrInvalidOrExpiredCode,
		},
		{
			name:  "Max attempts reached locks challenge and returns ErrMaxAttemptsExceeded",
			email: "tamu@example.com",
			code:  "999999",
			setupStore: func(s *mockStore) {
				s.challenges["tamu@example.com"] = &Challenge{
					ID:          "ch_locked",
					Email:       "tamu@example.com",
					CodeHash:    validHash,
					Attempts:    2,
					MaxAttempts: 3,
					ExpiresAt:   now.Add(5 * time.Minute),
				}
			},
			expectErr: ErrMaxAttemptsExceeded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			tc.setupStore(store)
			svc := NewService(store, nil, slog.Default())
			svc.nowFunc = func() time.Time { return now }

			token, sess, err := svc.VerifyChallenge(context.Background(), tc.email, tc.code)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token == "" || sess == nil {
				t.Fatalf("expected non-empty token and session")
			}
			if sess.GuestEmail != tc.email {
				t.Errorf("expected email %s, got %s", tc.email, sess.GuestEmail)
			}
		})
	}
}

func TestGuestService_ValidateAndRevokeSession_TableTest(t *testing.T) {
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	rawToken := "gst_sess_test_token_12345"
	tokenHash := HashString(rawToken)

	store := newMockStore()
	store.sessions[tokenHash] = &GuestSession{
		ID:           "sess_active",
		GuestEmail:   "tamu@example.com",
		TokenHash:    tokenHash,
		ExpiresAt:    now.Add(12 * time.Hour),
		LastActiveAt: now.Add(-1 * time.Hour),
		CreatedAt:    now.Add(-2 * time.Hour),
	}

	svc := NewService(store, nil, slog.Default())
	svc.nowFunc = func() time.Time { return now }

	// 1. Valid token
	sess, err := svc.ValidateSession(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("expected valid session, got err: %v", err)
	}
	if sess.GuestEmail != "tamu@example.com" {
		t.Errorf("expected email tamu@example.com, got %s", sess.GuestEmail)
	}

	// 2. Invalid / unknown token
	_, err = svc.ValidateSession(context.Background(), "gst_sess_unknown")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}

	// 3. Expired token
	store.sessions[tokenHash].ExpiresAt = now.Add(-1 * time.Minute)
	_, err = svc.ValidateSession(context.Background(), rawToken)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("expected ErrSessionExpired, got %v", err)
	}

	// 4. Revoke (logout)
	store.sessions[tokenHash].ExpiresAt = now.Add(1 * time.Hour)
	err = svc.RevokeSession(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("unexpected revoke err: %v", err)
	}
	_, err = svc.ValidateSession(context.Background(), rawToken)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound after revoke, got %v", err)
	}

	// 5. TouchSession DB failure does not extend in-memory expiry (BE-R04)
	token2 := "gst_sess_touch_fail"
	tokenHash2 := HashString(token2)
	initialExpiry := now.Add(2 * time.Hour)
	store.sessions[tokenHash2] = &GuestSession{
		ID:           "sess_touch_fail",
		GuestEmail:   "tamu2@example.com",
		TokenHash:    tokenHash2,
		ExpiresAt:    initialExpiry,
		LastActiveAt: now.Add(-1 * time.Hour),
		CreatedAt:    now.Add(-2 * time.Hour),
	}
	store.touchErr = errors.New("db disconnect")
	sess2, err := svc.ValidateSession(context.Background(), token2)
	if err != nil {
		t.Fatalf("expected non-nil session despite touch error, got %v", err)
	}
	if !sess2.ExpiresAt.Equal(initialExpiry) {
		t.Errorf("expected expiry to remain unchanged at %v, got %v", initialExpiry, sess2.ExpiresAt)
	}
	store.touchErr = nil
}

func TestGuestService_BookingsAndIDOR_TableTest(t *testing.T) {
	store := newMockStore()
	store.bookings = []BookingDetail{
		{
			ID:              "bk_001",
			GuestEmail:      "tamu1@example.com",
			RoomTypeName:    "Superior King",
			Status:          "confirmed",
			TotalPriceMinor: 550000,
			Currency:        "IDR",
			CheckIn:         "2026-10-10",
			CheckOut:        "2026-10-12",
		},
		{
			ID:              "bk_002",
			GuestEmail:      "tamu1@example.com",
			RoomTypeName:    "Deluxe King",
			Status:          "checked_out",
			TotalPriceMinor: 750000,
			Currency:        "IDR",
			CheckIn:         "2026-09-01",
			CheckOut:        "2026-09-03",
		},
		{
			ID:              "bk_003",
			GuestEmail:      "tamu2@example.com",
			RoomTypeName:    "Executive King",
			Status:          "confirmed",
			TotalPriceMinor: 1100000,
			Currency:        "IDR",
			CheckIn:         "2026-10-15",
			CheckOut:        "2026-10-17",
		},
	}

	svc := NewService(store, nil, slog.Default())

	// 1. List bookings for tamu1
	list, err := svc.ListBookings(context.Background(), "tamu1@example.com", "all", 10)
	if err != nil {
		t.Fatalf("unexpected error on list bookings: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 bookings for tamu1, got %d", len(list))
	}

	// 2. Tamu1 retrieves own booking detail (bk_001)
	detail, err := svc.GetBookingDetail(context.Background(), "tamu1@example.com", "bk_001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID != "bk_001" {
		t.Errorf("expected booking ID bk_001, got %s", detail.ID)
	}
	if !detail.AllowedActions.CanCancel || !detail.AllowedActions.CanDownloadReceipt {
		t.Errorf("expected CanCancel and CanDownloadReceipt to be true for confirmed booking")
	}

	// 3. IDOR Defense: Tamu1 attempts to access Tamu2's booking (bk_003) -> Returns ErrBookingNotFound (404 Not Found)
	_, err = svc.GetBookingDetail(context.Background(), "tamu1@example.com", "bk_003")
	if !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("IDOR security breach: expected ErrBookingNotFound, got %v", err)
	}
}

func TestGuestService_GetBookingReceipt_TableTest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		email      string
		bookingID  string
		setupStore func(s *mockStore)
		expectErr  error
		verifyRes  func(t *testing.T, res *ReceiptDTO)
	}{
		{
			name:      "Confirmed booking returns complete receipt DTO",
			email:     "rian@example.com",
			bookingID: "bk_conf_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_conf_01"] = &ReceiptDTO{
					InvoiceNumber:    "INV/PKU/202610/BKCONF01",
					BookingID:        "bk_conf_01",
					BookingReference: "PKU-20261003-BKCONF01",
					Status:           "confirmed",
					HotelInfo: HotelInfo{
						Name:    "Pulang ke Uttara",
						Address: "Jl. Kaliurang Km 5.6",
					},
					StayDetails: StayDetails{
						CheckInDate:  "2026-10-10",
						CheckOutDate: "2026-10-12",
						TotalNights:  2,
					},
					GuestDetails: GuestDetails{
						Name:  "Rian Ardianto",
						Email: "rian@example.com",
					},
					RoomItem: RoomItemReceipt{
						RoomTypeName: "Superior Room",
						NumRooms:     1,
					},
					PricingBreakdown: PricingBreakdown{
						TotalPriceMinor: 158950000,
					},
				}
			},
			expectErr: nil,
			verifyRes: func(t *testing.T, res *ReceiptDTO) {
				if res == nil {
					t.Fatalf("expected non-nil receipt")
				}
				if res.InvoiceNumber != "INV/PKU/202610/BKCONF01" {
					t.Errorf("expected invoice number INV/PKU/202610/BKCONF01, got %s", res.InvoiceNumber)
				}
				if res.Status != "confirmed" {
					t.Errorf("expected status confirmed, got %s", res.Status)
				}
			},
		},
		{
			name:      "Checked-in booking returns valid receipt",
			email:     "rian@example.com",
			bookingID: "bk_in_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_in_01"] = &ReceiptDTO{
					BookingID: "bk_in_01",
					Status:    "checked_in",
					GuestDetails: GuestDetails{
						Email: "rian@example.com",
					},
				}
			},
			expectErr: nil,
			verifyRes: func(t *testing.T, res *ReceiptDTO) {
				if res == nil || res.Status != "checked_in" {
					t.Fatalf("expected valid checked_in receipt")
				}
			},
		},
		{
			name:      "Checked-out booking returns valid receipt",
			email:     "rian@example.com",
			bookingID: "bk_out_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_out_01"] = &ReceiptDTO{
					BookingID: "bk_out_01",
					Status:    "checked_out",
					GuestDetails: GuestDetails{
						Email: "rian@example.com",
					},
				}
			},
			expectErr: nil,
			verifyRes: func(t *testing.T, res *ReceiptDTO) {
				if res == nil || res.Status != "checked_out" {
					t.Fatalf("expected valid checked_out receipt")
				}
			},
		},
		{
			name:      "Pending booking returns ErrReceiptNotAvailable",
			email:     "rian@example.com",
			bookingID: "bk_pending_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_pending_01"] = &ReceiptDTO{
					BookingID: "bk_pending_01",
					Status:    "pending",
					GuestDetails: GuestDetails{
						Email: "rian@example.com",
					},
				}
			},
			expectErr: ErrReceiptNotAvailable,
			verifyRes: nil,
		},
		{
			name:      "Cancelled booking returns ErrReceiptNotAvailable",
			email:     "rian@example.com",
			bookingID: "bk_cancel_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_cancel_01"] = &ReceiptDTO{
					BookingID: "bk_cancel_01",
					Status:    "cancelled",
					GuestDetails: GuestDetails{
						Email: "rian@example.com",
					},
				}
			},
			expectErr: ErrReceiptNotAvailable,
			verifyRes: nil,
		},
		{
			name:      "IDOR attempt accessing other guest booking returns ErrBookingNotFound",
			email:     "hacker@example.com",
			bookingID: "bk_conf_01",
			setupStore: func(s *mockStore) {
				s.receipts["bk_conf_01"] = &ReceiptDTO{
					BookingID: "bk_conf_01",
					Status:    "confirmed",
					GuestDetails: GuestDetails{
						Email: "victim@example.com",
					},
				}
			},
			expectErr: ErrBookingNotFound,
			verifyRes: nil,
		},
		{
			name:       "Non-existent booking returns ErrBookingNotFound",
			email:      "rian@example.com",
			bookingID:  "bk_ghost",
			setupStore: func(s *mockStore) {},
			expectErr:  ErrBookingNotFound,
			verifyRes:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			tc.setupStore(store)
			svc := NewService(store, nil, slog.Default())

			res, err := svc.GetBookingReceipt(ctx, tc.email, tc.bookingID)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.verifyRes != nil {
				tc.verifyRes(t, res)
			}
		})
	}
}

func TestGuestService_GenerateCalendarICS_TableTest(t *testing.T) {
	tests := []struct {
		name      string
		receipt   *ReceiptDTO
		expectErr bool
		verify    func(t *testing.T, data []byte)
	}{
		{
			name: "Valid confirmed receipt generates valid RFC 5545 iCalendar stream",
			receipt: &ReceiptDTO{
				BookingID:        "bk-ics-123",
				BookingReference: "PKU-20261003-8F2A",
				Status:           "confirmed",
				HotelInfo: HotelInfo{
					Name: "Pulang ke Uttara",
				},
				StayDetails: StayDetails{
					CheckInDate:  "2026-10-10",
					CheckOutDate: "2026-10-12",
					TotalNights:  2,
				},
				GuestDetails: GuestDetails{
					Name:  "Rian Ardianto",
					Email: "rian@example.com",
				},
				RoomItem: RoomItemReceipt{
					RoomTypeName: "Executive King Suite",
					NumRooms:     1,
				},
			},
			expectErr: false,
			verify: func(t *testing.T, data []byte) {
				ics := string(data)
				expectedSubstrings := []string{
					"BEGIN:VCALENDAR\r\n",
					"VERSION:2.0\r\n",
					"PRODID:-//Pulang ke Uttara//Hotel Booking Engine v1.0//ID\r\n",
					"TZID:Asia/Jakarta\r\n",
					"BEGIN:VEVENT\r\n",
					"UID:booking-bk-ics-123@pulangkeuttara.id\r\n",
					"DTSTART;TZID=Asia/Jakarta:20261010T140000\r\n",
					"DTEND;TZID=Asia/Jakarta:20261012T120000\r\n",
					"SUMMARY:Menginap di Pulang ke Uttara (Executive King Suite)\r\n",
					"STATUS:CONFIRMED\r\n",
					"BEGIN:VALARM\r\n",
					"TRIGGER:-P1D\r\n",
					"END:VALARM\r\n",
					"END:VEVENT\r\n",
					"END:VCALENDAR\r\n",
				}
				for _, sub := range expectedSubstrings {
					if !strings.Contains(ics, sub) {
						t.Errorf("iCalendar stream missing expected element: %q", sub)
					}
				}
			},
		},
		{
			name:      "Nil receipt returns error",
			receipt:   nil,
			expectErr: true,
			verify:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			svc := NewService(store, nil, slog.Default())

			data, err := svc.GenerateCalendarICS(tc.receipt)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for nil receipt, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.verify != nil {
				tc.verify(t, data)
			}
		})
	}
}

func TestGuestService_GetSessionProfile_TableTest(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		session     *GuestSession
		setupStore  func(s *mockStore)
		expectErr   error
		verifyView  func(t *testing.T, pv *ProfileView)
	}{
		{
			name:      "Nil session returns ErrSessionNotFound",
			session:   nil,
			setupStore: func(s *mockStore) {},
			expectErr: ErrSessionNotFound,
		},
		{
			name: "Valid session returns active bookings count and profile",
			session: &GuestSession{
				GuestEmail:   "tamu@example.com",
				TokenHash:    "dummy_hash",
				ExpiresAt:    now.Add(24 * time.Hour),
				LastActiveAt: now,
			},
			setupStore: func(s *mockStore) {
				s.bookings = []BookingDetail{
					{GuestEmail: "tamu@example.com", Status: "confirmed"},
					{GuestEmail: "tamu@example.com", Status: "pending"},
					{GuestEmail: "tamu@example.com", Status: "cancelled"},
				}
			},
			expectErr: nil,
			verifyView: func(t *testing.T, pv *ProfileView) {
				if pv.Email != "tamu@example.com" {
					t.Errorf("expected email tamu@example.com, got %s", pv.Email)
				}
				if pv.ActiveBookingsCount != 2 {
					t.Errorf("expected 2 active bookings, got %d", pv.ActiveBookingsCount)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			tc.setupStore(store)
			svc := NewService(store, nil, slog.Default())

			pv, err := svc.GetSessionProfile(context.Background(), tc.session)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.verifyView != nil {
				tc.verifyView(t, pv)
			}
		})
	}
}

func TestGuestService_ComputeAllowedActions_TableTest(t *testing.T) {
	refTime := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	pastHold := refTime.Add(-10 * time.Minute)
	futureHold := refTime.Add(15 * time.Minute)

	tests := []struct {
		name     string
		detail   *BookingDetail
		now      time.Time
		expected AllowedActions
	}{
		{
			name:     "nil detail returns empty actions",
			detail:   nil,
			now:      refTime,
			expected: AllowedActions{},
		},
		{
			name: "pending with active hold",
			detail: &BookingDetail{
				Status:    "pending",
				ExpiresAt: &futureHold,
			},
			now: refTime,
			expected: AllowedActions{
				CanPay:               true,
				CanCancel:            true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "pending with expired hold",
			detail: &BookingDetail{
				Status:    "pending",
				ExpiresAt: &pastHold,
			},
			now: refTime,
			expected: AllowedActions{
				CanPay:               false,
				CanCancel:            false,
				CanRequestAssistance: true,
			},
		},
		{
			name: "pending with nil expires_at",
			detail: &BookingDetail{
				Status: "pending",
			},
			now: refTime,
			expected: AllowedActions{
				CanPay:               true,
				CanCancel:            true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "confirmed non_refundable cannot be cancelled",
			detail: &BookingDetail{
				Status:             "confirmed",
				CancellationPolicy: rates.PolicyNonRefundable,
				CheckIn:            "2026-10-10",
			},
			now: refTime,
			expected: AllowedActions{
				CanCancel:            false,
				CanDownloadReceipt:   true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "confirmed flexible_48h before deadline can be cancelled",
			detail: &BookingDetail{
				Status:             "confirmed",
				CancellationPolicy: rates.PolicyFlexible48h,
				CheckIn:            "2026-10-10",
			},
			// CheckIn 2026-10-10 14:00 WIB, deadline 2026-10-08 14:00 WIB (07:00 UTC).
			// refTime is 2026-10-03, well before deadline.
			now: refTime,
			expected: AllowedActions{
				CanCancel:            true,
				CanDownloadReceipt:   true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "confirmed flexible_48h after deadline cannot be cancelled",
			detail: &BookingDetail{
				Status:             "confirmed",
				CancellationPolicy: rates.PolicyFlexible48h,
				CheckIn:            "2026-10-10",
			},
			// After deadline: 2026-10-09 10:00 WIB
			now: time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC),
			expected: AllowedActions{
				CanCancel:            false,
				CanDownloadReceipt:   true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "confirmed empty policy defaults to flexible_48h before deadline",
			detail: &BookingDetail{
				Status:  "confirmed",
				CheckIn: "2026-10-10",
			},
			now: refTime,
			expected: AllowedActions{
				CanCancel:            true,
				CanDownloadReceipt:   true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "checked_in allows receipt and assistance",
			detail: &BookingDetail{
				Status: "checked_in",
			},
			now: refTime,
			expected: AllowedActions{
				CanDownloadReceipt:   true,
				CanRequestAssistance: true,
			},
		},
		{
			name: "checked_out allows receipt only",
			detail: &BookingDetail{
				Status: "checked_out",
			},
			now: refTime,
			expected: AllowedActions{
				CanDownloadReceipt: true,
			},
		},
		{
			name: "cancelled allows no actions",
			detail: &BookingDetail{
				Status: "cancelled",
			},
			now:      refTime,
			expected: AllowedActions{},
		},
		{
			name: "expired allows no actions",
			detail: &BookingDetail{
				Status: "expired",
			},
			now:      refTime,
			expected: AllowedActions{},
		},
		{
			name: "unknown status allows no actions",
			detail: &BookingDetail{
				Status: "unknown",
			},
			now:      refTime,
			expected: AllowedActions{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actions := computeAllowedActions(tc.detail, tc.now)
			if actions != tc.expected {
				t.Errorf("expected %+v, got %+v", tc.expected, actions)
			}
		})
	}
}

func TestGuestService_CaseInsensitiveEmailOwnership(t *testing.T) {
	store := newMockStore()
	store.bookings = []BookingDetail{
		{
			ID:                 "bk_owner_1",
			GuestEmail:         "Guest.Owner@EXAMPLE.com",
			Status:             "confirmed",
			CancellationPolicy: rates.PolicyFlexible48h,
			CheckIn:            "2026-10-10",
			CheckOut:           "2026-10-12",
			TotalPriceMinor:    1500000,
			Currency:           "IDR",
		},
	}
	store.receipts["bk_owner_1"] = &ReceiptDTO{
		BookingID:        "bk_owner_1",
		BookingReference: "PUL-2026-0001",
		Status:           "confirmed",
		GuestDetails: GuestDetails{
			Email: "Guest.Owner@EXAMPLE.com",
			Name:  "Guest Owner",
		},
		StayDetails: StayDetails{
			CheckInDate:  "2026-10-10",
			CheckOutDate: "2026-10-12",
			TotalNights:  2,
		},
	}

	svc := NewService(store, nil, slog.Default())
	svc.SetNowFunc(func() time.Time {
		return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	})

	ctx := context.Background()

	// 1. Count active bookings with lowercase email
	count, err := svc.GetSessionProfile(ctx, &GuestSession{
		GuestEmail: "guest.owner@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.ActiveBookingsCount != 1 {
		t.Errorf("expected 1 active booking, got %d", count.ActiveBookingsCount)
	}

	// 2. List bookings with mixed case email
	list, err := svc.ListBookings(ctx, "GUEST.owner@example.com", "all", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 booking in list, got %d", len(list))
	}

	// 3. Get detail with lowercase email and verify policy-driven allowed_actions
	detail, err := svc.GetBookingDetail(ctx, "guest.owner@example.com", "bk_owner_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !detail.AllowedActions.CanCancel || !detail.AllowedActions.CanDownloadReceipt {
		t.Errorf("expected CanCancel and CanDownloadReceipt to be true, got %+v", detail.AllowedActions)
	}

	// 4. Get receipt with lowercase email
	receipt, err := svc.GetBookingReceipt(ctx, "guest.owner@example.com", "bk_owner_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receipt.BookingID != "bk_owner_1" {
		t.Errorf("expected receipt for bk_owner_1, got %s", receipt.BookingID)
	}
}

func TestGuestService_ListBookings_Pagination(t *testing.T) {
	store := newMockStore()
	store.bookings = []BookingDetail{
		{GuestEmail: "tamu@example.com", Status: "confirmed"},
	}
	svc := NewService(store, nil, slog.Default())

	// Limit <= 0 should default to 20
	res1, err := svc.ListBookings(context.Background(), "tamu@example.com", "all", -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res1) != 1 {
		t.Errorf("expected 1 result, got %d", len(res1))
	}

	// Limit > 100 should default to 20
	res2, err := svc.ListBookings(context.Background(), "tamu@example.com", "all", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res2) != 1 {
		t.Errorf("expected 1 result, got %d", len(res2))
	}
}

