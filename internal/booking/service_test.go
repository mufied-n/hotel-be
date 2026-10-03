package booking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
)

// ---------- fake InventoryTx (mirror logika SQL + GiST) ----------

type assignment struct {
	roomNumber string
	checkIn    time.Time
	checkOut   time.Time
}

type fakeTx struct {
	inventory    map[string]int            // "roomType|2006-01-02" → sisa
	bookings     map[string]*Booking       // bookingID → booking
	rooms        map[string][]string       // roomTypeID → daftar nomor kamar
	byBooking    map[string][]assignment   // bookingID → list assignment
	byRoom                 map[string][]assignment   // roomNumber → daftar assignment
	events                 []string                  // topic yang di-publish
	failLock               bool
	failInsert             bool
	insertErr              error
	transientConflictCount int
	roomNights             map[string][]rates.Quote  // bookingID → quotes
}

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func newFakeTx(bookings map[string]*Booking, rooms map[string][]string) *fakeTx {
	if bookings == nil {
		bookings = map[string]*Booking{}
	}
	return &fakeTx{
		inventory:  map[string]int{},
		bookings:   bookings,
		rooms:      rooms,
		byBooking:  map[string][]assignment{},
		byRoom:     map[string][]assignment{},
		roomNights: map[string][]rates.Quote{},
	}
}

func (f *fakeTx) InTx(_ context.Context, fn func(InventoryTx, EventPublisher) error) error { return fn(f, f) }

func (f *fakeTx) LockAndDecrement(_ context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	if f.failLock {
		return ErrInsufficient
	}
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		k := fmt.Sprintf("%s|%s", roomTypeID, d.Format("2006-01-02"))
		if f.inventory[k] < numRooms {
			return ErrInsufficient
		}
		f.inventory[k] -= numRooms
	}
	return nil
}

func (f *fakeTx) Increment(_ context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		k := fmt.Sprintf("%s|%s", roomTypeID, d.Format("2006-01-02"))
		f.inventory[k] += numRooms
	}
	return nil
}

func (f *fakeTx) InsertBookingWithHold(_ context.Context, b *Booking, quotes []rates.Quote, holdExpiresAt time.Time) error {
	if f.failInsert {
		return errors.New("insert failed")
	}
	if f.insertErr != nil {
		return f.insertErr
	}
	b.ID = "generated-" + b.GuestName
	b.ExpiresAt = &holdExpiresAt
	f.bookings[b.ID] = b
	f.roomNights[b.ID] = quotes
	return nil
}

func (f *fakeTx) UpdateStatus(_ context.Context, id string, to Status) error {
	b, ok := f.bookings[id]
	if !ok {
		return ErrNotFound
	}
	b.Status = to
	return nil
}

func (f *fakeTx) GetForUpdate(_ context.Context, id string) (Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return Booking{}, ErrNotFound
	}
	return *b, nil
}

func (f *fakeTx) PublishTx(_ context.Context, topic string, _ []byte) error {
	f.events = append(f.events, topic)
	return nil
}

func (f *fakeTx) PickAndAssignRooms(_ context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error) {
	if f.transientConflictCount > 0 {
		f.transientConflictCount--
		return nil, ErrTransientConflict
	}
	candidates := append([]string(nil), f.rooms[roomTypeID]...)
	sort.Strings(candidates)
	var assigned []string
	for _, room := range candidates {
		free := true
		for _, a := range f.byRoom[room] {
			// overlap bila a.checkIn < checkOut DAN checkIn < a.checkOut (half-open)
			if a.checkIn.Before(checkOut) && checkIn.Before(a.checkOut) {
				free = false
				break
			}
		}
		if free {
			a := assignment{roomNumber: room, checkIn: checkIn, checkOut: checkOut}
			f.byRoom[room] = append(f.byRoom[room], a)
			f.byBooking[bookingID] = append(f.byBooking[bookingID], a)
			assigned = append(assigned, room)
			if len(assigned) == count {
				return assigned, nil
			}
		}
	}
	return nil, ErrNoRoomAvailable
}

func (f *fakeTx) GetRoomAssignments(_ context.Context, bookingID string) ([]string, error) {
	assignments, ok := f.byBooking[bookingID]
	if !ok || len(assignments) == 0 {
		return nil, ErrNotFound
	}
	var rooms []string
	for _, a := range assignments {
		rooms = append(rooms, a.roomNumber)
	}
	return rooms, nil
}

type fakeReader struct{ bookings map[string]*Booking }

func (r *fakeReader) Get(_ context.Context, id string) (Booking, error) {
	b, ok := r.bookings[id]
	if !ok {
		return Booking{}, ErrNotFound
	}
	return *b, nil
}

type fakePayment struct {
	fail bool
}

func (p *fakePayment) CreateCharge(_ context.Context, _ Booking, amountMinor int64, _ string) (ChargeResult, error) {
	if p.fail {
		return ChargeResult{}, errors.New("gateway unavailable")
	}
	return ChargeResult{PaymentURL: "http://pay.fake", Reference: "ref-123"}, nil
}

type fakeNotifier struct{}

func (n *fakeNotifier) SendBookingConfirmed(_ context.Context, _ Booking) error {
	return nil
}

type mockAttemptStore struct {
	attempts []PaymentAttempt
}

func (m *mockAttemptStore) RecordAttempt(_ context.Context, _ PaymentAttempt) error { return nil }
func (m *mockAttemptStore) UpdateAttemptStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockAttemptStore) GetAttemptsByBookingID(_ context.Context, _ string) ([]PaymentAttempt, error) {
	return m.attempts, nil
}

type fakeInvStore struct {
	avail []inventory.Availability
	err   error
}

func (s *fakeInvStore) GetByDate(_ context.Context, _ string, _, _ time.Time) ([]inventory.Availability, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.avail, nil
}

type fakeRates struct {
	quotes []rates.Quote
	err    error
}

func (r *fakeRates) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.quotes, nil
}

// ---------- helper service ----------

func newTestService(tx *fakeTx, reader Reader) *Service {
	return NewService(tx, nil, nil, &fakePayment{}, &fakeNotifier{}, reader, 30*time.Minute, slog.Default())
}

// ---------- tests ----------

func TestCheckIn_HappyPathSingleRoom(t *testing.T) {
	b := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 1, Status: StatusConfirmed}
	tx := newFakeTx(map[string]*Booking{"b1": b}, map[string][]string{"std": {"102", "101"}})
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	res, err := svc.CheckIn(context.Background(), "b1")
	if err != nil {
		t.Fatalf("CheckIn error: %v", err)
	}
	if len(res.RoomNumbers) != 1 || res.RoomNumbers[0] != "101" {
		t.Errorf("rooms = %v, want [101]", res.RoomNumbers)
	}
	if b.Status != StatusCheckedIn {
		t.Errorf("status = %s, want checked_in", b.Status)
	}
	if len(tx.events) != 1 || tx.events[0] != "booking.checked_in" {
		t.Errorf("events = %v, want [booking.checked_in]", tx.events)
	}
}

func TestCheckIn_HappyPathMultiRoom(t *testing.T) {
	b := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 2, Status: StatusConfirmed}
	tx := newFakeTx(map[string]*Booking{"b1": b}, map[string][]string{"std": {"103", "101", "102"}})
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	res, err := svc.CheckIn(context.Background(), "b1")
	if err != nil {
		t.Fatalf("CheckIn error: %v", err)
	}
	if len(res.RoomNumbers) != 2 || res.RoomNumbers[0] != "101" || res.RoomNumbers[1] != "102" {
		t.Errorf("rooms = %v, want [101 102]", res.RoomNumbers)
	}
	if b.Status != StatusCheckedIn {
		t.Errorf("status = %s, want checked_in", b.Status)
	}
}

func TestCheckIn_Idempotent(t *testing.T) {
	b := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 1, Status: StatusConfirmed}
	tx := newFakeTx(map[string]*Booking{"b1": b}, map[string][]string{"std": {"101"}})
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	first, err := svc.CheckIn(context.Background(), "b1")
	if err != nil {
		t.Fatalf("first check-in error: %v", err)
	}
	second, err := svc.CheckIn(context.Background(), "b1")
	if err != nil {
		t.Fatalf("second check-in error (harus idempotent): %v", err)
	}
	if !second.Already || len(second.RoomNumbers) != len(first.RoomNumbers) || second.RoomNumbers[0] != first.RoomNumbers[0] {
		t.Errorf("second = %+v, want Already=true dan kamar sama (%v)", second, first.RoomNumbers)
	}
	if len(tx.events) != 1 {
		t.Errorf("event ter-publish = %d, want 1 (idempotent tidak publish ulang)", len(tx.events))
	}
}

func TestCheckIn_IllegalTransition(t *testing.T) {
	b := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 1, Status: StatusPending}
	tx := newFakeTx(map[string]*Booking{"b1": b}, map[string][]string{"std": {"101"}})
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	_, err := svc.CheckIn(context.Background(), "b1")
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("err = %v, want ErrIllegalTransition (pending belum boleh check-in)", err)
	}
	if b.Status != StatusPending {
		t.Errorf("status berubah menjadi %s, want pending (tidak boleh half-applied)", b.Status)
	}
}

func TestCheckIn_NoRoomAvailable(t *testing.T) {
	b := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 1, Status: StatusConfirmed}
	tx := newFakeTx(map[string]*Booking{"b1": b}, map[string][]string{"std": {"101"}})
	// Kamar 101 sudah dipakai tamu lain pada rentang yang tumpang tindih.
	tx.byRoom["101"] = []assignment{{roomNumber: "101", checkIn: date("2026-10-11"), checkOut: date("2026-10-13")}}
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	_, err := svc.CheckIn(context.Background(), "b1")
	if !errors.Is(err, ErrNoRoomAvailable) {
		t.Fatalf("err = %v, want ErrNoRoomAvailable", err)
	}
	if b.Status != StatusConfirmed {
		t.Errorf("status = %s, want confirmed (assignment gagal → rollback utuh)", b.Status)
	}
}

func TestCheckIn_NonOverlapReuseSameRoom(t *testing.T) {
	prev := &Booking{ID: "b0", RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"), NumRooms: 1, Status: StatusCheckedIn}
	next := &Booking{ID: "b1", RoomTypeID: "std", CheckIn: date("2026-10-12"), CheckOut: date("2026-10-14"), NumRooms: 1, Status: StatusConfirmed}
	tx := newFakeTx(
		map[string]*Booking{"b0": prev, "b1": next},
		map[string][]string{"std": {"101"}},
	)
	tx.byRoom["101"] = []assignment{{roomNumber: "101", checkIn: date("2026-10-10"), checkOut: date("2026-10-12")}}
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": next}})

	res, err := svc.CheckIn(context.Background(), "b1")
	if err != nil {
		t.Fatalf("CheckIn error: %v", err)
	}
	if len(res.RoomNumbers) != 1 || res.RoomNumbers[0] != "101" {
		t.Errorf("rooms = %v, want [101]", res.RoomNumbers)
	}
}

func TestCheckIn_TransientConflictRetry(t *testing.T) {
	tests := []struct {
		name              string
		conflictCount     int
		expectErr         error
		expectRoomNumbers []string
	}{
		{
			name:              "Single transient conflict recovers on second attempt",
			conflictCount:     1,
			expectErr:         nil,
			expectRoomNumbers: []string{"101"},
		},
		{
			name:              "Two transient conflicts recover on third attempt",
			conflictCount:     2,
			expectErr:         nil,
			expectRoomNumbers: []string{"101"},
		},
		{
			name:              "Three transient conflicts exhausts 3 attempts and returns ErrNoRoomAvailable",
			conflictCount:     3,
			expectErr:         ErrNoRoomAvailable,
			expectRoomNumbers: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Booking{
				ID:         "b-test-retry",
				RoomTypeID: "std",
				CheckIn:    date("2026-10-10"),
				CheckOut:   date("2026-10-12"),
				NumRooms:   1,
				Status:     StatusConfirmed,
			}
			tx := newFakeTx(
				map[string]*Booking{"b-test-retry": b},
				map[string][]string{"std": {"101"}},
			)
			tx.transientConflictCount = tc.conflictCount
			svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-test-retry": b}})

			res, err := svc.CheckIn(context.Background(), "b-test-retry")
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(res.RoomNumbers) != len(tc.expectRoomNumbers) || res.RoomNumbers[0] != tc.expectRoomNumbers[0] {
					t.Fatalf("expected rooms %v, got %v", tc.expectRoomNumbers, res.RoomNumbers)
				}
			}
		})
	}
}

func TestCreate_HappyPathAndReservationRoomNights(t *testing.T) {
	tx := newFakeTx(map[string]*Booking{}, map[string][]string{})
	tx.inventory["std|2026-10-10"] = 5
	tx.inventory["std|2026-10-11"] = 5
	inv := &fakeInvStore{
		avail: []inventory.Availability{
			{Date: date("2026-10-10"), TotalRooms: 5, AvailableRooms: 5},
			{Date: date("2026-10-11"), TotalRooms: 5, AvailableRooms: 5},
		},
	}
	ratesSvc := &fakeRates{
		quotes: []rates.Quote{
			{Date: date("2026-10-10"), RateMinor: 500_000},
			{Date: date("2026-10-11"), RateMinor: 500_000},
		},
	}
	pay := &fakePayment{}
	reader := &fakeReader{bookings: tx.bookings}
	svc := NewService(tx, inv, ratesSvc, pay, &fakeNotifier{}, reader, 30*time.Minute, slog.Default())

	in := CreateInput{
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-12"),
		NumRooms:   1,
		NumGuests:  2,
		GuestName:  "Budi",
		GuestEmail: "budi@example.com",
	}

	b, charge, err := svc.Create(context.Background(), withQuote(svc, in))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if b.Status != StatusPending {
		t.Errorf("status = %s, want pending", b.Status)
	}
	if b.TotalPriceMinor != 1_000_000 {
		t.Errorf("total_price = %d, want 1_000_000", b.TotalPriceMinor)
	}
	if charge.PaymentURL == "" || charge.Reference == "" {
		t.Errorf("charge = %+v, want non-empty url and ref", charge)
	}
	// Verify reservation_room_nights were recorded
	quotesSaved := tx.roomNights[b.ID]
	if len(quotesSaved) != 2 {
		t.Errorf("saved quotes count = %d, want 2", len(quotesSaved))
	}
}

func TestCreate_CanonicalEmailAndTrimmedName(t *testing.T) {
	tx := newFakeTx(nil, nil)
	tx.inventory["std|2026-10-10"] = 5
	tx.inventory["std|2026-10-11"] = 5
	inv := &fakeInvStore{
		avail: []inventory.Availability{
			{Date: date("2026-10-10"), TotalRooms: 5, AvailableRooms: 5},
			{Date: date("2026-10-11"), TotalRooms: 5, AvailableRooms: 5},
		},
	}
	ratesSvc := &fakeRates{
		quotes: []rates.Quote{
			{Date: date("2026-10-10"), RateMinor: 500_000},
			{Date: date("2026-10-11"), RateMinor: 500_000},
		},
	}
	pay := &fakePayment{}
	reader := &fakeReader{bookings: tx.bookings}
	svc := NewService(tx, inv, ratesSvc, pay, &fakeNotifier{}, reader, 30*time.Minute, slog.Default())

	in := CreateInput{
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-12"),
		NumRooms:   1,
		NumGuests:  2,
		GuestName:  "  Budi Santoso  ",
		GuestEmail: "  Budi.Santoso@EXAMPLE.COM  ",
	}

	b, _, err := svc.Create(context.Background(), withQuote(svc, in))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if b.GuestEmail != "budi.santoso@example.com" {
		t.Errorf("expected canonical lowercase email 'budi.santoso@example.com', got '%s'", b.GuestEmail)
	}
	if b.GuestName != "Budi Santoso" {
		t.Errorf("expected trimmed name 'Budi Santoso', got '%s'", b.GuestName)
	}
}

func TestCreate_InvalidInput(t *testing.T) {
	svc := newTestService(newFakeTx(nil, nil), &fakeReader{})
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name: "checkout before checkin",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 5),
				CheckOut:   today.AddDate(0, 0, 3),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidDateRange,
		},
		{
			name: "checkout same as checkin",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 5),
				CheckOut:   today.AddDate(0, 0, 5),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidDateRange,
		},
		{
			name: "exceeds max stay 30 nights",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 32),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrExceedsMaxStay,
		},
		{
			name: "zero rooms",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   0,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidCapacity,
		},
		{
			name: "more than 8 rooms",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   9,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidCapacity,
		},
		{
			name: "zero guests",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   1,
				NumGuests:  0,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidCapacity,
		},
		{
			name: "past checkin date",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, -3),
				CheckOut:   today.AddDate(0, 0, 1),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrPastDate,
		},
		{
			name: "exceeds 365 day horizon",
			input: CreateInput{
				CheckIn:    today.AddDate(1, 0, 10),
				CheckOut:   today.AddDate(1, 0, 15),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrExceedsHorizon,
		},
		{
			name: "empty guest name",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "   ",
				GuestEmail: "budi@example.com",
			},
			wantErr: ErrInvalidGuestInfo,
		},
		{
			name: "empty guest email",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "   ",
			},
			wantErr: ErrInvalidGuestInfo,
		},
		{
			name: "invalid guest email without at",
			input: CreateInput{
				CheckIn:    today.AddDate(0, 0, 1),
				CheckOut:   today.AddDate(0, 0, 2),
				NumRooms:   1,
				NumGuests:  1,
				GuestName:  "Budi",
				GuestEmail: "invalid-email",
			},
			wantErr: ErrInvalidGuestInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.Create(context.Background(), tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("%s: svc.Create() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestCreate_PaymentFailureCompensation(t *testing.T) {
	tx := newFakeTx(map[string]*Booking{}, map[string][]string{})
	tx.inventory["std|2026-10-10"] = 2
	inv := &fakeInvStore{
		avail: []inventory.Availability{{Date: date("2026-10-10"), TotalRooms: 2, AvailableRooms: 2}},
	}
	ratesSvc := &fakeRates{
		quotes: []rates.Quote{{Date: date("2026-10-10"), RateMinor: 100_000}},
	}
	pay := &fakePayment{fail: true}
	reader := &fakeReader{bookings: tx.bookings}
	svc := NewService(tx, inv, ratesSvc, pay, &fakeNotifier{}, reader, 30*time.Minute, slog.Default())

	_, _, err := svc.Create(context.Background(), withQuote(svc, CreateInput{
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-11"),
		NumRooms:   1,
		NumGuests:  1,
		GuestName:  "Test",
		GuestEmail: "test@example.com",
	}))
	if err == nil {
		t.Fatal("Create want error when payment fails")
	}
	// Verify compensation: booking should be cancelled
	b := tx.bookings["generated-Test"]
	if b == nil {
		t.Fatal("expected booking to exist before cancellation")
	}
	if b.Status != StatusCancelled {
		t.Errorf("compensated status = %s, want cancelled", b.Status)
	}
}

func TestCheckOut_HappyPath(t *testing.T) {
	b := &Booking{ID: "b1", Status: StatusCheckedIn}
	tx := newFakeTx(map[string]*Booking{"b1": b}, nil)
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	if err := svc.CheckOut(context.Background(), "b1"); err != nil {
		t.Fatalf("CheckOut err = %v", err)
	}
	if b.Status != StatusCheckedOut {
		t.Errorf("status = %s, want checked_out", b.Status)
	}
}

func TestMarkNoShow_ReleasesInventory(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	b := &Booking{
		ID:         "b1",
		RoomTypeID: "std",
		CheckIn:    today,
		CheckOut:   today.Add(24 * time.Hour),
		NumRooms:   1,
		Status:     StatusConfirmed,
	}
	tx := newFakeTx(map[string]*Booking{"b1": b}, nil)
	dateKey := "std|" + today.Format("2006-01-02")
	tx.inventory[dateKey] = 0
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	if err := svc.MarkNoShow(context.Background(), "b1"); err != nil {
		t.Fatalf("MarkNoShow err = %v", err)
	}
	if b.Status != StatusNoShow {
		t.Errorf("status = %s, want no_show", b.Status)
	}
	if tx.inventory[dateKey] != 1 {
		t.Errorf("inventory = %d, want 1 (restored)", tx.inventory[dateKey])
	}
}

func TestConfirm_Idempotent(t *testing.T) {
	b := &Booking{ID: "b1", Status: StatusPending}
	tx := newFakeTx(map[string]*Booking{"b1": b}, nil)
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	if err := svc.Confirm(context.Background(), "b1"); err != nil {
		t.Fatalf("Confirm err = %v", err)
	}
	if b.Status != StatusConfirmed {
		t.Errorf("status = %s, want confirmed", b.Status)
	}

	// Duplikat webhook
	if err := svc.Confirm(context.Background(), "b1"); err != nil {
		t.Fatalf("duplicate Confirm err = %v", err)
	}
}

func TestCancel_ReleasesInventory(t *testing.T) {
	b := &Booking{
		ID:         "b1",
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-11"),
		NumRooms:   1,
		Status:     StatusConfirmed,
	}
	tx := newFakeTx(map[string]*Booking{"b1": b}, nil)
	tx.inventory["std|2026-10-10"] = 4
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	if err := svc.Cancel(context.Background(), "b1"); err != nil {
		t.Fatalf("Cancel err = %v", err)
	}
	if b.Status != StatusCancelled {
		t.Errorf("status = %s, want cancelled", b.Status)
	}
	if tx.inventory["std|2026-10-10"] != 5 {
		t.Errorf("inventory = %d, want 5 (restored)", tx.inventory["std|2026-10-10"])
	}
}

func TestService_GetAndGuestTokenAndHoldTimeout(t *testing.T) {
	b := &Booking{
		ID:         "b-token-test",
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-11"),
		GuestName:  "Token Tester",
		GuestToken: "gst_test1234567890",
		Status:     StatusConfirmed,
	}
	tx := newFakeTx(map[string]*Booking{"b-token-test": b}, nil)
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-token-test": b}})

	if svc.HoldTimeout() != 30*time.Minute {
		t.Errorf("HoldTimeout() = %v, want 30m", svc.HoldTimeout())
	}

	got, err := svc.Get(context.Background(), "b-token-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GuestToken != "gst_test1234567890" {
		t.Errorf("GuestToken = %s, want gst_test1234567890", got.GuestToken)
	}

	_, errNotFound := svc.Get(context.Background(), "non-existent")
	if !errors.Is(errNotFound, ErrNotFound) {
		t.Errorf("Get() non-existent error = %v, want ErrNotFound", errNotFound)
	}
}

func TestBatchC_QuoteLockingAndPolicies(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("Create with valid quote locks price and breakdown", func(t *testing.T) {
		qs := rates.NewMemoryQuoteStore(15 * time.Minute)
		q := rates.LockedQuote{
			ID:               "quote-valid-123",
			CreatedAt:        now,
			ExpiresAt:        now.Add(15 * time.Minute),
			RoomTypeID:       "std",
			RatePlanCode:     rates.RatePlanBedBreakfast,
			RatePlanName:     "Bed & Breakfast",
			CancellationCode: rates.PolicyFlexible48h,
			CancellationDesc: "Flexible cancellation",
			CheckIn:          date("2026-10-10"),
			CheckOut:         date("2026-10-12"),
			NumRooms:         1,
			NumGuests:        2,
			Pricing: rates.PricingBreakdown{
				RoomSubtotalMinor:    1_000_000,
				BreakfastChargeMinor: 400_000,
				DiscountMinor:        0,
				TaxMinor:             140_000,
				TotalPriceMinor:      1_540_000,
				Currency:             "IDR",
			},
		}
		_ = qs.SaveQuote(ctx, q)

		inv := &fakeInvStore{
			avail: []inventory.Availability{
				{Date: date("2026-10-10"), TotalRooms: 10, AvailableRooms: 5},
				{Date: date("2026-10-11"), TotalRooms: 10, AvailableRooms: 5},
			},
		}
		ratesSvc := &fakeRates{
			quotes: []rates.Quote{
				{Date: date("2026-10-10"), RateMinor: 500_000},
				{Date: date("2026-10-11"), RateMinor: 500_000},
			},
		}
		tx := newFakeTx(map[string]*Booking{}, map[string][]string{})
		tx.inventory["std|2026-10-10"] = 5
		tx.inventory["std|2026-10-11"] = 5
		svc := NewService(tx, inv, ratesSvc, &fakePayment{}, &fakeNotifier{}, &fakeReader{}, 30*time.Minute, slog.Default())
		svc.SetQuoteStore(qs)

		b, _, err := svc.Create(ctx, CreateInput{
			QuoteID:         "quote-valid-123",
			RoomTypeID:      "std",
			CheckIn:         date("2026-10-10"),
			CheckOut:        date("2026-10-12"),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Budi Santoso",
			GuestEmail:      "budi@example.com",
			TermsAccepted:   true,
			PrivacyAccepted: true,
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if b.QuoteID != "quote-valid-123" {
			t.Errorf("b.QuoteID = %s, want quote-valid-123", b.QuoteID)
		}
		if b.RatePlanCode != rates.RatePlanBedBreakfast {
			t.Errorf("b.RatePlanCode = %s, want bed_and_breakfast", b.RatePlanCode)
		}
		if b.TotalPriceMinor != 1_540_000 {
			t.Errorf("b.TotalPriceMinor = %d, want 1_540_000", b.TotalPriceMinor)
		}
		if !b.TermsAccepted || b.TermsAcceptedAt == nil {
			t.Error("expected TermsAccepted to be recorded with timestamp")
		}
	})

	t.Run("Create fails when consent not provided", func(t *testing.T) {
		qs := rates.NewMemoryQuoteStore(15 * time.Minute)
		q := rates.LockedQuote{
			ID:        "quote-consent-test",
			ExpiresAt: now.Add(15 * time.Minute),
		}
		_ = qs.SaveQuote(ctx, q)
		svc := newTestService(newFakeTx(nil, nil), &fakeReader{})
		svc.SetQuoteStore(qs)

		_, _, err := svc.Create(ctx, CreateInput{
			QuoteID:         "quote-consent-test",
			RoomTypeID:      "std",
			CheckIn:         date("2026-10-10"),
			CheckOut:        date("2026-10-12"),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Budi",
			GuestEmail:      "budi@example.com",
			TermsAccepted:   false,
			PrivacyAccepted: true,
		})
		if !errors.Is(err, ErrConsentRequired) {
			t.Errorf("expected ErrConsentRequired, got %v", err)
		}
	})

	t.Run("Create fails when quote is expired", func(t *testing.T) {
		qs := rates.NewMemoryQuoteStore(10 * time.Millisecond)
		q := rates.LockedQuote{
			ID:        "quote-expired-test",
			ExpiresAt: now.Add(-1 * time.Minute), // expired in past
		}
		_ = qs.SaveQuote(ctx, q)
		svc := newTestService(newFakeTx(nil, nil), &fakeReader{})
		svc.SetQuoteStore(qs)

		_, _, err := svc.Create(ctx, CreateInput{
			QuoteID:         "quote-expired-test",
			RoomTypeID:      "std",
			CheckIn:         date("2026-10-10"),
			CheckOut:        date("2026-10-12"),
			NumRooms:        1,
			NumGuests:       2,
			GuestName:       "Budi",
			GuestEmail:      "budi@example.com",
			TermsAccepted:   true,
			PrivacyAccepted: true,
		})
		if !errors.Is(err, ErrQuoteExpired) {
			t.Errorf("expected ErrQuoteExpired, got %v", err)
		}
	})

	t.Run("Create fails when parameters mismatch quote", func(t *testing.T) {
		qs := rates.NewMemoryQuoteStore(15 * time.Minute)
		q := rates.LockedQuote{
			ID:         "quote-mismatch-test",
			ExpiresAt:  now.Add(15 * time.Minute),
			RoomTypeID: "std",
			CheckIn:    date("2026-10-10"),
			CheckOut:   date("2026-10-12"),
			NumRooms:   1,
			NumGuests:  2,
		}
		_ = qs.SaveQuote(ctx, q)
		svc := newTestService(newFakeTx(nil, nil), &fakeReader{})
		svc.SetQuoteStore(qs)

		_, _, err := svc.Create(ctx, CreateInput{
			QuoteID:         "quote-mismatch-test",
			RoomTypeID:      "std",
			CheckIn:         date("2026-10-10"),
			CheckOut:        date("2026-10-12"),
			NumRooms:        2, // Quote has 1 room!
			NumGuests:       2,
			GuestName:       "Budi",
			GuestEmail:      "budi@example.com",
			TermsAccepted:   true,
			PrivacyAccepted: true,
		})
		if !errors.Is(err, ErrQuoteMismatch) {
			t.Errorf("expected ErrQuoteMismatch, got %v", err)
		}
	})

	t.Run("Cancel confirmed booking with non_refundable policy is rejected", func(t *testing.T) {
		b := &Booking{
			ID:                 "b-non-ref",
			RoomTypeID:         "std",
			CheckIn:            now.Add(72 * time.Hour),
			CheckOut:           now.Add(96 * time.Hour),
			NumRooms:           1,
			Status:             StatusConfirmed,
			CancellationPolicy: rates.PolicyNonRefundable,
		}
		tx := newFakeTx(map[string]*Booking{"b-non-ref": b}, nil)
		svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-non-ref": b}})

		err := svc.Cancel(ctx, "b-non-ref")
		if !errors.Is(err, ErrNonRefundable) {
			t.Errorf("expected ErrNonRefundable, got %v", err)
		}
	})

	t.Run("Cancel confirmed booking with flexible_48h policy past deadline is rejected", func(t *testing.T) {
		// Check-in is tomorrow (less than 48 hours away)
		b := &Booking{
			ID:                 "b-flex-past-deadline",
			RoomTypeID:         "std",
			CheckIn:            now.Add(24 * time.Hour),
			CheckOut:           now.Add(48 * time.Hour),
			NumRooms:           1,
			Status:             StatusConfirmed,
			CancellationPolicy: rates.PolicyFlexible48h,
		}
		tx := newFakeTx(map[string]*Booking{"b-flex-past-deadline": b}, nil)
		svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-flex-past-deadline": b}})

		err := svc.Cancel(ctx, "b-flex-past-deadline")
		if !errors.Is(err, ErrCancellationDeadlineExceeded) {
			t.Errorf("expected ErrCancellationDeadlineExceeded, got %v", err)
		}
	})

	t.Run("Cancel pending booking with non_refundable policy is allowed (hold release)", func(t *testing.T) {
		b := &Booking{
			ID:                 "b-pending-non-ref",
			RoomTypeID:         "std",
			CheckIn:            now.Add(72 * time.Hour),
			CheckOut:           now.Add(96 * time.Hour),
			NumRooms:           1,
			Status:             StatusPending,
			CancellationPolicy: rates.PolicyNonRefundable,
		}
		tx := newFakeTx(map[string]*Booking{"b-pending-non-ref": b}, nil)
		svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-pending-non-ref": b}})

		err := svc.Cancel(ctx, "b-pending-non-ref")
		if err != nil {
			t.Fatalf("expected pending hold to be cancellable, got error = %v", err)
		}
		if b.Status != StatusCancelled {
			t.Errorf("status = %s, want cancelled", b.Status)
		}
	})

	t.Run("Cancel already cancelled booking is idempotent", func(t *testing.T) {
		b := &Booking{
			ID:     "b-already-cancelled",
			Status: StatusCancelled,
		}
		tx := newFakeTx(map[string]*Booking{"b-already-cancelled": b}, nil)
		svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-already-cancelled": b}})
		if err := svc.Cancel(ctx, "b-already-cancelled"); err != nil {
			t.Errorf("expected idempotent nil, got %v", err)
		}
	})

	t.Run("Cancel checked_in booking returns ErrIllegalTransition", func(t *testing.T) {
		b := &Booking{
			ID:     "b-checked-in-cannot-cancel",
			Status: StatusCheckedIn,
		}
		tx := newFakeTx(map[string]*Booking{"b-checked-in-cannot-cancel": b}, nil)
		svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-checked-in-cannot-cancel": b}})
		if err := svc.Cancel(ctx, "b-checked-in-cannot-cancel"); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("PaymentAttemptStore getter and setter", func(t *testing.T) {
		tx := newFakeTx(nil, nil)
		svc := newTestService(tx, &fakeReader{})
		// without store
		attempts, err := svc.GetPaymentAttempts(ctx, "bk-none")
		if err != nil || attempts != nil {
			t.Errorf("expected nil without store, got %v, %v", attempts, err)
		}
		// with store
		mockStore := &mockAttemptStore{attempts: []PaymentAttempt{{ID: "att-1", BookingID: "bk-1"}}}
		svc.SetPaymentAttemptStore(mockStore)
		res, err := svc.GetPaymentAttempts(ctx, "bk-1")
		if err != nil || len(res) != 1 {
			t.Errorf("expected 1 attempt, got %v, %v", res, err)
		}
	})

	t.Run("FreeCancellationDeadline helper calculation", func(t *testing.T) {
		checkIn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

		// flexible_48h: H-2 14:00 WIB (07:00 UTC)
		deadline, ok := FreeCancellationDeadline(checkIn, rates.PolicyFlexible48h)
		if !ok {
			t.Fatalf("expected ok=true for flexible_48h")
		}
		expectedUTC := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
		if !deadline.Equal(expectedUTC) {
			t.Errorf("deadline in UTC = %v, want %v", deadline.UTC(), expectedUTC)
		}
		// Pastikan deadline ber-zona WIB
		if deadline.Location().String() != "WIB" {
			t.Errorf("deadline location = %s, want WIB", deadline.Location().String())
		}
		if deadline.Hour() != 14 || deadline.Day() != 8 {
			t.Errorf("deadline local = %02d:%02d on day %d, want 14:00 on day 8", deadline.Hour(), deadline.Minute(), deadline.Day())
		}

		// non_refundable: no free cancellation
		_, okNR := FreeCancellationDeadline(checkIn, rates.PolicyNonRefundable)
		if okNR {
			t.Errorf("expected ok=false for non_refundable")
		}
	})

	t.Run("Clock-controlled cancellation deadline boundary table test (BE-R12)", func(t *testing.T) {
		checkIn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
		deadlineWIB := time.Date(2026, 10, 8, 14, 0, 0, 0, LocationWIB)

		tests := []struct {
			name        string
			simulatedAt time.Time
			policy      string
			wantErr     error
		}{
			{
				name:        "1 hour before deadline WIB (13:00 WIB / 06:00 UTC)",
				simulatedAt: deadlineWIB.Add(-1 * time.Hour),
				policy:      rates.PolicyFlexible48h,
				wantErr:     nil,
			},
			{
				name:        "1 second before deadline WIB (13:59:59 WIB / 06:59:59 UTC)",
				simulatedAt: deadlineWIB.Add(-1 * time.Second),
				policy:      rates.PolicyFlexible48h,
				wantErr:     nil,
			},
			{
				name:        "Exactly at deadline WIB (14:00:00 WIB / 07:00:00 UTC)",
				simulatedAt: deadlineWIB,
				policy:      rates.PolicyFlexible48h,
				wantErr:     nil, // Not strictly after deadline
			},
			{
				name:        "1 second after deadline WIB (14:00:01 WIB / 07:00:01 UTC) - REJECTED",
				simulatedAt: deadlineWIB.Add(1 * time.Second),
				policy:      rates.PolicyFlexible48h,
				wantErr:     ErrCancellationDeadlineExceeded,
			},
			{
				name:        "1 hour after deadline WIB (15:00 WIB / 08:00 UTC) - REJECTED",
				simulatedAt: deadlineWIB.Add(1 * time.Hour),
				policy:      rates.PolicyFlexible48h,
				wantErr:     ErrCancellationDeadlineExceeded,
			},
			{
				name:        "The old UTC bug window: 14:00 UTC / 21:00 WIB (+7 hours late) - REJECTED",
				simulatedAt: time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC),
				policy:      rates.PolicyFlexible48h,
				wantErr:     ErrCancellationDeadlineExceeded,
			},
			{
				name:        "Day before check-in (9 Oct 12:00 WIB) - REJECTED",
				simulatedAt: time.Date(2026, 10, 9, 12, 0, 0, 0, LocationWIB),
				policy:      rates.PolicyFlexible48h,
				wantErr:     ErrCancellationDeadlineExceeded,
			},
			{
				name:        "Day of check-in (10 Oct 10:00 WIB) - REJECTED",
				simulatedAt: time.Date(2026, 10, 10, 10, 0, 0, 0, LocationWIB),
				policy:      rates.PolicyFlexible48h,
				wantErr:     ErrCancellationDeadlineExceeded,
			},
			{
				name:        "Non-refundable policy rejected even well in advance",
				simulatedAt: deadlineWIB.Add(-100 * time.Hour),
				policy:      rates.PolicyNonRefundable,
				wantErr:     ErrNonRefundable,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				bookingID := "b-clk-" + tc.name
				b := &Booking{
					ID:                 bookingID,
					RoomTypeID:         "std",
					CheckIn:            checkIn,
					CheckOut:           checkIn.Add(48 * time.Hour),
					NumRooms:           1,
					Status:             StatusConfirmed,
					CancellationPolicy: tc.policy,
				}
				tx := newFakeTx(map[string]*Booking{bookingID: b}, nil)
				svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{bookingID: b}})
				svc.SetNowFunc(func() time.Time {
					return tc.simulatedAt
				})

				err := svc.Cancel(ctx, bookingID)
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Cancel() error = %v, wantErr = %v", err, tc.wantErr)
				}
				if tc.wantErr == nil && b.Status != StatusCancelled {
					t.Errorf("booking status = %s, want %s", b.Status, StatusCancelled)
				}
			})
		}
	})
}

func TestBatchD_GuestProfileAndHoldExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)

	t.Run("GuestPhone E.164 table test", func(t *testing.T) {
		tests := []struct {
			name    string
			phone   string
			wantErr error
		}{
			{name: "empty phone is optional", phone: "", wantErr: nil},
			{name: "valid Indonesian E.164", phone: "+6281234567890", wantErr: nil},
			{name: "valid US E.164", phone: "+12025550123", wantErr: nil},
			{name: "invalid domestic 08 format", phone: "081234567890", wantErr: ErrInvalidPhone},
			{name: "invalid leading zero country code", phone: "+012345678", wantErr: ErrInvalidPhone},
			{name: "invalid letters", phone: "+62abc1234", wantErr: ErrInvalidPhone},
			{name: "invalid spaces and dashes", phone: "+62 812-345-678", wantErr: ErrInvalidPhone},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tx := newFakeTx(nil, nil)
				tx.inventory["std|"+now.Format("2006-01-02")] = 5
				svc := newTestService(tx, &fakeReader{})
				in := CreateInput{
					RoomTypeID: "std",
					CheckIn:    now,
					CheckOut:   now.Add(24 * time.Hour),
					NumRooms:   1,
					NumGuests:  1,
					GuestName:  "Budi Santoso",
					GuestEmail: "budi@example.com",
					GuestPhone: tc.phone,
				}
				_, _, err := svc.Create(ctx, withQuote(svc, in))
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Errorf("expected error %v, got %v", tc.wantErr, err)
					}
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("EstimatedArrivalTime HH:MM table test", func(t *testing.T) {
		tests := []struct {
			name    string
			arrival string
			wantErr error
		}{
			{name: "empty arrival is optional", arrival: "", wantErr: nil},
			{name: "valid standard checkin 14:00", arrival: "14:00", wantErr: nil},
			{name: "valid midnight 00:00", arrival: "00:00", wantErr: nil},
			{name: "valid late night 23:59", arrival: "23:59", wantErr: nil},
			{name: "invalid 24:00", arrival: "24:00", wantErr: ErrInvalidArrivalTime},
			{name: "invalid minute 14:60", arrival: "14:60", wantErr: ErrInvalidArrivalTime},
			{name: "invalid single digit hour 9:00", arrival: "9:00", wantErr: ErrInvalidArrivalTime},
			{name: "invalid 12h format 2:00pm", arrival: "2:00pm", wantErr: ErrInvalidArrivalTime},
			{name: "invalid no colon 1400", arrival: "1400", wantErr: ErrInvalidArrivalTime},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tx := newFakeTx(nil, nil)
				tx.inventory["std|"+now.Format("2006-01-02")] = 5
				svc := newTestService(tx, &fakeReader{})
				in := CreateInput{
					RoomTypeID:           "std",
					CheckIn:              now,
					CheckOut:             now.Add(24 * time.Hour),
					NumRooms:             1,
					NumGuests:            1,
					GuestName:            "Budi Santoso",
					GuestEmail:           "budi@example.com",
					EstimatedArrivalTime: tc.arrival,
				}
				_, _, err := svc.Create(ctx, withQuote(svc, in))
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Errorf("expected error %v, got %v", tc.wantErr, err)
					}
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("SpecialRequests 500-char limit table test", func(t *testing.T) {
		tests := []struct {
			name     string
			requests string
			wantErr  error
		}{
			{name: "empty requests allowed", requests: "", wantErr: nil},
			{name: "under 500 chars allowed", requests: "High floor, quiet room please.", wantErr: nil},
			{name: "exactly 500 chars allowed", requests: strings.Repeat("A", 500), wantErr: nil},
			{name: "501 chars rejected", requests: strings.Repeat("A", 501), wantErr: ErrSpecialRequestTooLong},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tx := newFakeTx(nil, nil)
				tx.inventory["std|"+now.Format("2006-01-02")] = 5
				svc := newTestService(tx, &fakeReader{})
				in := CreateInput{
					RoomTypeID:      "std",
					CheckIn:         now,
					CheckOut:        now.Add(24 * time.Hour),
					NumRooms:        1,
					NumGuests:       1,
					GuestName:       "Budi Santoso",
					GuestEmail:      "budi@example.com",
					SpecialRequests: tc.requests,
				}
				_, _, err := svc.Create(ctx, withQuote(svc, in))
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Errorf("expected error %v, got %v", tc.wantErr, err)
					}
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("Hold Expiry on Confirm table test", func(t *testing.T) {
		pastExpiry := time.Now().Add(-5 * time.Minute)
		futureExpiry := time.Now().Add(15 * time.Minute)

		tests := []struct {
			name      string
			status    Status
			expiresAt *time.Time
			wantErr   error
			wantFinal Status
		}{
			{
				name:      "pending hold expired should be rejected with ErrHoldExpired",
				status:    StatusPending,
				expiresAt: &pastExpiry,
				wantErr:   ErrHoldExpired,
				wantFinal: StatusPending,
			},
			{
				name:      "pending hold active should transition to confirmed",
				status:    StatusPending,
				expiresAt: &futureExpiry,
				wantErr:   nil,
				wantFinal: StatusConfirmed,
			},
			{
				name:      "already confirmed booking returns nil idempotently even if expiresAt past",
				status:    StatusConfirmed,
				expiresAt: &pastExpiry,
				wantErr:   nil,
				wantFinal: StatusConfirmed,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := &Booking{
					ID:        "b-hold-test",
					Status:    tc.status,
					ExpiresAt: tc.expiresAt,
				}
				tx := newFakeTx(map[string]*Booking{"b-hold-test": b}, nil)
				svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-hold-test": b}})

				err := svc.Confirm(ctx, "b-hold-test")
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Errorf("expected error %v, got %v", tc.wantErr, err)
					}
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if b.Status != tc.wantFinal {
					t.Errorf("expected final status %v, got %v", tc.wantFinal, b.Status)
				}
			})
		}
	})
}

func TestBatchE_OperationalReliabilityAndConcurrency(t *testing.T) {
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)

	t.Run("Early check-out inventory restitution table test", func(t *testing.T) {
		tests := []struct {
			name           string
			checkIn        time.Time
			checkOut       time.Time
			initialStatus  Status
			expectEarly    bool
			expectRestored int
		}{
			{
				name:           "early checkout restores future nights",
				checkIn:        today.Add(-24 * time.Hour), // Check-in kemarin
				checkOut:       today.Add(48 * time.Hour),  // Rencana checkout lusa (masih ada 2 malam ke depan)
				initialStatus:  StatusCheckedIn,
				expectEarly:    true,
				expectRestored: 1, // Untuk malam ini dan besok
			},
			{
				name:           "normal checkout on schedule date does not restitute extra",
				checkIn:        today.Add(-48 * time.Hour),
				checkOut:       today, // Checkout tepat hari ini
				initialStatus:  StatusCheckedIn,
				expectEarly:    false,
				expectRestored: 0,
			},
			{
				name:           "already checked out is idempotent",
				checkIn:        today.Add(-48 * time.Hour),
				checkOut:       today.Add(-24 * time.Hour),
				initialStatus:  StatusCheckedOut,
				expectEarly:    false,
				expectRestored: 0,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := &Booking{
					ID:         "b-early-co",
					RoomTypeID: "std",
					CheckIn:    tc.checkIn,
					CheckOut:   tc.checkOut,
					NumRooms:   1,
					Status:     tc.initialStatus,
				}
				tx := newFakeTx(map[string]*Booking{"b-early-co": b}, nil)
				futureKey := "std|" + today.Format("2006-01-02")
				tx.inventory[futureKey] = 0

				svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-early-co": b}})
				err := svc.CheckOut(ctx, "b-early-co")
				if err != nil {
					t.Fatalf("unexpected CheckOut error: %v", err)
				}
				if b.Status != StatusCheckedOut {
					t.Errorf("status = %v, want checked_out", b.Status)
				}
				if tc.expectEarly {
					if tx.inventory[futureKey] != tc.expectRestored {
						t.Errorf("inventory for %s = %d, want %d", futureKey, tx.inventory[futureKey], tc.expectRestored)
					}
				}
			})
		}
	})

	t.Run("No-show cutoff table test", func(t *testing.T) {
		tests := []struct {
			name        string
			checkIn     time.Time
			checkOut    time.Time
			status      Status
			wantErr     error
			wantFinal   Status
			wantRestore bool
		}{
			{
				name:        "cannot mark no-show for future check-in date",
				checkIn:     today.Add(24 * time.Hour), // Besok
				checkOut:    today.Add(48 * time.Hour),
				status:      StatusConfirmed,
				wantErr:     ErrNoShowTooEarly,
				wantFinal:   StatusConfirmed,
				wantRestore: false,
			},
			{
				name:        "can mark no-show on check-in date",
				checkIn:     today, // Hari ini
				checkOut:    today.Add(24 * time.Hour),
				status:      StatusConfirmed,
				wantErr:     nil,
				wantFinal:   StatusNoShow,
				wantRestore: true,
			},
			{
				name:        "can mark no-show for past check-in date",
				checkIn:     today.Add(-24 * time.Hour), // Kemarin
				checkOut:    today.Add(24 * time.Hour),
				status:      StatusConfirmed,
				wantErr:     nil,
				wantFinal:   StatusNoShow,
				wantRestore: true,
			},
			{
				name:        "already no-show returns nil idempotently",
				checkIn:     today.Add(-24 * time.Hour),
				checkOut:    today.Add(24 * time.Hour),
				status:      StatusNoShow,
				wantErr:     nil,
				wantFinal:   StatusNoShow,
				wantRestore: false,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := &Booking{
					ID:         "b-noshow",
					RoomTypeID: "std",
					CheckIn:    tc.checkIn,
					CheckOut:   tc.checkOut,
					NumRooms:   1,
					Status:     tc.status,
				}
				tx := newFakeTx(map[string]*Booking{"b-noshow": b}, nil)
				dateKey := "std|" + today.Format("2006-01-02")
				tx.inventory[dateKey] = 0

				svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b-noshow": b}})
				err := svc.MarkNoShow(ctx, "b-noshow")
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Errorf("expected error %v, got %v", tc.wantErr, err)
					}
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if b.Status != tc.wantFinal {
					t.Errorf("status = %v, want %v", b.Status, tc.wantFinal)
				}
			})
		}
	})
}




// BE-R06: create tanpa quote_id wajib ditolak sebelum inventory/booking/gateway berubah.
func TestCreate_QuoteRequired(t *testing.T) {
	tests := []struct {
		name      string
		quoteID   string
		terms     bool
		privacy   bool
		wantErr   error
		wantTouch bool
	}{
		{name: "tanpa quote dan tanpa consent", wantErr: ErrQuoteRequired},
		{name: "tanpa quote walau consent true", terms: true, privacy: true, wantErr: ErrQuoteRequired},
		{name: "quote ada tanpa consent", quoteID: "q-1", wantErr: ErrConsentRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := newFakeTx(map[string]*Booking{}, map[string][]string{})
			tx.inventory["std|2026-10-10"] = 5
			tx.inventory["std|2026-10-11"] = 5
			pay := &fakePayment{}
			inv := &fakeInvStore{avail: []inventory.Availability{
				{Date: date("2026-10-10"), TotalRooms: 5, AvailableRooms: 5},
				{Date: date("2026-10-11"), TotalRooms: 5, AvailableRooms: 5},
			}}
			svc := NewService(tx, inv, &fakeRates{}, pay, &fakeNotifier{}, &fakeReader{bookings: tx.bookings}, 30*time.Minute, slog.Default())
			_, _, err := svc.Create(context.Background(), CreateInput{
				RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"),
				NumRooms: 1, NumGuests: 2, GuestName: "Budi", GuestEmail: "budi@example.com",
				QuoteID: tc.quoteID, TermsAccepted: tc.terms, PrivacyAccepted: tc.privacy,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if len(tx.bookings) != 0 || tx.inventory["std|2026-10-10"] != 5 {
				t.Errorf("state mutated on rejected create: bookings=%d inv=%d", len(tx.bookings), tx.inventory["std|2026-10-10"])
			}
		})
	}
}

// withQuote menyimpan quote terkunci yang cocok dengan in ke svc dan mengisi QuoteID + consent (BE-R06).
func withQuote(svc *Service, in CreateInput) CreateInput {
	qs := rates.NewMemoryQuoteStore(15 * time.Minute)
	var nightly []rates.Quote
	for d := in.CheckIn; d.Before(in.CheckOut); d = d.AddDate(0, 0, 1) {
		nightly = append(nightly, rates.Quote{Date: d, RateMinor: 500_000})
	}
	subtotal := int64(len(nightly)) * 500_000 * int64(in.NumRooms)
	_ = qs.SaveQuote(context.Background(), rates.LockedQuote{
		ID: "q-test", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(15 * time.Minute),
		RoomTypeID: in.RoomTypeID, RatePlanCode: rates.RatePlanRoomOnly, CancellationCode: rates.PolicyFlexible48h,
		CheckIn: in.CheckIn, CheckOut: in.CheckOut, NumRooms: in.NumRooms, NumGuests: in.NumGuests,
		NightlyRates: nightly,
		Pricing:      rates.PricingBreakdown{RoomSubtotalMinor: subtotal, TotalPriceMinor: subtotal, Currency: "IDR"},
	})
	svc.SetQuoteStore(qs)
	in.QuoteID, in.TermsAccepted, in.PrivacyAccepted = "q-test", true, true
	return in
}

// Satu quote hanya boleh menghasilkan satu booking (unique quote_id di DB → ErrQuoteAlreadyUsed).
func TestCreate_QuoteSingleUse(t *testing.T) {
	tests := []struct {
		name      string
		insertErr error
		wantErr   error
	}{
		{name: "quote belum dipakai", insertErr: nil, wantErr: nil},
		{name: "quote sudah dipakai booking lain", insertErr: ErrQuoteAlreadyUsed, wantErr: ErrQuoteAlreadyUsed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := newFakeTx(map[string]*Booking{}, map[string][]string{})
			tx.inventory["std|2026-10-10"] = 5
			tx.inventory["std|2026-10-11"] = 5
			tx.insertErr = tc.insertErr
			inv := &fakeInvStore{avail: []inventory.Availability{
				{Date: date("2026-10-10"), TotalRooms: 5, AvailableRooms: 5},
				{Date: date("2026-10-11"), TotalRooms: 5, AvailableRooms: 5},
			}}
			svc := NewService(tx, inv, &fakeRates{}, &fakePayment{}, &fakeNotifier{}, &fakeReader{bookings: tx.bookings}, 30*time.Minute, slog.Default())
			_, _, err := svc.Create(context.Background(), withQuote(svc, CreateInput{
				RoomTypeID: "std", CheckIn: date("2026-10-10"), CheckOut: date("2026-10-12"),
				NumRooms: 1, NumGuests: 2, GuestName: "Budi", GuestEmail: "budi@example.com",
			}))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestBypassRoomReadinessContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{name: "default context tidak bypass", ctx: context.Background(), want: false},
		{name: "context dengan bypass", ctx: WithBypassRoomReadiness(context.Background()), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBypassRoomReadiness(tc.ctx); got != tc.want {
				t.Errorf("IsBypassRoomReadiness = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeCatalogReader struct {
	variants map[string]catalog.RoomVariant
}

func (f *fakeCatalogReader) GetVariant(_ context.Context, idOrCode string) (catalog.RoomVariant, error) {
	v, ok := f.variants[idOrCode]
	if !ok {
		return catalog.RoomVariant{}, catalog.ErrVariantNotFound
	}
	return v, nil
}

func TestServiceCreate_CatalogCapacityInvariant(t *testing.T) {
	fakeCat := &fakeCatalogReader{
		variants: map[string]catalog.RoomVariant{
			"sup-king": {
				ID:          "sup-king",
				Code:        "sup-king",
				MaxCapacity: 3,
				MaxAdults:   2,
				MaxChildren: 1,
			},
		},
	}

	tests := []struct {
		name       string
		roomTypeID string
		numRooms   int
		numGuests  int
		wantErr    error
	}{
		{
			name:       "num_guests kurang dari num_rooms ditolak",
			roomTypeID: "sup-king",
			numRooms:   2,
			numGuests:  1,
			wantErr:    ErrInvalidCapacity,
		},
		{
			name:       "melebihi kapasitas varian kamar ditolak",
			roomTypeID: "sup-king",
			numRooms:   1,
			numGuests:  4, // max 3
			wantErr:    ErrExceedsCapacity,
		},
		{
			name:       "melebihi kapasitas varian multi-kamar ditolak",
			roomTypeID: "sup-king",
			numRooms:   2,
			numGuests:  7, // max 3 * 2 = 6
			wantErr:    ErrExceedsCapacity,
		},
		{
			name:       "varian tidak ditemukan di katalog ditolak",
			roomTypeID: "unknown-variant",
			numRooms:   1,
			numGuests:  2,
			wantErr:    ErrNotFound,
		},
		{
			name:       "kapasitas valid diterima",
			roomTypeID: "sup-king",
			numRooms:   1,
			numGuests:  3,
			wantErr:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := newFakeTx(nil, map[string][]string{"sup-king": {"101", "102"}})
			tx.inventory["sup-king|2026-10-10"] = 5
			tx.inventory["sup-king|2026-10-11"] = 5
			inv := &fakeInvStore{avail: []inventory.Availability{
				{Date: date("2026-10-10"), TotalRooms: 5, AvailableRooms: 5},
				{Date: date("2026-10-11"), TotalRooms: 5, AvailableRooms: 5},
			}}
			svc := NewService(tx, inv, &fakeRates{}, &fakePayment{}, &fakeNotifier{}, &fakeReader{bookings: tx.bookings}, 30*time.Minute, slog.Default())
			svc.SetCatalogStore(fakeCat)

			input := CreateInput{
				RoomTypeID:      tc.roomTypeID,
				CheckIn:         date("2026-10-10"),
				CheckOut:        date("2026-10-12"),
				NumRooms:        tc.numRooms,
				NumGuests:       tc.numGuests,
				GuestName:       "Tamu Uji",
				GuestEmail:      "tamu@example.com",
				TermsAccepted:   true,
				PrivacyAccepted: true,
			}
			if tc.wantErr == nil || errors.Is(tc.wantErr, ErrExceedsCapacity) || errors.Is(tc.wantErr, ErrNotFound) {
				input = withQuote(svc, input)
			}
			_, _, err := svc.Create(context.Background(), input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

