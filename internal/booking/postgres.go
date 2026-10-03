package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/hotel-booking/internal/rates"
)

// PostgresTxRunner mengimplementasikan booking.TxRunner di atas pgx.
// Satu tx.Postgres = satu transaksi database yang memuat perubahan booking,
// inventory, DAN event outbox (atomicity garansi penuh — desain §5.3).
type PostgresTxRunner struct {
	Pool *pgxpool.Pool
}

// txCtx membungkus pgx.Tx agar InventoryTx & EventPublisher beroperasi pada
// transaksi yang sama.
type txCtx struct {
	tx pgx.Tx
}

// InTx menjalankan fn dalam satu transaksi. Rollback otomatis saat fn error.
func (r *PostgresTxRunner) InTx(ctx context.Context, fn func(InventoryTx, EventPublisher) error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("booking: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op bila sudah commit

	t := &txCtx{tx: tx}
	if err := fn(t, t); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------- InventoryTx (dalam transaksi) ----------

// LockAndDecrement: critical path §12.3.
//   - ORDER BY date ASC → urutan lock deterministik (anti-deadlock)
//   - FOR UPDATE       → lock baris sampai commit
//   - UPDATE ... WHERE available_rooms >= $n → verifikasi atomik
func (t *txCtx) LockAndDecrement(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	tag, err := t.tx.Exec(ctx, `
		WITH locked AS (
			SELECT date FROM inventory
			WHERE room_type_id = $1 AND date >= $2 AND date < $3
			ORDER BY date ASC
			FOR UPDATE
		)
		UPDATE inventory i
		SET available_rooms = i.available_rooms - $4,
		    version = i.version + 1
		FROM locked l
		WHERE i.room_type_id = $1 AND i.date = l.date
		  AND i.available_rooms >= $4`,
		roomTypeID, from, to, numRooms)
	if err != nil {
		return fmt.Errorf("booking: lock_and_decrement: %w", err)
	}
	if tag.RowsAffected() != int64(daysBetween(from, to)) {
		// Termasuk kasus kalah race: baris lain sudah mendecrement stok sebelum
		// lock kita diperoleh → re-evaluasi WHERE menolak baris tersebut.
		return fmt.Errorf("%w for the requested date range", ErrInsufficient)
	}
	return nil
}

func (t *txCtx) Increment(ctx context.Context, roomTypeID string, from, to time.Time, numRooms int) error {
	tag, err := t.tx.Exec(ctx, `
		UPDATE inventory
		SET available_rooms = LEAST(available_rooms + $4, total_rooms),
		    version = version + 1
		WHERE room_type_id = $1 AND date >= $2 AND date < $3`,
		roomTypeID, from, to, numRooms)
	if err != nil {
		return fmt.Errorf("booking: increment: %w", err)
	}
	if tag.RowsAffected() != int64(daysBetween(from, to)) {
		return errors.New("booking: inventory rows missing during increment")
	}
	return nil
}

func (t *txCtx) InsertBookingWithHold(ctx context.Context, b *Booking, quotes []rates.Quote, holdExpiresAt time.Time) error {
	var quoteID any
	if b.QuoteID != "" {
		quoteID = b.QuoteID
	}
	b.ExpiresAt = &holdExpiresAt
	err := t.tx.QueryRow(ctx, `
		INSERT INTO bookings
			(room_type_id, check_in, check_out, num_rooms, num_guests,
			 status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at,
			 quote_id, rate_plan_code, cancellation_policy, cancellation_desc,
			 room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor,
			 terms_accepted, terms_accepted_at,
			 guest_phone, estimated_arrival_time, special_requests, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
		RETURNING id`,
		b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms, b.NumGuests,
		string(b.Status), b.TotalPriceMinor, b.Currency, b.GuestName, b.GuestEmail, b.GuestToken, b.CreatedAt,
		quoteID, b.RatePlanCode, b.CancellationPolicy, b.CancellationDesc,
		b.RoomSubtotalMinor, b.BreakfastChargeMinor, b.DiscountMinor, b.TaxMinor,
		b.TermsAccepted, b.TermsAcceptedAt,
		b.GuestPhone, b.EstimatedArrivalTime, b.SpecialRequests, b.ExpiresAt,
	).Scan(&b.ID)
	if err != nil {
		return fmt.Errorf("booking: insert: %w", err)
	}

	for _, q := range quotes {
		if _, err := t.tx.Exec(ctx, `
			INSERT INTO reservation_room_nights (booking_id, night_date, rate_minor)
			VALUES ($1, $2, $3)
			ON CONFLICT (booking_id, night_date) DO UPDATE SET rate_minor = EXCLUDED.rate_minor`,
			b.ID, q.Date, q.RateMinor); err != nil {
			return fmt.Errorf("booking: insert reservation_room_nights: %w", err)
		}
	}

	if _, err := t.tx.Exec(ctx, `
		INSERT INTO holds (booking_id, expires_at) VALUES ($1, $2)`,
		b.ID, holdExpiresAt); err != nil {
		return fmt.Errorf("booking: insert hold: %w", err)
	}
	return nil
}

func (t *txCtx) GetForUpdate(ctx context.Context, id string) (Booking, error) {
	return scanBooking(t.tx.QueryRow(ctx, `
		SELECT id, room_type_id, check_in, check_out, num_rooms, num_guests,
		       status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at,
		       COALESCE(quote_id::text, ''), COALESCE(rate_plan_code, 'room_only'),
		       COALESCE(cancellation_policy, 'flexible_48h'), COALESCE(cancellation_desc, ''),
		       COALESCE(room_subtotal_minor, 0), COALESCE(breakfast_charge_minor, 0),
		       COALESCE(discount_minor, 0), COALESCE(tax_minor, 0),
		       COALESCE(terms_accepted, true), terms_accepted_at,
		       COALESCE(guest_phone, ''), COALESCE(estimated_arrival_time, ''),
		       COALESCE(special_requests, ''), expires_at
		FROM bookings WHERE id = $1 FOR UPDATE`, id))
}

func (t *txCtx) UpdateStatus(ctx context.Context, id string, to Status) error {
	tag, err := t.tx.Exec(ctx, `
		UPDATE bookings SET status = $2, updated_at = now() WHERE id = $1`,
		id, string(to))
	if err != nil {
		return fmt.Errorf("booking: update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if to == StatusCheckedOut {
		_, _ = t.tx.Exec(ctx, `
			UPDATE rooms
			SET cleanliness_status = 'vacant_dirty', updated_by = 'front_desk', updated_at = now()
			WHERE room_number IN (SELECT room_number FROM room_assignments WHERE booking_id = $1)`,
			id)
	}
	return nil
}

// PickAndAssignRooms memilih kamar fisik bebas sejumlah count dan memasang assignment (BE-G17, FR-HK-04).
//   - NOT EXISTS: hindari kamar yang sudah ter-assign untuk rentang overlap.
//   - FOR UPDATE OF r SKIP LOCKED: kunci baris kandidat kamar sehingga check-in paralel
//     memilih kamar fisik berikutnya secara otomatis tanpa saling menggagalkan.
//   - EXCLUDE USING GIST: backstop bila terjadi exclusion_violation (23P01) pada query/rows.Err,
//     keduanya dipetakan ke ErrTransientConflict.
//   - FR-HK-04: Kamar yang dipilih WAJIB berstatus 'inspected'. Jika kamar bebas ada namun
//     belum inspected (dirty/cleaning/out_of_service), kembalikan ErrRoomNotReady.
//   - Saat assignment berhasil, status kebersihan kamar otomatis diubah menjadi 'occupied'.
func (t *txCtx) PickAndAssignRooms(ctx context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error) {
	cleanlinessFilter := "AND r.cleanliness_status = 'inspected'"
	if IsBypassRoomReadiness(ctx) {
		cleanlinessFilter = "AND r.cleanliness_status != 'out_of_order'"
	}

	query := fmt.Sprintf(`
		WITH free_inspected_rooms AS (
			SELECT r.room_number
			FROM rooms r
			WHERE r.room_type_id = $1
			  %s
			  AND NOT EXISTS (
				SELECT 1 FROM room_assignments ra
				WHERE ra.room_number = r.room_number
				  AND ra.stay_dates && daterange($2::date, $3::date, '[)')
			  )
			ORDER BY r.room_number
			LIMIT $4
			FOR UPDATE OF r SKIP LOCKED
		)
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		SELECT $5, room_number, daterange($2::date, $3::date, '[)')
		FROM free_inspected_rooms
		RETURNING room_number`, cleanlinessFilter)

	rows, err := t.tx.Query(ctx, query,
		roomTypeID, checkIn, checkOut, count, bookingID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40001") { // exclusion_violation or serialization_failure
			return nil, ErrTransientConflict
		}
		return nil, fmt.Errorf("booking: assign rooms: %w", err)
	}
	defer rows.Close()

	var assigned []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, fmt.Errorf("booking: scan assigned room: %w", err)
		}
		assigned = append(assigned, room)
	}
	if err := rows.Err(); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40001") {
			return nil, ErrTransientConflict
		}
		return nil, fmt.Errorf("booking: assigned rows: %w", err)
	}

	if len(assigned) < count {
		if IsBypassRoomReadiness(ctx) {
			return nil, ErrNoRoomAvailable
		}
		// Evaluasi apakah kekurangan kamar disebabkan karena kamar memang habis atau belum diinspeksi (FR-HK-04)
		var freeCount int
		_ = t.tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM rooms r
			WHERE r.room_type_id = $1
			  AND r.cleanliness_status != 'out_of_order'
			  AND NOT EXISTS (
				SELECT 1 FROM room_assignments ra
				WHERE ra.room_number = r.room_number
				  AND ra.stay_dates && daterange($2::date, $3::date, '[)')
			  )`,
			roomTypeID, checkIn, checkOut).Scan(&freeCount)

		if freeCount >= count {
			return nil, ErrRoomNotReady
		}
		return nil, ErrNoRoomAvailable
	}

	_, err = t.tx.Exec(ctx, `
		UPDATE rooms
		SET cleanliness_status = 'occupied', updated_by = 'front_desk', updated_at = now()
		WHERE room_number = ANY($1)`,
		assigned)
	if err != nil {
		return nil, fmt.Errorf("booking: update room to occupied: %w", err)
	}

	return assigned, nil
}

func (t *txCtx) GetRoomAssignments(ctx context.Context, bookingID string) ([]string, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT room_number FROM room_assignments WHERE booking_id = $1 ORDER BY room_number`,
		bookingID)
	if err != nil {
		return nil, fmt.Errorf("booking: get assignments: %w", err)
	}
	defer rows.Close()

	var rooms []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, fmt.Errorf("booking: scan assignment: %w", err)
		}
		rooms = append(rooms, room)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("booking: assignment rows: %w", err)
	}
	if len(rooms) == 0 {
		return nil, ErrNotFound
	}
	return rooms, nil
}

// ---------- EventPublisher (outbox, dalam transaksi yang sama) ----------

func (t *txCtx) PublishTx(ctx context.Context, topic string, payload []byte) error {
	_, err := t.tx.Exec(ctx,
		`INSERT INTO outbox (topic, payload) VALUES ($1, $2)`, topic, payload)
	if err != nil {
		return fmt.Errorf("booking: outbox publish: %w", err)
	}
	return nil
}

// ---------- helper ----------

func daysBetween(from, to time.Time) int {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC) / (24 * time.Hour))
}

// PostgresReader membaca booking tanpa transaksi (port Reader).
type PostgresReader struct{ Pool *pgxpool.Pool }

func (r *PostgresReader) Get(ctx context.Context, id string) (Booking, error) {
	return scanBooking(r.Pool.QueryRow(ctx, `
		SELECT id, room_type_id, check_in, check_out, num_rooms, num_guests,
		       status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at,
		       COALESCE(quote_id::text, ''), COALESCE(rate_plan_code, 'room_only'),
		       COALESCE(cancellation_policy, 'flexible_48h'), COALESCE(cancellation_desc, ''),
		       COALESCE(room_subtotal_minor, 0), COALESCE(breakfast_charge_minor, 0),
		       COALESCE(discount_minor, 0), COALESCE(tax_minor, 0),
		       COALESCE(terms_accepted, true), terms_accepted_at,
		       COALESCE(guest_phone, ''), COALESCE(estimated_arrival_time, ''),
		       COALESCE(special_requests, ''), expires_at
		FROM bookings WHERE id = $1`, id))
}

type rowScanner interface{ Scan(dest ...any) error }

func scanBooking(row rowScanner) (Booking, error) {
	var b Booking
	var status string
	err := row.Scan(&b.ID, &b.RoomTypeID, &b.CheckIn, &b.CheckOut, &b.NumRooms,
		&b.NumGuests, &status, &b.TotalPriceMinor, &b.Currency,
		&b.GuestName, &b.GuestEmail, &b.GuestToken, &b.CreatedAt,
		&b.QuoteID, &b.RatePlanCode, &b.CancellationPolicy, &b.CancellationDesc,
		&b.RoomSubtotalMinor, &b.BreakfastChargeMinor, &b.DiscountMinor, &b.TaxMinor,
		&b.TermsAccepted, &b.TermsAcceptedAt,
		&b.GuestPhone, &b.EstimatedArrivalTime, &b.SpecialRequests, &b.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("booking: scan: %w", err)
	}
	b.Status = Status(status)
	return b, nil
}

// PostgresPaymentAttemptStore mencatat audit percobaan pembayaran ke tabel payment_attempts (BE-G11).
type PostgresPaymentAttemptStore struct {
	pool *pgxpool.Pool
}

func NewPostgresPaymentAttemptStore(pool *pgxpool.Pool) *PostgresPaymentAttemptStore {
	return &PostgresPaymentAttemptStore{pool: pool}
}

func (s *PostgresPaymentAttemptStore) RecordAttempt(ctx context.Context, attempt PaymentAttempt) error {
	if s.pool == nil {
		return nil
	}
	var payloadBytes []byte
	if attempt.Payload != nil {
		payloadBytes, _ = json.Marshal(attempt.Payload)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO payment_attempts
			(booking_id, provider, provider_reference, amount_minor, currency, status, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		attempt.BookingID, attempt.Provider, attempt.ProviderReference,
		attempt.AmountMinor, attempt.Currency, attempt.Status, payloadBytes,
		attempt.CreatedAt, attempt.UpdatedAt,
	)
	return err
}

func (s *PostgresPaymentAttemptStore) UpdateAttemptStatus(ctx context.Context, bookingID string, status string) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_attempts
		SET status = $1, updated_at = $2
		WHERE booking_id = $3`,
		status, time.Now().UTC(), bookingID,
	)
	return err
}

func (s *PostgresPaymentAttemptStore) GetAttemptsByBookingID(ctx context.Context, bookingID string) ([]PaymentAttempt, error) {
	if s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, booking_id, provider, provider_reference, amount_minor, currency, status, created_at, updated_at
		FROM payment_attempts
		WHERE booking_id = $1
		ORDER BY created_at ASC`, bookingID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []PaymentAttempt
	for rows.Next() {
		var a PaymentAttempt
		if err := rows.Scan(&a.ID, &a.BookingID, &a.Provider, &a.ProviderReference, &a.AmountMinor, &a.Currency, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

