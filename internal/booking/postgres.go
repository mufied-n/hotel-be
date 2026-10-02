package booking

import (
	"context"
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
	err := t.tx.QueryRow(ctx, `
		INSERT INTO bookings
			(room_type_id, check_in, check_out, num_rooms, num_guests,
			 status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`,
		b.RoomTypeID, b.CheckIn, b.CheckOut, b.NumRooms, b.NumGuests,
		string(b.Status), b.TotalPriceMinor, b.Currency, b.GuestName, b.GuestEmail, b.GuestToken, b.CreatedAt,
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
		       status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at
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
	return nil
}

// PickAndAssignRooms memilih kamar fisik bebas sejumlah count dan memasang assignment.
//   - NOT EXISTS: hindari kamar yang sudah ter-assign untuk rentang overlap.
//   - EXCLUDE USING GIST (di DB): backstop — bila dua check-in paralel lolos
//     NOT EXISTS bersamaan, database MENOLAK insert kedua dengan SQLSTATE
//     23P01 (exclusion_violation). Keduanya dipetakan ke ErrNoRoomAvailable.
func (t *txCtx) PickAndAssignRooms(ctx context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error) {
	rows, err := t.tx.Query(ctx, `
		WITH free_rooms AS (
			SELECT r.room_number
			FROM rooms r
			WHERE r.room_type_id = $1
			  AND NOT EXISTS (
				SELECT 1 FROM room_assignments ra
				WHERE ra.room_number = r.room_number
				  AND ra.stay_dates && daterange($2, $3, '[)')
			  )
			ORDER BY r.room_number
			LIMIT $4
		)
		INSERT INTO room_assignments (booking_id, room_number, stay_dates)
		SELECT $5, room_number, daterange($2, $3, '[)')
		FROM free_rooms
		RETURNING room_number`,
		roomTypeID, checkIn, checkOut, count, bookingID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" { // exclusion_violation
			return nil, ErrNoRoomAvailable
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
		return nil, fmt.Errorf("booking: assigned rows: %w", err)
	}

	if len(assigned) < count {
		return nil, ErrNoRoomAvailable
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
		       status, total_price_minor, currency, guest_name, guest_email, guest_token, created_at
		FROM bookings WHERE id = $1`, id))
}

type rowScanner interface{ Scan(dest ...any) error }

func scanBooking(row rowScanner) (Booking, error) {
	var b Booking
	var status string
	err := row.Scan(&b.ID, &b.RoomTypeID, &b.CheckIn, &b.CheckOut, &b.NumRooms,
		&b.NumGuests, &status, &b.TotalPriceMinor, &b.Currency,
		&b.GuestName, &b.GuestEmail, &b.GuestToken, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("booking: scan: %w", err)
	}
	b.Status = Status(status)
	return b, nil
}
