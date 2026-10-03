package housekeeping

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan Store menggunakan pgxpool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) GetRoom(ctx context.Context, roomNumber string) (*RoomOperationalView, error) {
	query := `
		SELECT r.room_number, r.room_type_id::text, rt.name,
		       COALESCE(NULLIF(SUBSTRING(r.room_number FROM '^[0-9]'), '')::INT, 1) AS floor,
		       r.cleanliness_status, r.maintenance_notes, r.updated_by, r.updated_at,
		       b.id::text, b.guest_name
		FROM rooms r
		JOIN room_types rt ON r.room_type_id = rt.id
		LEFT JOIN room_assignments ra ON ra.room_number = r.room_number AND CURRENT_DATE <@ ra.stay_dates
		LEFT JOIN bookings b ON ra.booking_id = b.id AND b.status = 'checked_in'
		WHERE r.room_number = $1;
	`
	var (
		view   RoomOperationalView
		bID    sql.NullString
		gName  sql.NullString
		status string
	)
	err := s.pool.QueryRow(ctx, query, roomNumber).Scan(
		&view.RoomNumber, &view.RoomTypeID, &view.RoomTypeName,
		&view.Floor, &status, &view.MaintenanceNotes,
		&view.UpdatedBy, &view.UpdatedAt,
		&bID, &gName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("hk_store.get_room: %w", err)
	}
	view.CleanlinessStatus = CleanlinessStatus(status)
	if bID.Valid {
		view.CurrentBookingID = &bID.String
	}
	if gName.Valid {
		view.GuestName = &gName.String
	}

	return &view, nil
}

func (s *PostgresStore) UpdateRoomCleanliness(ctx context.Context, roomNumber string, to CleanlinessStatus, notes, actorID string) error {
	query := `
		UPDATE rooms
		SET cleanliness_status = $2,
		    maintenance_notes = CASE WHEN $3 = '' THEN maintenance_notes ELSE $3 END,
		    updated_by = $4,
		    updated_at = NOW()
		WHERE room_number = $1;
	`
	res, err := s.pool.Exec(ctx, query, roomNumber, string(to), notes, actorID)
	if err != nil {
		return fmt.Errorf("hk_store.update_room: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrRoomNotFound
	}
	return nil
}

func (s *PostgresStore) ListRooms(ctx context.Context, floor int, status string, roomTypeID string) ([]RoomOperationalView, error) {
	query := `
		SELECT r.room_number, r.room_type_id::text, rt.name,
		       COALESCE(NULLIF(SUBSTRING(r.room_number FROM '^[0-9]'), '')::INT, 1) AS floor,
		       r.cleanliness_status, r.maintenance_notes, r.updated_by, r.updated_at,
		       b.id::text, b.guest_name
		FROM rooms r
		JOIN room_types rt ON r.room_type_id = rt.id
		LEFT JOIN room_assignments ra ON ra.room_number = r.room_number AND CURRENT_DATE <@ ra.stay_dates
		LEFT JOIN bookings b ON ra.booking_id = b.id AND b.status = 'checked_in'
		WHERE ($1 = 0 OR COALESCE(NULLIF(SUBSTRING(r.room_number FROM '^[0-9]'), '')::INT, 1) = $1)
		  AND ($2 = '' OR r.cleanliness_status = $2)
		  AND ($3 = '' OR r.room_type_id::text = $3)
		ORDER BY r.room_number ASC;
	`
	rows, err := s.pool.Query(ctx, query, floor, status, roomTypeID)
	if err != nil {
		return nil, fmt.Errorf("hk_store.list_rooms: %w", err)
	}
	defer rows.Close()

	var results []RoomOperationalView
	for rows.Next() {
		var (
			view       RoomOperationalView
			bID        sql.NullString
			gName      sql.NullString
			statusText string
		)
		if err := rows.Scan(
			&view.RoomNumber, &view.RoomTypeID, &view.RoomTypeName,
			&view.Floor, &statusText, &view.MaintenanceNotes,
			&view.UpdatedBy, &view.UpdatedAt,
			&bID, &gName,
		); err != nil {
			return nil, fmt.Errorf("hk_store.scan_room: %w", err)
		}
		view.CleanlinessStatus = CleanlinessStatus(statusText)
		if bID.Valid {
			view.CurrentBookingID = &bID.String
		}
		if gName.Valid {
			view.GuestName = &gName.String
		}
		results = append(results, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hk_store.rows_err: %w", err)
	}
	if results == nil {
		results = []RoomOperationalView{}
	}
	return results, nil
}

func (s *PostgresStore) DeductInventoryForOOO(ctx context.Context, roomTypeID string, startDate, endDate time.Time) error {
	query := `
		UPDATE inventory
		SET available_rooms = GREATEST(available_rooms - 1, 0),
		    total_rooms = GREATEST(total_rooms - 1, 0)
		WHERE room_type_id = $1::uuid
		  AND date >= $2 AND date < $3;
	`
	_, err := s.pool.Exec(ctx, query, roomTypeID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("hk_store.deduct_inventory: %w", err)
	}
	return nil
}

var _ Store = (*PostgresStore)(nil)
