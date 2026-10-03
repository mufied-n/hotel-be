package script

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/adapter/notifier"
	"github.com/example/hotel-booking/internal/adapter/payment"
	"github.com/example/hotel-booking/internal/api"
	"github.com/example/hotel-booking/internal/assistance"
	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/finance"
	"github.com/example/hotel-booking/internal/frontdesk"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/housekeeping"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/platform/featureflag"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/stay"
	"github.com/gin-gonic/gin"
)

type e2eIncrementCall struct {
	RoomTypeID string
	From       time.Time
	To         time.Time
	NumRooms   int
}

type e2eTxMock struct {
	booking         booking.Booking
	rooms           []string
	increments      []e2eIncrementCall
	otpNotifier     *e2eOTPNotifier
	guestStore      *e2eGuestStore
	finStore        *e2eFinanceStore
	hkStore         *e2eHousekeepingStore
	frontdeskStore  *e2eFrontDeskStore
	stayStore       *e2eStayStore
	assistanceStore *e2eAssistanceStore
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
	if to == booking.StatusCheckedOut && m.hkStore != nil {
		for _, r := range m.rooms {
			if room, ok := m.hkStore.rooms[r]; ok {
				room.CleanlinessStatus = housekeeping.StatusVacantDirty
			}
		}
	}
	return nil
}
func (m *e2eTxMock) PickAndAssignRooms(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	if m.hkStore != nil {
		for _, r := range m.rooms {
			if room, ok := m.hkStore.rooms[r]; ok {
				if room.CleanlinessStatus != housekeeping.StatusInspected {
					return nil, booking.ErrRoomNotReady
				}
				room.CleanlinessStatus = housekeeping.StatusOccupied
			}
		}
	}
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

func (s *e2eGuestStore) CreateChallengeWithCooldown(ctx context.Context, c *guest.Challenge, cooldown time.Duration) error {
	for _, ch := range s.challenges {
		if ch.Email == c.Email {
			diff := c.CreatedAt.Sub(ch.CreatedAt)
			if diff < cooldown {
				return guest.ErrRateLimited
			}
		}
	}
	c.ID = "ch-e2e-001"
	s.challenges[c.Email] = c
	return nil
}

func (s *e2eGuestStore) VerifyAndConsumeChallenge(ctx context.Context, email, inputHash string, now time.Time, newSession *guest.GuestSession) (*guest.GuestSession, error) {
	ch, ok := s.challenges[email]
	if !ok || ch == nil {
		return nil, guest.ErrInvalidOrExpiredCode
	}
	if ch.VerifiedAt != nil {
		return nil, guest.ErrInvalidOrExpiredCode
	}
	if now.After(ch.ExpiresAt) {
		return nil, guest.ErrInvalidOrExpiredCode
	}
	if ch.Attempts >= ch.MaxAttempts {
		return nil, guest.ErrMaxAttemptsExceeded
	}

	if ch.CodeHash != inputHash {
		ch.Attempts++
		if ch.Attempts >= ch.MaxAttempts {
			return nil, guest.ErrMaxAttemptsExceeded
		}
		return nil, guest.ErrInvalidOrExpiredCode
	}

	ch.VerifiedAt = &now
	newSession.ID = "sess-e2e-001"
	s.sessions[newSession.TokenHash] = newSession
	return newSession, nil
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
	if strings.EqualFold(strings.TrimSpace(s.tx.booking.GuestEmail), strings.TrimSpace(email)) {
		return 1, nil
	}
	return 0, nil
}

func (s *e2eGuestStore) ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error) {
	if strings.EqualFold(strings.TrimSpace(s.tx.booking.GuestEmail), strings.TrimSpace(email)) {
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
	if strings.EqualFold(strings.TrimSpace(s.tx.booking.GuestEmail), strings.TrimSpace(email)) && s.tx.booking.ID == bookingID {
		return &guest.BookingDetail{
			ID:                 s.tx.booking.ID,
			RoomTypeID:         s.tx.booking.RoomTypeID,
			RoomTypeName:       "Deluxe Premier",
			CheckIn:            s.tx.booking.CheckIn.Format("2006-01-02"),
			CheckOut:           s.tx.booking.CheckOut.Format("2006-01-02"),
			NumRooms:           s.tx.booking.NumRooms,
			NumGuests:          s.tx.booking.NumGuests,
			Status:             string(s.tx.booking.Status),
			TotalPriceMinor:    s.tx.booking.TotalPriceMinor,
			Currency:           s.tx.booking.Currency,
			GuestName:          s.tx.booking.GuestName,
			GuestEmail:         s.tx.booking.GuestEmail,
			GuestPhone:         s.tx.booking.GuestPhone,
			CancellationPolicy: s.tx.booking.CancellationPolicy,
			RatePlanCode:       s.tx.booking.RatePlanCode,
			ExpiresAt:          s.tx.booking.ExpiresAt,
			CreatedAt:          s.tx.booking.CreatedAt,
		}, nil
	}
	return nil, nil
}

func (s *e2eGuestStore) GetBookingReceiptData(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error) {
	if strings.EqualFold(strings.TrimSpace(s.tx.booking.GuestEmail), strings.TrimSpace(email)) && s.tx.booking.ID == bookingID {
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

func (n *e2eOTPNotifier) SendGuestOTP(ctx context.Context, email, otpCode string, challengeID ...string) error {
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

type e2eHousekeepingStore struct {
	rooms map[string]*housekeeping.RoomOperationalView
}

func newE2EHousekeepingStore() *e2eHousekeepingStore {
	store := &e2eHousekeepingStore{
		rooms: make(map[string]*housekeeping.RoomOperationalView),
	}
	// Kamar lantai 2 (201..205)
	for i := 201; i <= 205; i++ {
		roomNum := fmt.Sprintf("%d", i)
		store.rooms[roomNum] = &housekeeping.RoomOperationalView{
			RoomNumber:        roomNum,
			RoomTypeID:        "01900000-0000-7000-8000-000000000001",
			RoomTypeName:      "Superior King",
			Floor:             2,
			CleanlinessStatus: housekeeping.StatusVacantDirty,
			UpdatedAt:         time.Now().UTC(),
			UpdatedBy:         "system",
		}
	}
	// Kamar 301 (Lantai 3) default inspected untuk tes check-in awal
	store.rooms["301"] = &housekeeping.RoomOperationalView{
		RoomNumber:        "301",
		RoomTypeID:        "01900000-0000-7000-8000-000000000003",
		RoomTypeName:      "Deluxe King",
		Floor:             3,
		CleanlinessStatus: housekeeping.StatusInspected,
		UpdatedAt:         time.Now().UTC(),
		UpdatedBy:         "system",
	}
	return store
}

func (s *e2eHousekeepingStore) ListRooms(ctx context.Context, floor int, status string, roomTypeID string) ([]housekeeping.RoomOperationalView, error) {
	var res []housekeeping.RoomOperationalView
	for _, r := range s.rooms {
		if floor > 0 && r.Floor != floor {
			continue
		}
		if status != "" && string(r.CleanlinessStatus) != status {
			continue
		}
		if roomTypeID != "" && r.RoomTypeID != roomTypeID {
			continue
		}
		res = append(res, *r)
	}
	return res, nil
}

func (s *e2eHousekeepingStore) GetRoom(ctx context.Context, roomNumber string) (*housekeeping.RoomOperationalView, error) {
	r, ok := s.rooms[roomNumber]
	if !ok {
		return nil, housekeeping.ErrRoomNotFound
	}
	copied := *r
	return &copied, nil
}

func (s *e2eHousekeepingStore) UpdateRoomCleanliness(ctx context.Context, roomNumber string, status housekeeping.CleanlinessStatus, notes string, actor string) error {
	r, ok := s.rooms[roomNumber]
	if !ok {
		return housekeeping.ErrRoomNotFound
	}
	r.CleanlinessStatus = status
	r.MaintenanceNotes = notes
	r.UpdatedBy = actor
	r.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *e2eHousekeepingStore) DeductInventoryForOOO(ctx context.Context, roomTypeID string, startDate, endDate time.Time) error {
	return nil
}

type e2eFrontDeskStore struct {
	tx    *e2eTxMock
	notes []frontdesk.HandoverNote
}

func newE2EFrontDeskStore(tx *e2eTxMock) *e2eFrontDeskStore {
	return &e2eFrontDeskStore{
		tx: tx,
	}
}

func (s *e2eFrontDeskStore) GetDailyRoster(ctx context.Context, targetDate time.Time) (*frontdesk.DailyRoster, error) {
	occupied := 0
	if s.tx.booking.Status == booking.StatusCheckedIn {
		occupied = s.tx.booking.NumRooms
	}
	ooo := 0
	clean := 0
	dirty := 0
	inspected := 0
	if s.tx.hkStore != nil {
		for _, r := range s.tx.hkStore.rooms {
			switch r.CleanlinessStatus {
			case housekeeping.StatusOutOfOrder:
				ooo++
			case housekeeping.StatusOccupied:
				occupied++
			case housekeeping.StatusInspected:
				inspected++
			case housekeeping.StatusVacantClean:
				clean++
			case housekeeping.StatusVacantDirty:
				dirty++
			}
		}
	}
	sellable := 95 - ooo
	occRate := 0.0
	if sellable > 0 {
		occRate = (float64(occupied) / float64(sellable)) * 100.0
	}

	return &frontdesk.DailyRoster{
		Date: targetDate.Format("2006-01-02"),
		Metrics: frontdesk.RosterMetrics{
			TotalRooms:           95,
			SellableRooms:        sellable,
			OutOfOrderRooms:      ooo,
			OccupiedRooms:        occupied,
			VacantInspectedRooms: inspected,
			VacantDirtyRooms:     dirty,
			CleaningRooms:        clean,
			OccupancyRatePercent: occRate,
		},
		ExpectedArrivals: []frontdesk.ExpectedArrivalItem{
			{
				BookingID:            s.tx.booking.ID,
				GuestName:            s.tx.booking.GuestName,
				GuestPhone:           s.tx.booking.GuestPhone,
				RoomTypeID:           s.tx.booking.RoomTypeID,
				RoomTypeName:         "Superior King",
				AssignedRooms:        s.tx.rooms,
				NumRooms:             s.tx.booking.NumRooms,
				NumGuests:            s.tx.booking.NumGuests,
				EstimatedArrivalTime: "14:00",
				SpecialRequests:      "Quiet high floor room",
				TotalPriceMinor:      s.tx.booking.TotalPriceMinor,
			},
		},
		ExpectedDepartures: []frontdesk.ExpectedDepartureItem{
			{
				BookingID:    s.tx.booking.ID,
				GuestName:    s.tx.booking.GuestName,
				RoomNumbers:  s.tx.rooms,
				CheckInDate:  s.tx.booking.CheckIn.Format("2006-01-02"),
				CheckOutDate: s.tx.booking.CheckOut.Format("2006-01-02"),
			},
		},
		InHouseCount: occupied,
	}, nil
}

func (s *e2eFrontDeskStore) CreateHandoverNote(ctx context.Context, note *frontdesk.HandoverNote) error {
	note.ID = fmt.Sprintf("hnd-e2e-%03d", len(s.notes)+1)
	note.CreatedAt = time.Now().UTC()
	s.notes = append([]frontdesk.HandoverNote{*note}, s.notes...)
	return nil
}

func (s *e2eFrontDeskStore) ListHandoverNotes(ctx context.Context, limit, offset int) ([]frontdesk.HandoverNote, int, error) {
	total := len(s.notes)
	if offset >= total {
		return []frontdesk.HandoverNote{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return s.notes[offset:end], total, nil
}

type e2eStayStore struct {
	tx    *e2eTxMock
	moves []stay.RoomMoveLog
}

func newE2EStayStore(tx *e2eTxMock) *e2eStayStore {
	return &e2eStayStore{tx: tx}
}

func (s *e2eStayStore) GetBooking(ctx context.Context, bookingID string) (*stay.BookingDetails, error) {
	if s.tx.booking.ID != bookingID {
		return nil, stay.ErrBookingNotFound
	}
	currentRoom := ""
	if len(s.tx.rooms) > 0 {
		currentRoom = s.tx.rooms[len(s.tx.rooms)-1]
	}
	return &stay.BookingDetails{
		ID:              s.tx.booking.ID,
		Status:          string(s.tx.booking.Status),
		RoomTypeID:      s.tx.booking.RoomTypeID,
		CheckIn:         s.tx.booking.CheckIn,
		CheckOut:        s.tx.booking.CheckOut,
		NumRooms:        s.tx.booking.NumRooms,
		TotalPriceMinor: s.tx.booking.TotalPriceMinor,
		CurrentRoom:     currentRoom,
	}, nil
}

func (s *e2eStayStore) MoveRoom(ctx context.Context, input stay.RoomMoveInput, moveDate time.Time) (*stay.RoomMoveResult, error) {
	if s.tx.booking.ID != input.BookingID {
		return nil, stay.ErrBookingNotFound
	}
	if s.tx.booking.Status != booking.StatusCheckedIn {
		return nil, stay.ErrInvalidBookingStatus
	}
	currentRoom := ""
	if len(s.tx.rooms) > 0 {
		currentRoom = s.tx.rooms[0]
	}
	if currentRoom == input.TargetRoomNumber {
		return nil, stay.ErrSameRoomMove
	}

	targetRoom, ok := s.tx.hkStore.rooms[input.TargetRoomNumber]
	if !ok || targetRoom.CleanlinessStatus != housekeeping.StatusInspected {
		return nil, stay.ErrTargetRoomNotReady
	}

	if oldRoom, ok := s.tx.hkStore.rooms[currentRoom]; ok {
		oldRoom.CleanlinessStatus = housekeeping.StatusVacantDirty
		oldRoom.MaintenanceNotes = fmt.Sprintf("Room moved to %s: %s", input.TargetRoomNumber, input.Notes)
	}
	targetRoom.CleanlinessStatus = housekeeping.StatusOccupied

	s.tx.rooms = []string{input.TargetRoomNumber}

	logEntry := stay.RoomMoveLog{
		ID:             fmt.Sprintf("mov-e2e-%03d", len(s.moves)+1),
		BookingID:      input.BookingID,
		FromRoomNumber: currentRoom,
		ToRoomNumber:   input.TargetRoomNumber,
		MoveDate:       moveDate.Format("2006-01-02"),
		ReasonCategory: string(input.ReasonCategory),
		Notes:          input.Notes,
		ActorID:        input.ActorID,
		CreatedAt:      time.Now().UTC(),
	}
	s.moves = append([]stay.RoomMoveLog{logEntry}, s.moves...)

	return &stay.RoomMoveResult{
		Status:             "ok",
		BookingID:          input.BookingID,
		PreviousRoomNumber: currentRoom,
		NewRoomNumber:      input.TargetRoomNumber,
		MoveDate:           moveDate.Format("2006-01-02"),
		Message:            fmt.Sprintf("pemindahan kamar berhasil; kamar %s telah ditandai vacant_dirty", currentRoom),
	}, nil
}

func (s *e2eStayStore) ExtendStay(ctx context.Context, bookingID string, additionalNights int, newCheckOut time.Time, additionalRates []int64, additionalTotal int64) (*stay.ExtendStayResult, error) {
	if s.tx.booking.ID != bookingID {
		return nil, stay.ErrBookingNotFound
	}
	if s.tx.booking.Status != booking.StatusCheckedIn && s.tx.booking.Status != booking.StatusConfirmed {
		return nil, stay.ErrInvalidBookingStatus
	}

	oldCheckOut := s.tx.booking.CheckOut
	s.tx.booking.CheckOut = newCheckOut
	s.tx.booking.TotalPriceMinor += additionalTotal

	return &stay.ExtendStayResult{
		Status:                "ok",
		BookingID:             bookingID,
		PreviousCheckOut:      oldCheckOut.Format("2006-01-02"),
		NewCheckOut:           newCheckOut.Format("2006-01-02"),
		AdditionalNights:      additionalNights,
		AdditionalAmountMinor: additionalTotal,
		NewTotalPriceMinor:    s.tx.booking.TotalPriceMinor,
		PaymentStatus:         "settled",
	}, nil
}

func (s *e2eStayStore) ListRoomMoves(ctx context.Context, bookingID string) ([]stay.RoomMoveLog, error) {
	if s.tx.booking.ID != bookingID {
		return nil, stay.ErrBookingNotFound
	}
	return s.moves, nil
}

type e2eAssistanceStore struct {
	mu       sync.Mutex
	requests []assistance.SpecialRequest
	tx       *e2eTxMock
}

func newE2EAssistanceStore(tx *e2eTxMock) *e2eAssistanceStore {
	return &e2eAssistanceStore{tx: tx}
}

func (s *e2eAssistanceStore) CreateRequest(ctx context.Context, req assistance.SpecialRequest) (*assistance.SpecialRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req.ID = fmt.Sprintf("req-%d", len(s.requests)+1)
	s.requests = append(s.requests, req)
	res := req
	return &res, nil
}

func (s *e2eAssistanceStore) GetRequestByID(ctx context.Context, id string) (*assistance.SpecialRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.requests {
		if r.ID == id {
			res := r
			return &res, nil
		}
	}
	return nil, nil
}

func (s *e2eAssistanceStore) ListByBookingID(ctx context.Context, bookingID string) ([]assistance.SpecialRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []assistance.SpecialRequest
	for _, r := range s.requests {
		if r.BookingID == bookingID {
			res = append(res, r)
		}
	}
	return res, nil
}

func (s *e2eAssistanceStore) ListStaffQueue(ctx context.Context, filter assistance.ListFilter) ([]assistance.StaffQueueItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []assistance.StaffQueueItem
	for _, r := range s.requests {
		if filter.Department != "" && string(r.Department) != filter.Department {
			continue
		}
		if filter.Status != "" && string(r.Status) != filter.Status {
			continue
		}
		if filter.BookingID != "" && r.BookingID != filter.BookingID {
			continue
		}
		res = append(res, assistance.StaffQueueItem{
			SpecialRequest: r,
			GuestName:      s.tx.booking.GuestName,
			RoomNumbers:    s.tx.rooms,
			CheckInDate:    s.tx.booking.CheckIn.Format("2006-01-02"),
			CheckOutDate:   s.tx.booking.CheckOut.Format("2006-01-02"),
		})
	}
	return res, nil
}

func (s *e2eAssistanceStore) UpdateStatus(ctx context.Context, reqID string, toStatus assistance.Status, notes string, handledBy string, handledAt time.Time) (*assistance.SpecialRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.requests {
		if r.ID == reqID {
			s.requests[i].Status = toStatus
			s.requests[i].StaffNotes = notes
			s.requests[i].HandledBy = handledBy
			s.requests[i].HandledAt = &handledAt
			s.requests[i].UpdatedAt = handledAt
			res := s.requests[i]
			return &res, nil
		}
	}
	return nil, assistance.ErrRequestNotFound
}

func (s *e2eAssistanceStore) GetBookingOwner(ctx context.Context, bookingID string) (guestEmail string, exists bool, err error) {
	if bookingID == "bk-other-guest-unowned" {
		return "stranger@example.com", true, nil
	}
	return s.tx.booking.GuestEmail, true, nil
}

func newE2EFeatureFlagManager() featureflag.Manager {
	flags := map[string]featureflag.Flag{
		"ff_catalog_write":                {Key: "ff_catalog_write", Enabled: true, AllowedRoles: []string{"revenue_mgr", "gm_admin"}},
		"ff_multi_variant_search":         {Key: "ff_multi_variant_search", Enabled: true},
		"ff_quote_locking_engine":         {Key: "ff_quote_locking_engine", Enabled: true},
		"ff_promotions_engine":            {Key: "ff_promotions_engine", Enabled: true},
		"ff_checkout_idempotency":         {Key: "ff_checkout_idempotency", Enabled: true},
		"ff_pii_masking_guard":            {Key: "ff_pii_masking_guard", Enabled: true},
		"ff_strict_cancellation_policy":   {Key: "ff_strict_cancellation_policy", Enabled: true},
		"ff_xendit_payment_gateway":       {Key: "ff_xendit_payment_gateway", Enabled: true},
		"ff_resend_email_notifier":        {Key: "ff_resend_email_notifier", Enabled: true},
		"ff_guest_portal_auth":            {Key: "ff_guest_portal_auth", Enabled: true},
		"ff_guest_my_bookings":            {Key: "ff_guest_my_bookings", Enabled: true},
		"ff_booking_artifacts_receipt":    {Key: "ff_booking_artifacts_receipt", Enabled: true},
		"ff_booking_artifacts_icalendar":  {Key: "ff_booking_artifacts_icalendar", Enabled: true},
		"ff_finance_reconciliation":       {Key: "ff_finance_reconciliation", Enabled: true, AllowedRoles: []string{"finance", "gm_admin"}},
		"ff_gateway_automated_refund":     {Key: "ff_gateway_automated_refund", Enabled: true, AllowedRoles: []string{"finance", "gm_admin"}},
		"ff_housekeeping_board":           {Key: "ff_housekeeping_board", Enabled: true, AllowedRoles: []string{"housekeeping", "receptionist", "gm_admin"}},
		"ff_room_readiness_checkin_guard": {Key: "ff_room_readiness_checkin_guard", Enabled: true},
		"ff_front_desk_operations":        {Key: "ff_front_desk_operations", Enabled: true},
		"ff_stay_modification":            {Key: "ff_stay_modification", Enabled: true},
		"ff_guest_special_requests":       {Key: "ff_guest_special_requests", Enabled: true},
	}
	return featureflag.NewMemoryManager(flags)
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
		{"p", "housekeeping", "/api/v1/housekeeping/rooms", "GET"},
		{"p", "housekeeping", "/api/v1/housekeeping/rooms/:id/status", "PUT"},
		{"p", "receptionist", "/api/v1/housekeeping/rooms", "GET"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms", "POST"},
		{"p", "revenue_mgr", "/api/v1/catalog/rooms/:id", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "finance", "/api/v1/finance/refunds", "POST"},
		{"p", "finance", "/api/v1/finance/cases", "GET"},
		{"p", "finance", "/api/v1/finance/cases/:id/resolve", "POST"},
		{"p", "finance", "/api/v1/finance/reconciliations", "GET"},
		{"p", "receptionist", "/api/v1/front-desk/*", "*"},
		{"p", "housekeeping", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "housekeeping", "/api/v1/front-desk/special-requests", "GET"},
		{"p", "housekeeping", "/api/v1/front-desk/special-requests/:id/status", "PUT"},
		{"p", "revenue_mgr", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "finance", "/api/v1/front-desk/daily-roster", "GET"},
		{"p", "receptionist", "/api/v1/bookings/:id/room-move", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/extend-stay", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/room-moves", "GET"},
		{"p", "finance", "/api/v1/bookings/:id/room-moves", "GET"},
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
	catalogStore := catalog.NewMemoryStore(catalog.DefaultVariants())
	bkSvc.SetCatalogStore(catalogStore)
	rateEngine.SetBaseRateSource(catalogStore)

	xenditGw := payment.NewXendit("https://api.xendit.co", "test_xendit_sec", "test_e2e_xendit_webhook_token", "http://localhost:3000", nil)

	guestStore := newE2EGuestStore(tx)
	guestNotifier := &e2eOTPNotifier{}
	tx.otpNotifier = guestNotifier
	tx.guestStore = guestStore
	guestSvc := guest.NewService(guestStore, guestNotifier, nil)

	finStore := newE2EFinanceStore(tx)
	tx.finStore = finStore
	finSvc := finance.NewService(finStore, nil, nil)

	hkStore := newE2EHousekeepingStore()
	tx.hkStore = hkStore
	hkSvc := housekeeping.NewService(hkStore, nil)

	fdStore := newE2EFrontDeskStore(tx)
	tx.frontdeskStore = fdStore
	fdSvc := frontdesk.NewService(fdStore, nil)

	stayStore := newE2EStayStore(tx)
	tx.stayStore = stayStore
	staySvc := stay.NewService(stayStore, rateEngine, nil)

	astStore := newE2EAssistanceStore(tx)
	tx.assistanceStore = astStore
	astSvc := assistance.NewService(astStore, nil)

	handler := api.NewRouter(api.Deps{
		StaffAuth:       api.TestStaffVerifier(),
		BookingSvc:      bkSvc,
		InvStore:        inv,
		RateSvc:         ratesSvc,
		RateEngine:      rateEngine,
		QuoteStore:      quoteStore,
		CatalogStore:    catalogStore,
		Enforcer:        enforcer,
		IsDevelopment:   true,
		XenditGateway:   xenditGw,
		GuestSvc:        guestSvc,
		FinanceSvc:      finSvc,
		HousekeepingSvc: hkSvc,
		FrontDeskSvc:    fdSvc,
		StaySvc:         staySvc,
		AssistanceSvc:   astSvc,
		FeatureFlag:     newE2EFeatureFlagManager(),
		ReadyCheck:      func(ctx context.Context) error { return nil },
		FakePay: func(c *gin.Context) {
			bID := c.Query("booking_id")
			if bID == "" {
				bID = "bk-e2e-001"
			}
			if err := bkSvc.Confirm(c.Request.Context(), bID); err != nil {
				if errors.Is(err, booking.ErrHoldExpired) {
					c.Header("Content-Type", "application/json")
					c.Status(http.StatusConflict)
					_, _ = c.Writer.Write([]byte(`{"error":"hold has expired, room availability was released","code":"HOLD_EXPIRED"}`))
					return
				}
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"status":"confirmed"}`))
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
		req.Header.Set("Authorization", "Bearer "+"revenue_mgr")
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
		req.Header.Set("Authorization", "Bearer "+"revenue_mgr")
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
		req.Header.Set("Authorization", "Bearer "+"revenue_mgr")
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
		payload := []byte(fmt.Sprintf(`{
			"quote_id": %q,
			"terms_accepted": true,
			"privacy_accepted": true,
			"room_type_id": "01900000-0000-7000-8000-000000000001",
			"check_in": "2026-10-10",
			"check_out": "2026-10-12",
			"num_rooms": 1,
			"num_guests": 2,
			"guest_name": "Budi Santoso",
			"guest_email": "budi@example.com"
		}`, e2eQuoteID(t, client, srv.URL, "2026-10-10", "2026-10-12")))
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
		req.Header.Set("Authorization", "Bearer "+"receptionist")
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
		req.Header.Set("Authorization", "Bearer "+"housekeeping")
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
		req.Header.Set("Authorization", "Bearer "+"gm_admin")
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
			StaffAuth:     api.TestStaffVerifier(),
			Enforcer:      auth.DefaultTestEnforcer(),
			IsDevelopment: false,
			FakePay: func(c *gin.Context) {
				c.Status(http.StatusOK)
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
		"quote_id": "` + e2eQuoteID(t, client, srv.URL, checkIn, checkOut) + `",
		"terms_accepted": true,
		"privacy_accepted": true,
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
		tx.booking.CheckOut = today.Add(48 * time.Hour) // scheduled 2 nights ahead
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
		payload := fmt.Sprintf(`{"id": "inv_test_wh_2", "external_id": "bk-e2e-001", "status": "PAID", "amount": %d, "payment_method": "QRIS"}`, tx.booking.TotalPriceMinor)
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
		// Total booking mengikuti quote terkunci terakhir (termasuk pajak), sudah di-refund 500.000 di E2E-41.
		// Permintaan refund 5.000.000 jauh melebihi sisa saldo dan wajib ditolak dengan 409 Conflict.
		overBody := `{"booking_id":"bk-e2e-001","amount_minor":5000000,"reason":"Excessive refund attempt"}`
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

	// 46. Housekeeping Room Board Query & Floor Filter (FR-HK-01)
	t.Run("E2E-46: Housekeeping Room Board Query & Floor Filter (200 OK)", func(t *testing.T) {
		// 1. Staf Housekeeping memanggil GET /api/v1/housekeeping/rooms?floor=2
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/housekeeping/rooms?floor=2", nil)
		req.Header.Set("Authorization", "Bearer housekeeping")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("housekeeping board request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("housekeeping board status = %d, want 200", res.StatusCode)
		}

		var board map[string]any
		_ = json.NewDecoder(res.Body).Decode(&board)
		rooms, ok := board["rooms"].([]any)
		if !ok || len(rooms) != 5 {
			t.Fatalf("expected 5 rooms on floor 2, got %v", board["total_rooms"])
		}

		// 2. Resepsionis juga berwenang memantau status operasional kamar
		reqRec, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/housekeeping/rooms", nil)
		reqRec.Header.Set("Authorization", "Bearer receptionist")
		resRec, err := client.Do(reqRec)
		if err != nil || resRec.StatusCode != http.StatusOK {
			t.Fatalf("receptionist board access status = %d, want 200", resRec.StatusCode)
		}

		// 3. Guest ditolak dari housekeeping dashboard (403 Forbidden)
		reqGuest, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/housekeeping/rooms", nil)
		reqGuest.Header.Set("Authorization", "Bearer guest")
		resGuest, _ := client.Do(reqGuest)
		if resGuest.StatusCode != http.StatusForbidden {
			t.Errorf("guest board access status = %d, want 403", resGuest.StatusCode)
		}
	})

	// 47. Cleanliness Lifecycle Transition & Anti-Bypass Guard (FR-HK-02)
	t.Run("E2E-47: Cleanliness Lifecycle Transition & Anti-Bypass Guard (200 OK & 409 Conflict)", func(t *testing.T) {
		// 1. Room Attendant mulai membersihkan kamar 202: vacant_dirty -> cleaning
		reqStart, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/housekeeping/rooms/202/status",
			bytes.NewBufferString(`{"to_status":"cleaning","notes":"Attendant Ahmad started cleaning"}`))
		reqStart.Header.Set("Authorization", "Bearer housekeeping")
		reqStart.Header.Set("Content-Type", "application/json")
		resStart, err := client.Do(reqStart)
		if err != nil || resStart.StatusCode != http.StatusOK {
			t.Fatalf("start cleaning status = %d, want 200", resStart.StatusCode)
		}

		// 2. Room Attendant selesai membersihkan: cleaning -> vacant_clean
		reqClean, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/housekeeping/rooms/202/status",
			bytes.NewBufferString(`{"to_status":"vacant_clean","notes":"Linen replaced, amenities stocked"}`))
		reqClean.Header.Set("Authorization", "Bearer housekeeping")
		reqClean.Header.Set("Content-Type", "application/json")
		resClean, err := client.Do(reqClean)
		if err != nil || resClean.StatusCode != http.StatusOK {
			t.Fatalf("finish cleaning status = %d, want 200", resClean.StatusCode)
		}

		// 3. HK Supervisor melakukan QC dan inspeksi: vacant_clean -> inspected
		reqInspect, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/housekeeping/rooms/202/status",
			bytes.NewBufferString(`{"to_status":"inspected","notes":"QC inspection passed"}`))
		reqInspect.Header.Set("Authorization", "Bearer housekeeping")
		reqInspect.Header.Set("Content-Type", "application/json")
		resInspect, err := client.Do(reqInspect)
		if err != nil || resInspect.StatusCode != http.StatusOK {
			t.Fatalf("inspect room status = %d, want 200", resInspect.StatusCode)
		}

		// 4. Anti-Bypass Guard: Kamar 204 masih vacant_dirty, coba langsung loncat ke inspected -> 409 Conflict
		reqBypass, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/housekeeping/rooms/204/status",
			bytes.NewBufferString(`{"to_status":"inspected","notes":"Direct bypass attempt"}`))
		reqBypass.Header.Set("Authorization", "Bearer housekeeping")
		reqBypass.Header.Set("Content-Type", "application/json")
		resBypass, err := client.Do(reqBypass)
		if err != nil || resBypass.StatusCode != http.StatusConflict {
			t.Fatalf("bypass attempt status = %d, want 409 Conflict", resBypass.StatusCode)
		}
		var errResp map[string]any
		_ = json.NewDecoder(resBypass.Body).Decode(&errResp)
		if errResp["code"] != "INVALID_STATUS_TRANSITION" {
			t.Errorf("error code = %v, want INVALID_STATUS_TRANSITION", errResp["code"])
		}
	})

	// 48. Front Desk Check-In Guard Rejection on Dirty Room (FR-HK-04)
	t.Run("E2E-48: Front Desk Check-In Guard Rejection on Dirty Room (409 ROOM_NOT_READY)", func(t *testing.T) {
		// Pasang kamar 301 ke status vacant_dirty dan siapkan booking confirmed
		_ = tx.hkStore.UpdateRoomCleanliness(context.Background(), "301", housekeeping.StatusVacantDirty, "dirty", "system")
		tx.booking.Status = booking.StatusConfirmed

		// Resepsionis mencoba check-in tamu ke kamar kotor
		reqCheckIn, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		reqCheckIn.Header.Set("Authorization", "Bearer receptionist")
		resCheckIn, err := client.Do(reqCheckIn)
		if err != nil {
			t.Fatalf("check-in request failed: %v", err)
		}
		if resCheckIn.StatusCode != http.StatusConflict {
			t.Fatalf("check-in on dirty room status = %d, want 409 Conflict", resCheckIn.StatusCode)
		}

		var errResp map[string]any
		_ = json.NewDecoder(resCheckIn.Body).Decode(&errResp)
		if errResp["code"] != "ROOM_NOT_READY" {
			t.Errorf("error code = %v, want ROOM_NOT_READY", errResp["code"])
		}
	})

	// 49. Front Desk Check-In on Inspected Room & Auto-Occupied Transition (FR-HK-04)
	t.Run("E2E-49: Front Desk Check-In on Inspected Room & Auto-Occupied Transition (200 OK)", func(t *testing.T) {
		// HK Supervisor menginspeksi kamar 301 sehingga berstatus inspected
		_ = tx.hkStore.UpdateRoomCleanliness(context.Background(), "301", housekeeping.StatusInspected, "inspected and ready", "hk_supervisor")
		tx.booking.Status = booking.StatusConfirmed

		// Resepsionis check-in tamu
		reqCheckIn, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-in", nil)
		reqCheckIn.Header.Set("Authorization", "Bearer receptionist")
		resCheckIn, err := client.Do(reqCheckIn)
		if err != nil {
			t.Fatalf("check-in request failed: %v", err)
		}
		if resCheckIn.StatusCode != http.StatusOK {
			t.Fatalf("check-in status = %d, want 200 OK", resCheckIn.StatusCode)
		}

		// Verifikasi status kamar di housekeeping board berubah otomatis menjadi occupied
		room301, err := tx.hkStore.GetRoom(context.Background(), "301")
		if err != nil {
			t.Fatalf("failed to get room 301: %v", err)
		}
		if room301.CleanlinessStatus != housekeeping.StatusOccupied {
			t.Errorf("room 301 status = %v, want occupied", room301.CleanlinessStatus)
		}
	})

	// 50. Front Desk Check-Out & Auto-Dirty Transition (FR-HK-05)
	t.Run("E2E-50: Front Desk Check-Out & Auto-Dirty Transition (200 OK)", func(t *testing.T) {
		// Tamu melakukan check-out lewat front desk
		reqOut, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/check-out", nil)
		reqOut.Header.Set("Authorization", "Bearer receptionist")
		resOut, err := client.Do(reqOut)
		if err != nil {
			t.Fatalf("check-out request failed: %v", err)
		}
		if resOut.StatusCode != http.StatusOK {
			t.Fatalf("check-out status = %d, want 200 OK", resOut.StatusCode)
		}

		// Verifikasi status kamar 301 otomatis bertransisi menjadi vacant_dirty
		room301, err := tx.hkStore.GetRoom(context.Background(), "301")
		if err != nil {
			t.Fatalf("failed to get room 301: %v", err)
		}
		if room301.CleanlinessStatus != housekeeping.StatusVacantDirty {
			t.Errorf("room 301 status = %v, want vacant_dirty", room301.CleanlinessStatus)
		}
	})

	// 51. GM Admin Out-of-Order (OOO) Isolation & Non-GM Rejection (FR-HK-03)
	t.Run("E2E-51: GM Admin Out-of-Order (OOO) Isolation & Non-GM Rejection (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Resepsionis mencoba menetapkan status OOO -> 403 Forbidden
		reqNonGM, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/housekeeping/rooms/203/out-of-order",
			bytes.NewBufferString(`{"start_date":"2026-10-10","end_date":"2026-10-15","reason":"AC repair"}`))
		reqNonGM.Header.Set("Authorization", "Bearer receptionist")
		reqNonGM.Header.Set("Content-Type", "application/json")
		resNonGM, _ := client.Do(reqNonGM)
		if resNonGM.StatusCode != http.StatusForbidden {
			t.Errorf("receptionist OOO status = %d, want 403 Forbidden", resNonGM.StatusCode)
		}

		// 2. GM Admin menetapkan kamar 203 menjadi Out of Order -> 200 OK
		reqGM, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/housekeeping/rooms/203/out-of-order",
			bytes.NewBufferString(`{"start_date":"2026-10-10","end_date":"2026-10-15","reason":"Major bathroom plumbing overhaul"}`))
		reqGM.Header.Set("Authorization", "Bearer gm_admin")
		reqGM.Header.Set("Content-Type", "application/json")
		resGM, err := client.Do(reqGM)
		if err != nil {
			t.Fatalf("gm OOO request failed: %v", err)
		}
		if resGM.StatusCode != http.StatusOK {
			t.Fatalf("gm OOO status = %d, want 200 OK", resGM.StatusCode)
		}

		var oooResp map[string]any
		_ = json.NewDecoder(resGM.Body).Decode(&oooResp)
		if oooResp["cleanliness_status"] != "out_of_order" {
			t.Errorf("cleanliness_status = %v, want out_of_order", oooResp["cleanliness_status"])
		}

		// Verifikasi status kamar 203 di store adalah out_of_order
		room203, _ := tx.hkStore.GetRoom(context.Background(), "203")
		if room203.CleanlinessStatus != housekeeping.StatusOutOfOrder {
			t.Errorf("room 203 status = %v, want out_of_order", room203.CleanlinessStatus)
		}
	})

	// 52. Front Desk Daily Operations Roster Query & Multi-Role RBAC (FR-FDR-01, FR-FDR-02)
	t.Run("E2E-52: Front Desk Daily Operations Roster Query & Multi-Role RBAC (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest mencoba akses roster -> 403 Forbidden
		reqGuest, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster", nil)
		resGuest, err := client.Do(reqGuest)
		if err != nil {
			t.Fatalf("guest roster request failed: %v", err)
		}
		if resGuest.StatusCode != http.StatusForbidden {
			t.Errorf("guest roster status = %d, want 403 Forbidden", resGuest.StatusCode)
		}

		// 2. Receptionist akses roster -> 200 OK
		reqRecep, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster", nil)
		reqRecep.Header.Set("Authorization", "Bearer receptionist")
		resRecep, err := client.Do(reqRecep)
		if err != nil {
			t.Fatalf("receptionist roster request failed: %v", err)
		}
		if resRecep.StatusCode != http.StatusOK {
			t.Fatalf("receptionist roster status = %d, want 200 OK", resRecep.StatusCode)
		}

		var rosterResp map[string]any
		_ = json.NewDecoder(resRecep.Body).Decode(&rosterResp)
		if rosterResp["date"] == "" {
			t.Error("expected non-empty date")
		}
		metrics, ok := rosterResp["metrics"].(map[string]any)
		if !ok {
			t.Fatalf("expected metrics object in response, got %v", rosterResp["metrics"])
		}
		if metrics["total_rooms"] != float64(95) {
			t.Errorf("total_rooms = %v, want 95", metrics["total_rooms"])
		}
		if metrics["sellable_rooms"] != float64(94) { // kamar 203 berstatus OOO di E2E-51
			t.Errorf("sellable_rooms = %v, want 94", metrics["sellable_rooms"])
		}
		if metrics["out_of_order_rooms"] != float64(1) {
			t.Errorf("out_of_order_rooms = %v, want 1", metrics["out_of_order_rooms"])
		}

		// 3. Housekeeping akses roster -> 200 OK
		reqHK, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster", nil)
		reqHK.Header.Set("Authorization", "Bearer housekeeping")
		resHK, err := client.Do(reqHK)
		if err != nil {
			t.Fatalf("housekeeping roster request failed: %v", err)
		}
		if resHK.StatusCode != http.StatusOK {
			t.Errorf("housekeeping roster status = %d, want 200 OK", resHK.StatusCode)
		}

		// 4. Revenue Manager akses roster -> 200 OK
		reqRev, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster", nil)
		reqRev.Header.Set("Authorization", "Bearer revenue_mgr")
		resRev, err := client.Do(reqRev)
		if err != nil {
			t.Fatalf("revenue_mgr roster request failed: %v", err)
		}
		if resRev.StatusCode != http.StatusOK {
			t.Errorf("revenue_mgr roster status = %d, want 200 OK", resRev.StatusCode)
		}
	})

	// 53. Front Desk Daily Roster Forecast Date Query & Validation (FR-FDR-01)
	t.Run("E2E-53: Front Desk Daily Roster Forecast Date Query & Validation (200 OK & 400 Bad Request)", func(t *testing.T) {
		// 1. Format tanggal salah -> 400 Bad Request
		reqBadDate, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster?date=10-10-2026", nil)
		reqBadDate.Header.Set("Authorization", "Bearer receptionist")
		resBadDate, err := client.Do(reqBadDate)
		if err != nil {
			t.Fatalf("bad date request failed: %v", err)
		}
		if resBadDate.StatusCode != http.StatusBadRequest {
			t.Errorf("bad date status = %d, want 400 Bad Request", resBadDate.StatusCode)
		}

		// 2. Format tanggal valid YYYY-MM-DD -> 200 OK
		reqValidDate, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/daily-roster?date=2026-10-10", nil)
		reqValidDate.Header.Set("Authorization", "Bearer receptionist")
		resValidDate, err := client.Do(reqValidDate)
		if err != nil {
			t.Fatalf("valid date request failed: %v", err)
		}
		if resValidDate.StatusCode != http.StatusOK {
			t.Fatalf("valid date status = %d, want 200 OK", resValidDate.StatusCode)
		}

		var dateResp map[string]any
		_ = json.NewDecoder(resValidDate.Body).Decode(&dateResp)
		if dateResp["date"] != "2026-10-10" {
			t.Errorf("date = %v, want 2026-10-10", dateResp["date"])
		}
	})

	// 54. Front Desk Record Shift Handover Note (FR-FDR-03)
	t.Run("E2E-54: Front Desk Record Shift Handover Note (201 Created, 400 Bad Request, & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest mencoba catat serah terima shift -> 403 Forbidden
		bodyNote := `{"shift":"morning","cash_float_minor":1500000,"pending_issues":"Kunci kamar 201 perlu baterai baru","vip_guest_notes":"VIP Mr. Tan check-in jam 14.00"}`
		reqGuestNote, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/front-desk/handover-notes", bytes.NewBufferString(bodyNote))
		reqGuestNote.Header.Set("Content-Type", "application/json")
		resGuestNote, _ := client.Do(reqGuestNote)
		if resGuestNote.StatusCode != http.StatusForbidden {
			t.Errorf("guest handover note status = %d, want 403 Forbidden", resGuestNote.StatusCode)
		}

		// 2. Resepsionis kirim input shift tidak valid -> 400 Bad Request
		bodyInvalidShift := `{"shift":"evening","cash_float_minor":1500000,"pending_issues":"Issue"}`
		reqInvalidShift, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/front-desk/handover-notes", bytes.NewBufferString(bodyInvalidShift))
		reqInvalidShift.Header.Set("Authorization", "Bearer receptionist")
		reqInvalidShift.Header.Set("Content-Type", "application/json")
		resInvalidShift, _ := client.Do(reqInvalidShift)
		if resInvalidShift.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid shift status = %d, want 400 Bad Request", resInvalidShift.StatusCode)
		}

		// 3. Resepsionis kirim catatan kosong -> 400 Bad Request
		bodyEmptyNote := `{"shift":"morning","cash_float_minor":1500000,"pending_issues":"","vip_guest_notes":""}`
		reqEmptyNote, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/front-desk/handover-notes", bytes.NewBufferString(bodyEmptyNote))
		reqEmptyNote.Header.Set("Authorization", "Bearer receptionist")
		reqEmptyNote.Header.Set("Content-Type", "application/json")
		resEmptyNote, _ := client.Do(reqEmptyNote)
		if resEmptyNote.StatusCode != http.StatusBadRequest {
			t.Errorf("empty note status = %d, want 400 Bad Request", resEmptyNote.StatusCode)
		}

		// 4. Resepsionis sukses mencatat shift serah terima pagi -> 201 Created
		reqValidNote, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/front-desk/handover-notes", bytes.NewBufferString(bodyNote))
		reqValidNote.Header.Set("Authorization", "Bearer receptionist")
		reqValidNote.Header.Set("Content-Type", "application/json")
		resValidNote, err := client.Do(reqValidNote)
		if err != nil {
			t.Fatalf("valid note request failed: %v", err)
		}
		if resValidNote.StatusCode != http.StatusCreated {
			t.Fatalf("valid note status = %d, want 201 Created", resValidNote.StatusCode)
		}

		var createdResp map[string]any
		_ = json.NewDecoder(resValidNote.Body).Decode(&createdResp)
		createdNote, ok := createdResp["note"].(map[string]any)
		if !ok {
			t.Fatalf("expected note object in response, got %v", createdResp)
		}
		if createdNote["id"] == "" {
			t.Error("expected non-empty handover note ID")
		}
		if createdNote["shift"] != "morning" {
			t.Errorf("shift = %v, want morning", createdNote["shift"])
		}
		if createdNote["actor_role"] != "receptionist" {
			t.Errorf("actor_role = %v, want receptionist", createdNote["actor_role"])
		}
	})

	// 55. Front Desk List Shift Handover Notes History (FR-FDR-03)
	t.Run("E2E-55: Front Desk List Shift Handover Notes History (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Resepsionis catat shift kedua (afternoon)
		bodyAfternoon := `{"shift":"afternoon","cash_float_minor":1500000,"pending_issues":"Kunci 201 fixed","vip_guest_notes":"Mr. Tan in-house"}`
		reqAfternoon, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/front-desk/handover-notes", bytes.NewBufferString(bodyAfternoon))
		reqAfternoon.Header.Set("Authorization", "Bearer receptionist")
		reqAfternoon.Header.Set("Content-Type", "application/json")
		resAfternoon, _ := client.Do(reqAfternoon)
		if resAfternoon.StatusCode != http.StatusCreated {
			t.Fatalf("afternoon note status = %d, want 201 Created", resAfternoon.StatusCode)
		}

		// 2. Resepsionis baca riwayat handover logbook -> 200 OK
		reqList, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/handover-notes?limit=10&offset=0", nil)
		reqList.Header.Set("Authorization", "Bearer receptionist")
		resList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("list notes request failed: %v", err)
		}
		if resList.StatusCode != http.StatusOK {
			t.Fatalf("list notes status = %d, want 200 OK", resList.StatusCode)
		}

		var listResp map[string]any
		_ = json.NewDecoder(resList.Body).Decode(&listResp)
		if listResp["total"] != float64(2) {
			t.Errorf("total notes = %v, want 2", listResp["total"])
		}
		notesList, ok := listResp["notes"].([]any)
		if !ok || len(notesList) != 2 {
			t.Fatalf("notes count = %v, want 2", len(notesList))
		}

		// 3. Housekeeping mencoba baca catatan serah terima shift meja depan -> 403 Forbidden
		reqHKNotes, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/handover-notes", nil)
		reqHKNotes.Header.Set("Authorization", "Bearer housekeeping")
		resHKNotes, err := client.Do(reqHKNotes)
		if err != nil {
			t.Fatalf("housekeeping list notes failed: %v", err)
		}
		if resHKNotes.StatusCode != http.StatusForbidden {
			t.Errorf("housekeeping list notes status = %d, want 403 Forbidden", resHKNotes.StatusCode)
		}
	})

	// 56. Mid-Stay Room Move on Inspected Room & Auto-Dirty Transition (FR-STAY-01)
	t.Run("E2E-56: Mid-Stay Room Move on Inspected Room & Auto-Dirty Transition (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest mencoba memindahkan kamar -> 403 Forbidden
		bodyMove := `{"target_room_number":"202","reason_category":"maintenance_defect","notes":"AC 301 bocor"}`
		reqGuestMove, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/room-move", bytes.NewBufferString(bodyMove))
		reqGuestMove.Header.Set("Content-Type", "application/json")
		resGuestMove, _ := client.Do(reqGuestMove)
		if resGuestMove.StatusCode != http.StatusForbidden {
			t.Errorf("guest room move status = %d, want 403 Forbidden", resGuestMove.StatusCode)
		}

		// Siapkan status booking checked_in di kamar 301 dan kamar 202 inspected
		tx.booking.Status = booking.StatusCheckedIn
		tx.rooms = []string{"301"}
		_ = tx.hkStore.UpdateRoomCleanliness(context.Background(), "202", housekeeping.StatusInspected, "inspected and ready", "hk_supervisor")

		// 2. Receptionist memindahkan tamu dari 301 ke 202 -> 200 OK
		reqRecepMove, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/room-move", bytes.NewBufferString(bodyMove))
		reqRecepMove.Header.Set("Authorization", "Bearer receptionist")
		reqRecepMove.Header.Set("Content-Type", "application/json")
		resRecepMove, err := client.Do(reqRecepMove)
		if err != nil {
			t.Fatalf("receptionist room move failed: %v", err)
		}
		if resRecepMove.StatusCode != http.StatusOK {
			t.Fatalf("receptionist room move status = %d, want 200 OK", resRecepMove.StatusCode)
		}

		var moveResp map[string]any
		_ = json.NewDecoder(resRecepMove.Body).Decode(&moveResp)
		if moveResp["previous_room_number"] != "301" {
			t.Errorf("previous_room_number = %v, want 301", moveResp["previous_room_number"])
		}
		if moveResp["new_room_number"] != "202" {
			t.Errorf("new_room_number = %v, want 202", moveResp["new_room_number"])
		}

		// Verifikasi status kebersihan di housekeeping board: 301 -> vacant_dirty, 202 -> occupied
		room301, _ := tx.hkStore.GetRoom(context.Background(), "301")
		room202, _ := tx.hkStore.GetRoom(context.Background(), "202")
		if room301.CleanlinessStatus != housekeeping.StatusVacantDirty {
			t.Errorf("room 301 status = %v, want vacant_dirty", room301.CleanlinessStatus)
		}
		if room202.CleanlinessStatus != housekeeping.StatusOccupied {
			t.Errorf("room 202 status = %v, want occupied", room202.CleanlinessStatus)
		}
	})

	// 57. Room Move Readiness Guard Rejection on Dirty Target Room (FR-STAY-01)
	t.Run("E2E-57: Room Move Readiness Guard Rejection on Dirty Target Room (409 Conflict)", func(t *testing.T) {
		// Kamar 204 berstatus vacant_dirty
		_ = tx.hkStore.UpdateRoomCleanliness(context.Background(), "204", housekeeping.StatusVacantDirty, "uncleaned", "system")

		bodyMoveDirty := `{"target_room_number":"204","reason_category":"guest_request","notes":"Wants lower floor"}`
		reqMoveDirty, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/room-move", bytes.NewBufferString(bodyMoveDirty))
		reqMoveDirty.Header.Set("Authorization", "Bearer receptionist")
		reqMoveDirty.Header.Set("Content-Type", "application/json")
		resMoveDirty, err := client.Do(reqMoveDirty)
		if err != nil {
			t.Fatalf("room move to dirty request failed: %v", err)
		}
		if resMoveDirty.StatusCode != http.StatusConflict {
			t.Fatalf("room move to dirty status = %d, want 409 Conflict", resMoveDirty.StatusCode)
		}

		var errResp map[string]any
		_ = json.NewDecoder(resMoveDirty.Body).Decode(&errResp)
		if errResp["code"] != "TARGET_ROOM_NOT_READY" {
			t.Errorf("error code = %v, want TARGET_ROOM_NOT_READY", errResp["code"])
		}
	})

	// 58. Stay Extension with Dynamic Pricing & Inventory Calculation (FR-STAY-02)
	t.Run("E2E-58: Stay Extension with Dynamic Pricing & Inventory Calculation (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest mencoba panggil extend stay -> 403 Forbidden
		bodyExtend := `{"additional_nights":2,"payment_method":"front_desk_edc"}`
		reqGuestExt, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/extend-stay", bytes.NewBufferString(bodyExtend))
		reqGuestExt.Header.Set("Content-Type", "application/json")
		resGuestExt, _ := client.Do(reqGuestExt)
		if resGuestExt.StatusCode != http.StatusForbidden {
			t.Errorf("guest extend stay status = %d, want 403 Forbidden", resGuestExt.StatusCode)
		}

		oldCheckOut := tx.booking.CheckOut

		// 2. Receptionist memperpanjang masa menginap 2 malam -> 200 OK
		reqRecepExt, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/extend-stay", bytes.NewBufferString(bodyExtend))
		reqRecepExt.Header.Set("Authorization", "Bearer receptionist")
		reqRecepExt.Header.Set("Content-Type", "application/json")
		resRecepExt, err := client.Do(reqRecepExt)
		if err != nil {
			t.Fatalf("receptionist extend stay request failed: %v", err)
		}
		if resRecepExt.StatusCode != http.StatusOK {
			t.Fatalf("receptionist extend stay status = %d, want 200 OK", resRecepExt.StatusCode)
		}

		var extResp map[string]any
		_ = json.NewDecoder(resRecepExt.Body).Decode(&extResp)
		if extResp["additional_nights"] != float64(2) {
			t.Errorf("additional_nights = %v, want 2", extResp["additional_nights"])
		}
		if extResp["previous_check_out"] != oldCheckOut.Format("2006-01-02") {
			t.Errorf("previous_check_out = %v, want %s", extResp["previous_check_out"], oldCheckOut.Format("2006-01-02"))
		}
		expectedNewCheckOut := oldCheckOut.AddDate(0, 0, 2).Format("2006-01-02")
		if extResp["new_check_out"] != expectedNewCheckOut {
			t.Errorf("new_check_out = %v, want %s", extResp["new_check_out"], expectedNewCheckOut)
		}
		if extResp["additional_amount_minor"].(float64) <= 0 {
			t.Error("expected additional_amount_minor > 0")
		}
	})

	// 59. Stay Extension Validation: Rejection on Zero Nights (FR-STAY-02)
	t.Run("E2E-59: Stay Extension Validation: Rejection on Zero Nights (400 Bad Request)", func(t *testing.T) {
		bodyZero := `{"additional_nights":0}`
		reqZero, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/bookings/bk-e2e-001/extend-stay", bytes.NewBufferString(bodyZero))
		reqZero.Header.Set("Authorization", "Bearer receptionist")
		reqZero.Header.Set("Content-Type", "application/json")
		resZero, err := client.Do(reqZero)
		if err != nil {
			t.Fatalf("zero nights request failed: %v", err)
		}
		if resZero.StatusCode != http.StatusBadRequest {
			t.Fatalf("zero nights status = %d, want 400 Bad Request", resZero.StatusCode)
		}

		var errResp map[string]any
		_ = json.NewDecoder(resZero.Body).Decode(&errResp)
		if errResp["code"] != "INVALID_ADDITIONAL_NIGHTS" {
			t.Errorf("error code = %v, want INVALID_ADDITIONAL_NIGHTS", errResp["code"])
		}
	})

	// 60. Room Move Audit Log Inquiry & RBAC Isolation (FR-STAY-03)
	t.Run("E2E-60: Room Move Audit Log Inquiry & RBAC Isolation (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest mencoba membaca riwayat pemindahan kamar -> 403 Forbidden
		reqGuestMoves, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001/room-moves", nil)
		resGuestMoves, _ := client.Do(reqGuestMoves)
		if resGuestMoves.StatusCode != http.StatusForbidden {
			t.Errorf("guest list room moves status = %d, want 403 Forbidden", resGuestMoves.StatusCode)
		}

		// 2. Receptionist membaca riwayat pemindahan kamar -> 200 OK
		reqRecepMoves, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/bookings/bk-e2e-001/room-moves", nil)
		reqRecepMoves.Header.Set("Authorization", "Bearer receptionist")
		resRecepMoves, err := client.Do(reqRecepMoves)
		if err != nil {
			t.Fatalf("receptionist list room moves failed: %v", err)
		}
		if resRecepMoves.StatusCode != http.StatusOK {
			t.Fatalf("receptionist list room moves status = %d, want 200 OK", resRecepMoves.StatusCode)
		}

		var listResp map[string]any
		_ = json.NewDecoder(resRecepMoves.Body).Decode(&listResp)
		moves, ok := listResp["moves"].([]any)
		if !ok || len(moves) == 0 {
			t.Fatalf("expected moves array in response, got %v", listResp["moves"])
		}
		firstMove, ok := moves[0].(map[string]any)
		if !ok {
			t.Fatalf("expected move object in moves[0], got %v", moves[0])
		}
		if firstMove["from_room_number"] != "301" || firstMove["to_room_number"] != "202" {
			t.Errorf("unexpected move log: %v", firstMove)
		}
		if firstMove["reason_category"] != "maintenance_defect" {
			t.Errorf("reason_category = %v, want maintenance_defect", firstMove["reason_category"])
		}
	})

	// 61. E2E Feature Flags & Runtime Configuration Lifecycle
	t.Run("E2E-61: Feature Flags & Runtime Configuration Lifecycle", func(t *testing.T) {
		// 1. Receptionist mencoba GET /api/v1/admin/feature-flags -> 403 Forbidden (Casbin RBAC)
		reqRec, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/feature-flags", nil)
		reqRec.Header.Set("Authorization", "Bearer receptionist")
		resRec, err := client.Do(reqRec)
		if err != nil || resRec.StatusCode != http.StatusForbidden {
			t.Fatalf("receptionist admin flags status = %d, want 403 Forbidden", resRec.StatusCode)
		}

		// 2. GM Admin membaca daftar feature flags -> 200 OK
		reqGM, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/feature-flags", nil)
		reqGM.Header.Set("Authorization", "Bearer gm_admin")
		resGM, err := client.Do(reqGM)
		if err != nil || resGM.StatusCode != http.StatusOK {
			t.Fatalf("gm_admin flags status = %d, want 200 OK", resGM.StatusCode)
		}
		var listResp struct {
			Total int `json:"total"`
		}
		_ = json.NewDecoder(resGM.Body).Decode(&listResp)
		if listResp.Total < 17 {
			t.Errorf("total flags = %d, want at least 17", listResp.Total)
		}

		// 3. Matikan flag ff_multi_variant_search via PUT
		updatePayload := `{"enabled": false, "allowed_roles": []}`
		reqToggle, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/admin/feature-flags/ff_multi_variant_search", bytes.NewBufferString(updatePayload))
		reqToggle.Header.Set("Authorization", "Bearer gm_admin")
		reqToggle.Header.Set("Content-Type", "application/json")
		resToggle, err := client.Do(reqToggle)
		if err != nil || resToggle.StatusCode != http.StatusOK {
			t.Fatalf("toggle flag status = %d, want 200 OK", resToggle.StatusCode)
		}

		// 4. Guest akses GET /api/v1/search -> 503 Service Unavailable (FEATURE_DISABLED)
		reqSearch, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=1", nil)
		resSearch, err := client.Do(reqSearch)
		if err != nil || resSearch.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("disabled search status = %d, want 503 Service Unavailable", resSearch.StatusCode)
		}

		// 5. Restore flag ff_multi_variant_search via PUT
		restorePayload := `{"enabled": true, "allowed_roles": []}`
		reqRestore, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/admin/feature-flags/ff_multi_variant_search", bytes.NewBufferString(restorePayload))
		reqRestore.Header.Set("Authorization", "Bearer gm_admin")
		reqRestore.Header.Set("Content-Type", "application/json")
		resRestore, err := client.Do(reqRestore)
		if err != nil || resRestore.StatusCode != http.StatusOK {
			t.Fatalf("restore flag status = %d, want 200 OK", resRestore.StatusCode)
		}

		// 6. Guest akses GET /api/v1/search -> 200 OK pulih
		reqSearch2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=1", nil)
		resSearch2, err := client.Do(reqSearch2)
		if err != nil || resSearch2.StatusCode != http.StatusOK {
			t.Fatalf("restored search status = %d, want 200 OK", resSearch2.StatusCode)
		}
	})

	var specialRequestID string
	var assistGuestSessionToken string

	// 62. Guest submits structured special request (auto-routed to housekeeping)
	t.Run("E2E-62: Guest submits special request & auto-routes to Housekeeping (201 Created)", func(t *testing.T) {
		targetEmail := tx.booking.GuestEmail
		targetBookingID := tx.booking.ID

		// 1. Dapatkan sesi aktif tamu via challenge & verify (reset cooldown dari tes sebelumnya)
		delete(tx.guestStore.challenges, targetEmail)
		chalRes, _ := client.Post(srv.URL+"/api/v1/auth/guest/challenge", "application/json", strings.NewReader(fmt.Sprintf(`{"email":"%s"}`, targetEmail)))
		if chalRes.StatusCode != http.StatusOK {
			t.Fatalf("challenge failed: %d", chalRes.StatusCode)
		}
		verBody, _ := json.Marshal(map[string]string{"email": targetEmail, "code": tx.otpNotifier.lastOTP})
		verRes, _ := client.Post(srv.URL+"/api/v1/auth/guest/verify", "application/json", bytes.NewReader(verBody))
		var verResp map[string]any
		_ = json.NewDecoder(verRes.Body).Decode(&verResp)
		assistGuestSessionToken = verResp["token"].(string)

		// 2. Submit special request
		reqPayload := `{"category":"celebration_setup","description":"Anniversary ke-5, mohon handuk angsa dan kartu ucapan","target_time":"15:00"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/guest/bookings/"+targetBookingID+"/special-requests", strings.NewReader(reqPayload))
		req.Header.Set("Authorization", "Bearer "+assistGuestSessionToken)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		b, _ := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != http.StatusCreated {
			t.Fatalf("submit special request status = %d, want 201 Created, body: %s", res.StatusCode, string(b))
		}

		var created assistance.SpecialRequest
		_ = json.NewDecoder(bytes.NewReader(b)).Decode(&created)
		if created.Department != assistance.DepartmentHousekeeping {
			t.Errorf("expected department housekeeping, got %s", created.Department)
		}
		if created.Status != assistance.StatusPending {
			t.Errorf("expected status pending, got %s", created.Status)
		}
		specialRequestID = created.ID
	})

	// 63. Anti-IDOR Defense on Guest Special Requests
	t.Run("E2E-63: Guest special request Anti-IDOR defense (404 Not Found)", func(t *testing.T) {
		reqPayload := `{"category":"baby_crib","description":"Boks bayi untuk balita"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/guest/bookings/bk-other-guest-unowned/special-requests", strings.NewReader(reqPayload))
		req.Header.Set("Authorization", "Bearer "+assistGuestSessionToken)
		req.Header.Set("Content-Type", "application/json")
		res, _ := client.Do(req)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for unowned booking IDOR attempt, got %d", res.StatusCode)
		}
	})

	// 64. Staff inspects departmental special requests queue & RBAC isolation
	t.Run("E2E-64: Staff inspects departmental special requests queue & RBAC isolation (200 OK & 403 Forbidden)", func(t *testing.T) {
		// 1. Guest akses queue staf -> 403 Forbidden
		reqGuest, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/special-requests", nil)
		resGuest, _ := client.Do(reqGuest)
		if resGuest.StatusCode != http.StatusForbidden {
			t.Errorf("guest accessing staff queue status = %d, want 403 Forbidden", resGuest.StatusCode)
		}

		// 2. Housekeeping staf akses queue -> 200 OK
		reqHK, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/front-desk/special-requests?department=housekeeping", nil)
		reqHK.Header.Set("Authorization", "Bearer housekeeping")
		resHK, err := client.Do(reqHK)
		if err != nil || resHK.StatusCode != http.StatusOK {
			t.Fatalf("housekeeping queue status = %d, want 200 OK", resHK.StatusCode)
		}

		var queueResp struct {
			Items []assistance.StaffQueueItem `json:"items"`
			Total int                         `json:"total"`
		}
		_ = json.NewDecoder(resHK.Body).Decode(&queueResp)
		if queueResp.Total == 0 || len(queueResp.Items) == 0 {
			t.Fatalf("expected at least 1 item in housekeeping queue, got 0")
		}
		if queueResp.Items[0].ID != specialRequestID {
			t.Errorf("expected request ID %s in queue, got %s", specialRequestID, queueResp.Items[0].ID)
		}
	})

	// 65. Housekeeping fulfills guest special request & Guest sees updated status
	t.Run("E2E-65: Housekeeping fulfills special request & guest views fulfillment (200 OK)", func(t *testing.T) {
		targetBookingID := tx.booking.ID

		// 1. Housekeeping update status to fulfilled
		updatePayload := `{"to_status":"fulfilled","staff_notes":"Handuk angsa dan kartu ucapan selamat anniversary telah siap di kamar 201"}`
		reqUpdate, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/front-desk/special-requests/"+specialRequestID+"/status", strings.NewReader(updatePayload))
		reqUpdate.Header.Set("Authorization", "Bearer housekeeping")
		reqUpdate.Header.Set("Content-Type", "application/json")
		resUpdate, err := client.Do(reqUpdate)
		if err != nil || resUpdate.StatusCode != http.StatusOK {
			t.Fatalf("housekeeping fulfill status = %d, want 200 OK", resUpdate.StatusCode)
		}

		// 2. Tamu memeriksa status permohonan di My Bookings -> 200 OK status fulfilled
		reqGuestList, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/guest/bookings/"+targetBookingID+"/special-requests", nil)
		reqGuestList.Header.Set("Authorization", "Bearer "+assistGuestSessionToken)
		resGuestList, err := client.Do(reqGuestList)
		if err != nil || resGuestList.StatusCode != http.StatusOK {
			t.Fatalf("guest list requests status = %d, want 200 OK", resGuestList.StatusCode)
		}

		var guestListResp struct {
			BookingID string                      `json:"booking_id"`
			Requests  []assistance.SpecialRequest `json:"requests"`
		}
		_ = json.NewDecoder(resGuestList.Body).Decode(&guestListResp)
		if len(guestListResp.Requests) == 0 {
			t.Fatalf("expected at least 1 request in guest list, got 0")
		}
		reqItem := guestListResp.Requests[0]
		if reqItem.Status != assistance.StatusFulfilled {
			t.Errorf("expected request status fulfilled, got %s", reqItem.Status)
		}
		if !strings.Contains(reqItem.StaffNotes, "selamat anniversary") {
			t.Errorf("expected staff notes containing 'selamat anniversary', got %s", reqItem.StaffNotes)
		}
	})

	// 66. Transport Modernization: Gin Router Parity & Parameter Extraction
	t.Run("E2E-66: Gin Router Parity & Parameter Extraction", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("healthz status = %d, want 200", res.StatusCode)
		}
		if res.Header.Get("X-Request-Id") == "" {
			t.Errorf("expected X-Request-Id header to be injected by Gin middleware")
		}
	})

	// 67. Transport Modernization: JSON v2 Duplicate Key Rejection
	t.Run("E2E-67: JSON v2 Duplicate Key Rejection", func(t *testing.T) {
		dupKeyPayload := `{"room_type_id":"01900000-0000-7000-8000-000000000001","room_type_id":"01900000-0000-7000-8000-000000000002"}`
		res, err := client.Post(srv.URL+"/api/v1/quotes", "application/json", bytes.NewBufferString(dupKeyPayload))
		if err != nil {
			t.Fatalf("post quote failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("duplicate key status = %d, want 400 Bad Request", res.StatusCode)
		}
	})

	// 68. Transport Modernization: Go Validator v10 DTO Schema Checks & RFC 7807 Error Code
	t.Run("E2E-68: Go Validator v10 DTO Schema Checks", func(t *testing.T) {
		res, err := client.Get(srv.URL + "/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&rooms=10")
		if err != nil {
			t.Fatalf("search request failed: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("search rooms > 8 status = %d, want 400", res.StatusCode)
		}
		var prob map[string]any
		_ = json.NewDecoder(res.Body).Decode(&prob)
		if prob["code"] != "INVALID_ROOM_COUNT" {
			t.Errorf("expected code INVALID_ROOM_COUNT, got %v", prob["code"])
		}
	})
}

// e2eQuoteID membuat quote terkunci via API; create booking wajib quote_id (BE-R06).
func e2eQuoteID(t *testing.T, client *http.Client, base, checkIn, checkOut string) string {
	t.Helper()
	body := fmt.Sprintf(`{"room_type_id":"01900000-0000-7000-8000-000000000001","check_in":%q,"check_out":%q,"num_rooms":1,"num_guests":2}`, checkIn, checkOut)
	res, err := client.Post(base+"/api/v1/quotes", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("quote request failed: %v", err)
	}
	defer res.Body.Close()
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&q)
	if res.StatusCode != http.StatusOK || q.QuoteID == "" {
		t.Fatalf("setup quote failed: status %d", res.StatusCode)
	}
	return q.QuoteID
}
