package guest

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type mockStore struct {
	challenges    map[string]*Challenge
	sessions      map[string]*GuestSession
	bookings      []BookingDetail
	createErr     error
	updateErr     error
	sessionErr    error
	listErr       error
	detailErr     error
}

func newMockStore() *mockStore {
	return &mockStore{
		challenges: make(map[string]*Challenge),
		sessions:   make(map[string]*GuestSession),
	}
}

func (m *mockStore) CreateChallenge(ctx context.Context, c *Challenge) error {
	if m.createErr != nil {
		return m.createErr
	}
	c.ID = "ch_001"
	m.challenges[c.Email] = c
	return nil
}

func (m *mockStore) GetLatestActiveChallenge(ctx context.Context, email string) (*Challenge, error) {
	c, ok := m.challenges[email]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (m *mockStore) UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error {
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
	for _, c := range m.challenges {
		if c.ID == id {
			c.VerifiedAt = &verifiedAt
		}
	}
	return nil
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
		if b.GuestEmail == email && (b.Status == "pending" || b.Status == "confirmed") {
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
		if b.GuestEmail == email {
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
		if b.ID == bookingID && b.GuestEmail == email {
			bCopy := b
			return &bCopy, nil
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
