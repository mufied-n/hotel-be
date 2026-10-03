package integration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/booking"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
	"github.com/example/hotel-booking/internal/workers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dummyPayment struct{}

func (d *dummyPayment) CreateCharge(_ context.Context, _ booking.Booking, _ int64, _ string) (booking.ChargeResult, error) {
	return booking.ChargeResult{PaymentURL: "http://pay.test/123", Reference: "ref-int-001"}, nil
}

type dummyNotifier struct{}

func (d *dummyNotifier) SendBookingConfirmed(_ context.Context, _ booking.Booking) error {
	return nil
}

type dummyRateProvider struct{}

func (d *dummyRateProvider) QuotesForStay(_ context.Context, _ string, from, to time.Time) ([]rates.Quote, error) {
	days := int(to.Sub(from).Hours() / 24)
	if days <= 0 {
		days = 1
	}
	var qs []rates.Quote
	for i := 0; i < days; i++ {
		qs = append(qs, rates.Quote{
			Date:      from.Add(time.Duration(i*24) * time.Hour),
			RateMinor: 1_000_000,
		})
	}
	return qs, nil
}

func setupRealBookingService(pool *pgxpool.Pool) (*booking.Service, *booking.PostgresReader, *booking.PostgresPaymentAttemptStore) {
	runner := &booking.PostgresTxRunner{Pool: pool}
	reader := &booking.PostgresReader{Pool: pool}
	attemptStore := booking.NewPostgresPaymentAttemptStore(pool)
	invStore := &inventory.PostgresStore{Pool: pool}
	rateEngine := rates.NewEngine(map[string]int64{
		"01900000-0000-7000-8000-000000000001": 1_000_000,
	}, 1.25)
	svc := booking.NewService(
		runner,
		invStore,
		rateEngine,
		&dummyPayment{},
		&dummyNotifier{},
		reader,
		30*time.Minute,
		nil,
	)
	qs := rates.NewMemoryQuoteStore(15 * time.Minute)
	svc.SetQuoteStore(qs)
	quoteStores.Store(svc, qs)
	svc.SetPaymentAttemptStore(attemptStore)
	return svc, reader, attemptStore
}

// quoteStores memetakan service uji ke quote store-nya agar create (wajib quote_id, BE-R06) dapat
// menyiapkan quote terkunci yang cocok dengan input secara thread-safe.
var quoteStores sync.Map

// quoted menyimpan quote terkunci yang cocok dengan in dan mengisi QuoteID + consent.
func quoted(svc *booking.Service, in booking.CreateInput) booking.CreateInput {
	v, _ := quoteStores.Load(svc)
	qs := v.(*rates.MemoryQuoteStore)
	var nightly []rates.Quote
	for d := in.CheckIn; d.Before(in.CheckOut); d = d.AddDate(0, 0, 1) {
		nightly = append(nightly, rates.Quote{Date: d, RateMinor: 1_000_000})
	}
	total := int64(len(nightly)) * 1_000_000 * int64(in.NumRooms)
	id := uuid.NewString()
	_ = qs.SaveQuote(context.Background(), rates.LockedQuote{
		ID: id, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(15 * time.Minute),
		RoomTypeID: in.RoomTypeID, RatePlanCode: rates.RatePlanRoomOnly, CancellationCode: rates.PolicyFlexible48h,
		CheckIn: in.CheckIn, CheckOut: in.CheckOut, NumRooms: in.NumRooms, NumGuests: in.NumGuests,
		NightlyRates: nightly,
		Pricing:      rates.PricingBreakdown{RoomSubtotalMinor: total, TotalPriceMinor: total, Currency: "IDR"},
	})
	in.QuoteID, in.TermsAccepted, in.PrivacyAccepted = id, true, true
	return in
}

// TestRealDB_RaceOnLastRoom memverifikasi bahwa penguncian SELECT ... FOR UPDATE pada PostgreSQL 18
// mencegah double booking saat N request paralel merebut 1 kamar terakhir (BE-G20, AC-F01).
func TestRealDB_RaceOnLastRoom(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, _, _ := setupRealBookingService(pool)

	ctx := context.Background()
	roomTypeID := "01900000-0000-7000-8000-000000000001"
	targetDate := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour) // Tomorrow
	checkoutDate := targetDate.Add(24 * time.Hour)

	// Set ketersediaan tepat = 1 kamar
	_, err := pool.Exec(ctx, `
		UPDATE inventory 
		SET available_rooms = 1, total_rooms = 1 
		WHERE room_type_id = $1 AND date = $2;
	`, roomTypeID, targetDate)
	if err != nil {
		t.Fatalf("failed to set available_rooms = 1: %v", err)
	}

	numConcurrent := 20
	var wg sync.WaitGroup
	var successCount int32
	var failCount int32

	// Barrier start channel agar seluruh 20 goroutines mulai serentak di milidetik yang sama
	startBarrier := make(chan struct{})

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		guestIndex := i
		go func() {
			defer wg.Done()
			<-startBarrier

			_, _, err := svc.Create(context.Background(), quoted(svc, booking.CreateInput{
				RoomTypeID:           roomTypeID,
				CheckIn:              targetDate,
				CheckOut:             checkoutDate,
				NumRooms:             1,
				NumGuests:            2,
				GuestName:            fmt.Sprintf("Guest %d", guestIndex),
				GuestEmail:           fmt.Sprintf("guest%d@example.com", guestIndex),
				GuestPhone:           "+6281234567890",
				EstimatedArrivalTime: "14:00",
			}))
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else {
				if errors.Is(err, inventory.ErrInsufficient) || errors.Is(err, booking.ErrInsufficient) {
					atomic.AddInt32(&failCount, 1)
				} else {
					t.Errorf("unexpected error on concurrent create: %v", err)
				}
			}
		}()
	}

	// Lepas barrier
	close(startBarrier)
	wg.Wait()

	// 1. Tepat 1 transaksi berhasil, 19 gagal
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful booking hold, got %d", successCount)
	}
	if failCount != int32(numConcurrent-1) {
		t.Fatalf("expected %d failed booking holds, got %d", numConcurrent-1, failCount)
	}

	// 2. Cek database langsung: stok kamar harus tepat 0 (tidak boleh negatif)
	var finalAvailable int
	err = pool.QueryRow(ctx, `
		SELECT available_rooms 
		FROM inventory 
		WHERE room_type_id = $1 AND date = $2;
	`, roomTypeID, targetDate).Scan(&finalAvailable)
	if err != nil {
		t.Fatalf("query final available rooms failed: %v", err)
	}
	if finalAvailable != 0 {
		t.Errorf("expected final available_rooms = 0, got %d", finalAvailable)
	}

	// 3. Cek tabel bookings: tepat 1 baris
	var totalBookings int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM bookings;").Scan(&totalBookings)
	if err != nil {
		t.Fatalf("query bookings count failed: %v", err)
	}
	if totalBookings != 1 {
		t.Errorf("expected exactly 1 row in bookings, got %d", totalBookings)
	}
}

// TestRealDB_MultiNightRollbackAtomicity memverifikasi sifat all-or-nothing transaksi PostgreSQL:
// jika 1 malam di tengah/akhir habis, seluruh malam yang sempat didecrement di-rollback (BE-G20, AC-F02).
func TestRealDB_MultiNightRollbackAtomicity(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, _, _ := setupRealBookingService(pool)

	ctx := context.Background()
	roomTypeID := "01900000-0000-7000-8000-000000000001"
	d1 := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour) // Night 1
	d2 := d1.Add(24 * time.Hour)                                        // Night 2
	d3 := d2.Add(24 * time.Hour)                                        // Night 3 (Habis!)
	d4 := d3.Add(24 * time.Hour)                                        // Checkout

	// Set Night 1 = 2 kamar, Night 2 = 2 kamar, Night 3 = 0 kamar
	_, _ = pool.Exec(ctx, "UPDATE inventory SET available_rooms = 2 WHERE room_type_id = $1 AND date = $2;", roomTypeID, d1)
	_, _ = pool.Exec(ctx, "UPDATE inventory SET available_rooms = 2 WHERE room_type_id = $1 AND date = $2;", roomTypeID, d2)
	_, _ = pool.Exec(ctx, "UPDATE inventory SET available_rooms = 0 WHERE room_type_id = $1 AND date = $2;", roomTypeID, d3)

	// Coba booking 3 malam (d1 -> d4)
	_, _, err := svc.Create(ctx, quoted(svc, booking.CreateInput{
		RoomTypeID:           roomTypeID,
		CheckIn:              d1,
		CheckOut:             d4,
		NumRooms:             1,
		NumGuests:            2,
		GuestName:            "Atomicity Tester",
		GuestEmail:           "atomicity@example.com",
		GuestPhone:           "+6281234567890",
		EstimatedArrivalTime: "15:00",
	}))
	if err == nil {
		t.Fatal("expected Create to fail due to night 3 insufficient stock, got nil")
	}

	// Verifikasi Rollback Mutlak: Stok d1 dan d2 TIDAK boleh berkurang!
	var stock1, stock2 int
	_ = pool.QueryRow(ctx, "SELECT available_rooms FROM inventory WHERE room_type_id = $1 AND date = $2;", roomTypeID, d1).Scan(&stock1)
	_ = pool.QueryRow(ctx, "SELECT available_rooms FROM inventory WHERE room_type_id = $1 AND date = $2;", roomTypeID, d2).Scan(&stock2)

	if stock1 != 2 {
		t.Errorf("Night 1 stock leaked! expected 2, got %d", stock1)
	}
	if stock2 != 2 {
		t.Errorf("Night 2 stock leaked! expected 2, got %d", stock2)
	}

	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM bookings;").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 bookings in database after rollback, got %d", count)
	}
}

// TestRealDB_ParallelRoomAssignment_SkipLocked memverifikasi bahwa query CTE dengan
// FOR UPDATE OF r SKIP LOCKED memungkinkan 2 resepsionis check-in secara paralel tanpa tabrakan 23P01 (BE-G17, BE-G20).
func TestRealDB_ParallelRoomAssignment_SkipLocked(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, _, _ := setupRealBookingService(pool)

	ctx := context.Background()
	roomTypeID := "01900000-0000-7000-8000-000000000001"
	checkIn := time.Now().UTC().Truncate(24 * time.Hour)
	checkOut := checkIn.Add(48 * time.Hour)

	// Buat 2 booking confirmed di database
	b1, _, err1 := svc.Create(ctx, quoted(svc, booking.CreateInput{
		RoomTypeID:  roomTypeID,
		CheckIn:     checkIn,
		CheckOut:    checkOut,
		NumRooms:    1,
		NumGuests:   2,
		GuestName:   "Tamu A",
		GuestEmail:  "tamu.a@example.com",
		GuestPhone:  "+6281111111111",
	}))
	if err1 != nil {
		t.Fatalf("create booking 1 failed: %v", err1)
	}
	_ = svc.Confirm(ctx, b1.ID)

	b2, _, err2 := svc.Create(ctx, quoted(svc, booking.CreateInput{
		RoomTypeID:  roomTypeID,
		CheckIn:     checkIn,
		CheckOut:    checkOut,
		NumRooms:    1,
		NumGuests:   2,
		GuestName:   "Tamu B",
		GuestEmail:  "tamu.b@example.com",
		GuestPhone:  "+6282222222222",
	}))
	if err2 != nil {
		t.Fatalf("create booking 2 failed: %v", err2)
	}
	_ = svc.Confirm(ctx, b2.ID)

	// Eksekusi paralel 2 check-in
	var wg sync.WaitGroup
	var res1, res2 booking.CheckInResult
	var errA, errB error

	barrier := make(chan struct{})
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-barrier
		res1, errA = svc.CheckIn(context.Background(), b1.ID)
	}()

	go func() {
		defer wg.Done()
		<-barrier
		res2, errB = svc.CheckIn(context.Background(), b2.ID)
	}()

	close(barrier)
	wg.Wait()

	if errA != nil {
		t.Fatalf("check-in booking 1 failed: %v", errA)
	}
	if errB != nil {
		t.Fatalf("check-in booking 2 failed: %v", errB)
	}

	if len(res1.RoomNumbers) != 1 || len(res2.RoomNumbers) != 1 {
		t.Fatalf("expected each check-in to have 1 room number, got %v and %v", res1.RoomNumbers, res2.RoomNumbers)
	}
	if res1.RoomNumbers[0] == res2.RoomNumbers[0] {
		t.Fatalf("COLLISION! Both guests got the same room number: %s", res1.RoomNumbers[0])
	}

	// Verifikasi kamar tersimpan di database
	var roomsA []string
	rowsA, err := pool.Query(ctx, "SELECT room_number FROM room_assignments WHERE booking_id = $1;", b1.ID)
	if err != nil {
		t.Fatalf("query room_assignments b1 failed: %v", err)
	}
	for rowsA.Next() {
		var r string
		_ = rowsA.Scan(&r)
		roomsA = append(roomsA, r)
	}
	rowsA.Close()

	var roomsB []string
	rowsB, err := pool.Query(ctx, "SELECT room_number FROM room_assignments WHERE booking_id = $1;", b2.ID)
	if err != nil {
		t.Fatalf("query room_assignments b2 failed: %v", err)
	}
	for rowsB.Next() {
		var r string
		_ = rowsB.Scan(&r)
		roomsB = append(roomsB, r)
	}
	rowsB.Close()

	if len(roomsA) == 0 || len(roomsB) == 0 || roomsA[0] == roomsB[0] {
		t.Errorf("DB assignments collision or empty: %v vs %v", roomsA, roomsB)
	}
}

// TestRealDB_GiSTExclusionConstraintDoubleBookingRejection memverifikasi bahwa constraint
// EXCLUDE USING gist (room_id WITH =, stay_range WITH &&) menolak double booking fisik kamar (BE-G20).
func TestRealDB_GiSTExclusionConstraintDoubleBookingRejection(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)

	ctx := context.Background()

	// Ambil 1 kamar fisik nyata
	var roomNumber, roomTypeID string
	err := pool.QueryRow(ctx, "SELECT room_number, room_type_id FROM rooms LIMIT 1;").Scan(&roomNumber, &roomTypeID)
	if err != nil {
		t.Fatalf("failed to query room: %v", err)
	}

	// Insert 2 booking
	b1ID := "01900000-0000-7000-8000-000000000011"
	b2ID := "01900000-0000-7000-8000-000000000022"
	now := time.Now().UTC().Truncate(24 * time.Hour)
	d1 := now.Add(24 * time.Hour)
	d2 := now.Add(48 * time.Hour)
	d3 := now.Add(72 * time.Hour)

	_, err = pool.Exec(ctx, `
		INSERT INTO bookings (id, room_type_id, status, check_in, check_out, num_rooms, num_guests, guest_name, guest_email, total_price_minor)
		VALUES 
			($1, $2, 'confirmed', $3, $4, 1, 2, 'Guest 1', 'g1@test.com', 1000000),
			($5, $2, 'confirmed', $6, $7, 1, 2, 'Guest 2', 'g2@test.com', 1000000);
	`, b1ID, roomTypeID, d1, d3, b2ID, d2, d3)
	if err != nil {
		t.Fatalf("insert bookings failed: %v", err)
	}

	// Alokasi 1: Kamar untuk b1 pada range [d1, d3)
	_, err = pool.Exec(ctx, `
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		VALUES ($1, $2, daterange($3::date, $4::date, '[)'));
	`, b1ID, roomNumber, d1, d3)
	if err != nil {
		t.Fatalf("first room assignment failed: %v", err)
	}

	// Alokasi 2: Coba alokasi kamar yang SAMA untuk b2 pada range tumpang tindih [d2, d3)
	_, err = pool.Exec(ctx, `
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		VALUES ($1, $2, daterange($3::date, $4::date, '[)'));
	`, b2ID, roomNumber, d2, d3)
	if err == nil {
		t.Fatal("expected GiST exclusion violation on overlapping room assignment, but got nil error")
	}

	// Verifikasi kode error PostgreSQL adalah 23P01 (exclusion_violation)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code != "23P01" {
			t.Errorf("expected SQLSTATE 23P01 (exclusion_violation), got %s", pgErr.Code)
		}
	} else {
		t.Errorf("expected pgconn.PgError, got %T: %v", err, err)
	}
}

// TestRealDB_HoldExpirySweepVsPaymentRace memverifikasi bahwa pembayaran yang tiba terlambat
// setelah hold kedaluwarsa ditolak secara tegas (HOLD_EXPIRED) dan hold disapu (BE-G12, BE-G20).
func TestRealDB_HoldExpirySweepVsPaymentRace(t *testing.T) {
	pool := GetTestPool(t)
	ResetTestData(t, pool)
	svc, reader, attemptStore := setupRealBookingService(pool)

	ctx := context.Background()
	roomTypeID := "01900000-0000-7000-8000-000000000001"
	checkIn := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	checkOut := checkIn.Add(24 * time.Hour)

	b, _, err := svc.Create(ctx, quoted(svc, booking.CreateInput{
		RoomTypeID:           roomTypeID,
		CheckIn:              checkIn,
		CheckOut:             checkOut,
		NumRooms:             1,
		NumGuests:            2,
		GuestName:            "Late Payer",
		GuestEmail:           "late@example.com",
		GuestPhone:           "+6281234567890",
		EstimatedArrivalTime: "14:00",
	}))
	if err != nil {
		t.Fatalf("create booking failed: %v", err)
	}

	// Simulasikan hold kedaluwarsa 5 menit yang lalu di database
	pastExpiry := time.Now().UTC().Add(-5 * time.Minute)
	_, err = pool.Exec(ctx, "UPDATE holds SET expires_at = $1 WHERE booking_id = $2;", pastExpiry, b.ID)
	if err != nil {
		t.Fatalf("simulate past expiry holds failed: %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE bookings SET expires_at = $1 WHERE id = $2;", pastExpiry, b.ID)
	if err != nil {
		t.Fatalf("simulate past expiry bookings failed: %v", err)
	}

	// Jalankan Confirm (pembayaran tiba terlambat)
	confirmErr := svc.Confirm(ctx, b.ID)
	if !errors.Is(confirmErr, booking.ErrHoldExpired) {
		t.Fatalf("expected ErrHoldExpired for late payment, got %v", confirmErr)
	}

	// Cek tabel payment_attempts untuk kepatuhan PCI-DSS audit ledger
	attempts, err := attemptStore.GetAttemptsByBookingID(ctx, b.ID)
	if err != nil {
		t.Fatalf("query attempts failed: %v", err)
	}
	if len(attempts) == 0 {
		t.Fatal("expected payment attempt record, got 0")
	}
	if attempts[0].Status != "received_after_expiry" {
		t.Errorf("expected attempt status 'received_after_expiry', got %s", attempts[0].Status)
	}

	// Jalankan SweepExpiredHolds untuk memastikan hold pending dibatalkan dan inventaris dipulihkan
	workers.SweepExpiredHolds(ctx, pool, func(ctx context.Context, bookingID string) error {
		return svc.Cancel(ctx, bookingID)
	}, slog.Default())

	// Verifikasi booking menjadi cancelled
	finalB, err := reader.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("get booking failed: %v", err)
	}
	if finalB.Status != booking.StatusCancelled {
		t.Errorf("expected booking status cancelled, got %s", finalB.Status)
	}
}
