package stay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan Store menggunakan PostgreSQL pool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// GetBooking mengambil data esensial booking untuk proses perpanjangan atau pemindahan.
func (s *PostgresStore) GetBooking(ctx context.Context, bookingID string) (*BookingDetails, error) {
	query := `
		SELECT b.id, b.status, b.room_type_id, b.check_in, b.check_out, b.num_rooms, b.total_price_minor,
		       COALESCE(
		           (SELECT ra.room_number 
		            FROM room_assignments ra 
		            WHERE ra.booking_id = b.id 
		            ORDER BY upper(ra.stay_dates) DESC 
		            LIMIT 1), ''
		       ) AS current_room
		FROM bookings b
		WHERE b.id = $1`

	var d BookingDetails
	err := s.pool.QueryRow(ctx, query, bookingID).Scan(
		&d.ID, &d.Status, &d.RoomTypeID, &d.CheckIn, &d.CheckOut, &d.NumRooms, &d.TotalPriceMinor, &d.CurrentRoom,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("stay: get booking: %w", err)
	}
	return &d, nil
}

// MoveRoom memindahkan kamar tamu aktif secara atomik di database.
func (s *PostgresStore) MoveRoom(ctx context.Context, input RoomMoveInput, moveDate time.Time) (*RoomMoveResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("stay: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Kunci data reservasi dan verifikasi status
	var status string
	var checkIn, checkOut time.Time
	err = tx.QueryRow(ctx, `
		SELECT status, check_in, check_out 
		FROM bookings 
		WHERE id = $1 
		FOR UPDATE`, input.BookingID).Scan(&status, &checkIn, &checkOut)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("stay: lock booking: %w", err)
	}

	if status != "checked_in" {
		return nil, ErrInvalidBookingStatus
	}

	// 2. Ambil penugasan kamar saat ini
	var currentRoom string
	var stayDatesStr string
	err = tx.QueryRow(ctx, `
		SELECT room_number, stay_dates::text 
		FROM room_assignments 
		WHERE booking_id = $1 
		ORDER BY upper(stay_dates) DESC 
		LIMIT 1 
		FOR UPDATE`, input.BookingID).Scan(&currentRoom, &stayDatesStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("stay: no room currently assigned to booking")
		}
		return nil, fmt.Errorf("stay: get current assignment: %w", err)
	}

	if currentRoom == input.TargetRoomNumber {
		return nil, ErrSameRoomMove
	}

	// 3. Verifikasi status kebersihan kamar target (Wajib 'inspected')
	var targetCleanliness string
	err = tx.QueryRow(ctx, `
		SELECT cleanliness_status 
		FROM rooms 
		WHERE room_number = $1 
		FOR UPDATE`, input.TargetRoomNumber).Scan(&targetCleanliness)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("stay: target room %s does not exist", input.TargetRoomNumber)
		}
		return nil, fmt.Errorf("stay: verify target room: %w", err)
	}

	if targetCleanliness != "inspected" {
		return nil, ErrTargetRoomNotReady
	}

	moveDateClean := time.Date(moveDate.Year(), moveDate.Month(), moveDate.Day(), 0, 0, 0, 0, time.UTC)
	checkInClean := time.Date(checkIn.Year(), checkIn.Month(), checkIn.Day(), 0, 0, 0, 0, time.UTC)
	checkOutClean := time.Date(checkOut.Year(), checkOut.Month(), checkOut.Day(), 0, 0, 0, 0, time.UTC)

	// 4. Potong durasi kamar lama
	if !moveDateClean.After(checkInClean) {
		// Tamu pindah pada hari yang sama dengan check-in
		_, err = tx.Exec(ctx, `
			DELETE FROM room_assignments 
			WHERE booking_id = $1 AND room_number = $2`, input.BookingID, currentRoom)
		if err != nil {
			return nil, fmt.Errorf("stay: remove initial assignment: %w", err)
		}
	} else {
		// Tamu sudah menginap beberapa malam di kamar lama
		_, err = tx.Exec(ctx, `
			UPDATE room_assignments 
			SET stay_dates = daterange(lower(stay_dates), $2::date, '[)')
			WHERE booking_id = $1 AND room_number = $3`,
			input.BookingID, moveDateClean.Format("2006-01-02"), currentRoom)
		if err != nil {
			return nil, fmt.Errorf("stay: trim old assignment: %w", err)
		}
	}

	// 5. Masukkan penugasan kamar baru
	_, err = tx.Exec(ctx, `
		INSERT INTO room_assignments (booking_id, room_number, stay_dates) 
		VALUES ($1, $2, daterange($3::date, $4::date, '[)'))`,
		input.BookingID, input.TargetRoomNumber, moveDateClean.Format("2006-01-02"), checkOutClean.Format("2006-01-02"))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40001") {
			return nil, ErrRoomPhysicalOverlap
		}
		return nil, fmt.Errorf("stay: assign new room: %w", err)
	}

	// 6. Transisi status kamar: kamar lama -> vacant_dirty, kamar baru -> occupied
	actor := input.ActorID
	if actor == "" {
		actor = "front_desk"
	}

	_, err = tx.Exec(ctx, `
		UPDATE rooms 
		SET cleanliness_status = 'vacant_dirty', 
		    maintenance_notes = $1, 
		    updated_by = $2, 
		    updated_at = now() 
		WHERE room_number = $3`,
		fmt.Sprintf("Room moved to %s: %s", input.TargetRoomNumber, input.Notes), actor, currentRoom)
	if err != nil {
		return nil, fmt.Errorf("stay: update old room status: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE rooms 
		SET cleanliness_status = 'occupied', 
		    updated_by = $1, 
		    updated_at = now() 
		WHERE room_number = $2`,
		actor, input.TargetRoomNumber)
	if err != nil {
		return nil, fmt.Errorf("stay: update new room status: %w", err)
	}

	// 7. Catat log audit ke room_move_logs
	_, err = tx.Exec(ctx, `
		INSERT INTO room_move_logs (booking_id, from_room_number, to_room_number, move_date, reason_category, notes, actor_id) 
		VALUES ($1, $2, $3, $4::date, $5, $6, $7)`,
		input.BookingID, currentRoom, input.TargetRoomNumber, moveDateClean.Format("2006-01-02"),
		input.ReasonCategory, input.Notes, actor)
	if err != nil {
		return nil, fmt.Errorf("stay: record room move log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("stay: commit room move: %w", err)
	}

	return &RoomMoveResult{
		Status:             "ok",
		BookingID:          input.BookingID,
		PreviousRoomNumber: currentRoom,
		NewRoomNumber:      input.TargetRoomNumber,
		MoveDate:           moveDateClean.Format("2006-01-02"),
		Message:            fmt.Sprintf("pemindahan kamar berhasil; kamar %s telah ditandai vacant_dirty", currentRoom),
	}, nil
}

// ExtendStay memperpanjang tanggal menginap, mengurangi kuota inventaris, dan menambahkan malam reservasi.
func (s *PostgresStore) ExtendStay(
	ctx context.Context,
	bookingID string,
	additionalNights int,
	newCheckOut time.Time,
	additionalRates []int64,
	additionalTotal int64,
) (*ExtendStayResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("stay: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Kunci booking
	var status, roomTypeID string
	var checkIn, oldCheckOut time.Time
	var numRooms int
	var oldTotalPrice int64
	err = tx.QueryRow(ctx, `
		SELECT status, room_type_id, check_in, check_out, num_rooms, total_price_minor 
		FROM bookings 
		WHERE id = $1 
		FOR UPDATE`, bookingID).Scan(&status, &roomTypeID, &checkIn, &oldCheckOut, &numRooms, &oldTotalPrice)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("stay: lock booking for extension: %w", err)
	}

	if status != "checked_in" && status != "confirmed" {
		return nil, ErrInvalidBookingStatus
	}

	oldCheckOutClean := time.Date(oldCheckOut.Year(), oldCheckOut.Month(), oldCheckOut.Day(), 0, 0, 0, 0, time.UTC)
	newCheckOutClean := time.Date(newCheckOut.Year(), newCheckOut.Month(), newCheckOut.Day(), 0, 0, 0, 0, time.UTC)

	// 2. Potong kuota inventaris untuk malam-malam perpanjangan
	for d := oldCheckOutClean; d.Before(newCheckOutClean); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		cmd, err := tx.Exec(ctx, `
			UPDATE inventory 
			SET available_rooms = available_rooms - $1 
			WHERE room_type_id = $2 
			  AND date = $3::date 
			  AND available_rooms >= $1`,
			numRooms, roomTypeID, dateStr)
		if err != nil {
			return nil, fmt.Errorf("stay: decrement inventory: %w", err)
		}
		if cmd.RowsAffected() == 0 {
			return nil, ErrNoAvailabilityForExtension
		}
	}

	// 3. Periksa dan perpanjang penugasan kamar fisik jika booking sudah check-in
	if status == "checked_in" {
		var currentRoom string
		err = tx.QueryRow(ctx, `
			SELECT room_number 
			FROM room_assignments 
			WHERE booking_id = $1 
			ORDER BY upper(stay_dates) DESC 
			LIMIT 1 
			FOR UPDATE`, bookingID).Scan(&currentRoom)
		if err == nil && currentRoom != "" {
			// Periksa apakah kamar fisik bebas di rentang tanggal perpanjangan
			var overlapCount int
			_ = tx.QueryRow(ctx, `
				SELECT COUNT(*) 
				FROM room_assignments 
				WHERE room_number = $1 
				  AND booking_id != $2 
				  AND stay_dates && daterange($3::date, $4::date, '[)')`,
				currentRoom, bookingID, oldCheckOutClean.Format("2006-01-02"), newCheckOutClean.Format("2006-01-02")).Scan(&overlapCount)
			if overlapCount > 0 {
				return nil, ErrRoomPhysicalOverlap
			}

			// Perpanjang stay_dates pada penugasan kamar fisik aktif
			_, err = tx.Exec(ctx, `
				UPDATE room_assignments 
				SET stay_dates = daterange(lower(stay_dates), $2::date, '[)') 
				WHERE booking_id = $1 AND room_number = $3`,
				bookingID, newCheckOutClean.Format("2006-01-02"), currentRoom)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40001") {
					return nil, ErrRoomPhysicalOverlap
				}
				return nil, fmt.Errorf("stay: extend room assignment: %w", err)
			}
		}
	}

	// 4. Update data check-out dan total tagihan pada tabel bookings
	newTotalPrice := oldTotalPrice + additionalTotal
	_, err = tx.Exec(ctx, `
		UPDATE bookings 
		SET check_out = $1, total_price_minor = $2, updated_at = now() 
		WHERE id = $3`,
		newCheckOutClean, newTotalPrice, bookingID)
	if err != nil {
		return nil, fmt.Errorf("stay: update booking: %w", err)
	}

	// 5. Tambahkan malam-malam baru ke reservation_room_nights
	rateIdx := 0
	for d := oldCheckOutClean; d.Before(newCheckOutClean); d = d.AddDate(0, 0, 1) {
		var nightRate int64
		if rateIdx < len(additionalRates) {
			nightRate = additionalRates[rateIdx]
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO reservation_room_nights (booking_id, night_date, rate_minor) 
			VALUES ($1, $2::date, $3)
			ON CONFLICT (booking_id, night_date) DO UPDATE SET rate_minor = EXCLUDED.rate_minor`,
			bookingID, d.Format("2006-01-02"), nightRate)
		if err != nil {
			return nil, fmt.Errorf("stay: insert reservation night: %w", err)
		}
		rateIdx++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("stay: commit extension: %w", err)
	}

	return &ExtendStayResult{
		Status:                "ok",
		BookingID:             bookingID,
		PreviousCheckOut:      oldCheckOutClean.Format("2006-01-02"),
		NewCheckOut:           newCheckOutClean.Format("2006-01-02"),
		AdditionalNights:      additionalNights,
		AdditionalAmountMinor: additionalTotal,
		NewTotalPriceMinor:    newTotalPrice,
		PaymentStatus:         "settled",
	}, nil
}

// ListRoomMoves mengambil riwayat perpindahan kamar untuk sebuah booking.
func (s *PostgresStore) ListRoomMoves(ctx context.Context, bookingID string) ([]RoomMoveLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, booking_id, from_room_number, to_room_number, 
		       to_char(move_date, 'YYYY-MM-DD'), reason_category, notes, actor_id, created_at 
		FROM room_move_logs 
		WHERE booking_id = $1 
		ORDER BY created_at DESC`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("stay: list room moves: %w", err)
	}
	defer rows.Close()

	var logs []RoomMoveLog
	for rows.Next() {
		var l RoomMoveLog
		if err := rows.Scan(
			&l.ID, &l.BookingID, &l.FromRoomNumber, &l.ToRoomNumber,
			&l.MoveDate, &l.ReasonCategory, &l.Notes, &l.ActorID, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("stay: scan room move log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
