package booking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

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
	byRoom       map[string][]assignment   // roomNumber → daftar assignment
	events       []string                  // topic yang di-publish
	failLock     bool
	failInsert   bool
	roomNights   map[string][]rates.Quote  // bookingID → quotes
}

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func newFakeTx(bookings map[string]*Booking, rooms map[string][]string) *fakeTx {
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

func (f *fakeTx) InsertBookingWithHold(_ context.Context, b *Booking, quotes []rates.Quote, _ time.Time) error {
	if f.failInsert {
		return errors.New("insert failed")
	}
	b.ID = "generated-" + b.GuestName
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

	b, charge, err := svc.Create(context.Background(), in)
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

func TestCreate_InvalidInput(t *testing.T) {
	svc := newTestService(newFakeTx(nil, nil), &fakeReader{})

	// CheckOut before CheckIn
	_, _, err := svc.Create(context.Background(), CreateInput{
		CheckIn:  date("2026-10-12"),
		CheckOut: date("2026-10-10"),
		NumRooms: 1, NumGuests: 1,
	})
	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("err = %v, want ErrInvalidDateRange", err)
	}

	// Zero capacity
	_, _, err = svc.Create(context.Background(), CreateInput{
		CheckIn:  date("2026-10-10"),
		CheckOut: date("2026-10-12"),
		NumRooms: 0, NumGuests: 1,
	})
	if !errors.Is(err, ErrInvalidCapacity) {
		t.Errorf("err = %v, want ErrInvalidCapacity", err)
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

	_, _, err := svc.Create(context.Background(), CreateInput{
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-11"),
		NumRooms:   1,
		NumGuests:  1,
		GuestName:  "Test",
	})
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
	b := &Booking{
		ID:         "b1",
		RoomTypeID: "std",
		CheckIn:    date("2026-10-10"),
		CheckOut:   date("2026-10-11"),
		NumRooms:   1,
		Status:     StatusConfirmed,
	}
	tx := newFakeTx(map[string]*Booking{"b1": b}, nil)
	tx.inventory["std|2026-10-10"] = 0
	svc := newTestService(tx, &fakeReader{bookings: map[string]*Booking{"b1": b}})

	if err := svc.MarkNoShow(context.Background(), "b1"); err != nil {
		t.Fatalf("MarkNoShow err = %v", err)
	}
	if b.Status != StatusNoShow {
		t.Errorf("status = %s, want no_show", b.Status)
	}
	if tx.inventory["std|2026-10-10"] != 1 {
		t.Errorf("inventory = %d, want 1 (restored)", tx.inventory["std|2026-10-10"])
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

