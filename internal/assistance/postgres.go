package assistance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan Store untuk PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CreateRequest menyimpan permintaan khusus baru ke database.
func (s *PostgresStore) CreateRequest(ctx context.Context, req SpecialRequest) (*SpecialRequest, error) {
	query := `
		INSERT INTO booking_special_requests (
			booking_id, category, department, description, target_time, status, staff_notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, booking_id, category, department, description, target_time, status, staff_notes, handled_by, handled_at, created_at, updated_at;
	`

	var res SpecialRequest
	var handledAt *time.Time
	err := s.pool.QueryRow(ctx, query,
		req.BookingID,
		string(req.Category),
		string(req.Department),
		req.Description,
		req.TargetTime,
		string(req.Status),
		req.StaffNotes,
		req.CreatedAt,
		req.UpdatedAt,
	).Scan(
		&res.ID,
		&res.BookingID,
		&res.Category,
		&res.Department,
		&res.Description,
		&res.TargetTime,
		&res.Status,
		&res.StaffNotes,
		&res.HandledBy,
		&handledAt,
		&res.CreatedAt,
		&res.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("assistance: insert request: %w", err)
	}
	res.HandledAt = handledAt
	return &res, nil
}

// GetRequestByID mengambil satu permintaan khusus berdasarkan UUID.
func (s *PostgresStore) GetRequestByID(ctx context.Context, id string) (*SpecialRequest, error) {
	query := `
		SELECT id, booking_id, category, department, description, target_time, status, staff_notes, handled_by, handled_at, created_at, updated_at
		FROM booking_special_requests
		WHERE id = $1;
	`

	var res SpecialRequest
	var handledAt *time.Time
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&res.ID,
		&res.BookingID,
		&res.Category,
		&res.Department,
		&res.Description,
		&res.TargetTime,
		&res.Status,
		&res.StaffNotes,
		&res.HandledBy,
		&handledAt,
		&res.CreatedAt,
		&res.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("assistance: get request by id: %w", err)
	}
	res.HandledAt = handledAt
	return &res, nil
}

// ListByBookingID menyajikan seluruh permintaan khusus untuk satu booking.
func (s *PostgresStore) ListByBookingID(ctx context.Context, bookingID string) ([]SpecialRequest, error) {
	query := `
		SELECT id, booking_id, category, department, description, target_time, status, staff_notes, handled_by, handled_at, created_at, updated_at
		FROM booking_special_requests
		WHERE booking_id = $1
		ORDER BY created_at ASC;
	`

	rows, err := s.pool.Query(ctx, query, bookingID)
	if err != nil {
		return nil, fmt.Errorf("assistance: list by booking id: %w", err)
	}
	defer rows.Close()

	items := make([]SpecialRequest, 0)
	for rows.Next() {
		var item SpecialRequest
		var handledAt *time.Time
		if err := rows.Scan(
			&item.ID,
			&item.BookingID,
			&item.Category,
			&item.Department,
			&item.Description,
			&item.TargetTime,
			&item.Status,
			&item.StaffNotes,
			&item.HandledBy,
			&handledAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("assistance: scan request: %w", err)
		}
		item.HandledAt = handledAt
		items = append(items, item)
	}

	return items, nil
}

// ListStaffQueue menyajikan antrean tugas staf yang terfilter menurut departemen, status, dan booking_id.
func (s *PostgresStore) ListStaffQueue(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error) {
	query := `
		SELECT 
			r.id, r.booking_id, r.category, r.department, r.description, r.target_time, r.status, r.staff_notes, r.handled_by, r.handled_at, r.created_at, r.updated_at,
			b.guest_name,
			COALESCE(ARRAY_AGG(ra.room_number) FILTER (WHERE ra.room_number IS NOT NULL), '{}') AS room_numbers,
			to_char(b.check_in, 'YYYY-MM-DD') AS check_in_date,
			to_char(b.check_out, 'YYYY-MM-DD') AS check_out_date
		FROM booking_special_requests r
		JOIN bookings b ON r.booking_id = b.id
		LEFT JOIN room_assignments ra ON b.id = ra.booking_id
		WHERE ($1 = '' OR r.department = $1)
		  AND ($2 = '' OR r.status = $2)
		  AND ($3 = '' OR r.booking_id = $3::uuid)
		GROUP BY r.id, b.guest_name, b.check_in, b.check_out
		ORDER BY r.created_at DESC;
	`

	rows, err := s.pool.Query(ctx, query, filter.Department, filter.Status, filter.BookingID)
	if err != nil {
		return nil, fmt.Errorf("assistance: list staff queue: %w", err)
	}
	defer rows.Close()

	items := make([]StaffQueueItem, 0)
	for rows.Next() {
		var item StaffQueueItem
		var handledAt *time.Time
		if err := rows.Scan(
			&item.ID,
			&item.BookingID,
			&item.Category,
			&item.Department,
			&item.Description,
			&item.TargetTime,
			&item.Status,
			&item.StaffNotes,
			&item.HandledBy,
			&handledAt,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.GuestName,
			&item.RoomNumbers,
			&item.CheckInDate,
			&item.CheckOutDate,
		); err != nil {
			return nil, fmt.Errorf("assistance: scan staff queue: %w", err)
		}
		item.HandledAt = handledAt
		items = append(items, item)
	}

	return items, nil
}

// UpdateStatus memperbarui status permintaan khusus, catatan staf, dan aktor penangan.
func (s *PostgresStore) UpdateStatus(ctx context.Context, reqID string, toStatus Status, notes string, handledBy string, handledAt time.Time) (*SpecialRequest, error) {
	query := `
		UPDATE booking_special_requests
		SET status = $2,
		    staff_notes = $3,
		    handled_by = $4,
		    handled_at = $5,
		    updated_at = $5
		WHERE id = $1
		RETURNING id, booking_id, category, department, description, target_time, status, staff_notes, handled_by, handled_at, created_at, updated_at;
	`

	var res SpecialRequest
	var scannedHandledAt *time.Time
	err := s.pool.QueryRow(ctx, query, reqID, string(toStatus), notes, handledBy, handledAt).Scan(
		&res.ID,
		&res.BookingID,
		&res.Category,
		&res.Department,
		&res.Description,
		&res.TargetTime,
		&res.Status,
		&res.StaffNotes,
		&res.HandledBy,
		&scannedHandledAt,
		&res.CreatedAt,
		&res.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRequestNotFound
		}
		return nil, fmt.Errorf("assistance: update status: %w", err)
	}
	res.HandledAt = scannedHandledAt
	return &res, nil
}

// GetBookingOwner mengambil email pemilik booking untuk verifikasi Anti-IDOR.
func (s *PostgresStore) GetBookingOwner(ctx context.Context, bookingID string) (guestEmail string, exists bool, err error) {
	query := `SELECT guest_email FROM bookings WHERE id = $1;`
	err = s.pool.QueryRow(ctx, query, bookingID).Scan(&guestEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("assistance: get booking owner: %w", err)
	}
	return guestEmail, true, nil
}
