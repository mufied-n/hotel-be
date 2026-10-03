package frontdesk

import (
	"context"
	"fmt"
	"math"
	"time"

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

// GetDailyRoster menghasilkan agregasi lengkap operasional meja depan untuk tanggal target.
func (s *PostgresStore) GetDailyRoster(ctx context.Context, targetDate time.Time) (*DailyRoster, error) {
	dateStr := targetDate.Format("2006-01-02")

	// 1. Agregasi Metrik Okupansi & Status Kamar
	metricsQuery := `
		SELECT
			COUNT(*) AS total_rooms,
			COUNT(*) FILTER (WHERE cleanliness_status = 'occupied') AS occupied_rooms,
			COUNT(*) FILTER (WHERE cleanliness_status = 'inspected') AS vacant_inspected_rooms,
			COUNT(*) FILTER (WHERE cleanliness_status = 'vacant_dirty') AS vacant_dirty_rooms,
			COUNT(*) FILTER (WHERE cleanliness_status = 'cleaning') AS cleaning_rooms,
			COUNT(*) FILTER (WHERE cleanliness_status = 'out_of_order') AS out_of_order_rooms
		FROM rooms;`

	var metrics RosterMetrics
	err := s.pool.QueryRow(ctx, metricsQuery).Scan(
		&metrics.TotalRooms,
		&metrics.OccupiedRooms,
		&metrics.VacantInspectedRooms,
		&metrics.VacantDirtyRooms,
		&metrics.CleaningRooms,
		&metrics.OutOfOrderRooms,
	)
	if err != nil {
		return nil, fmt.Errorf("frontdesk: aggregate metrics: %w", err)
	}

	metrics.SellableRooms = metrics.TotalRooms - metrics.OutOfOrderRooms
	if metrics.SellableRooms > 0 {
		rate := (float64(metrics.OccupiedRooms) / float64(metrics.SellableRooms)) * 100.0
		// Bulatkan ke 2 desimal
		metrics.OccupancyRatePercent = math.Round(rate*100) / 100
	}

	// 2. Daftar Expected Arrivals (Confirmed bookings with check_in = targetDate)
	arrivalsQuery := `
		SELECT 
			b.id, b.guest_name, COALESCE(b.guest_phone, ''), b.room_type_id,
			COALESCE(v.name, 'Room') AS room_type_name,
			b.num_rooms, b.num_guests,
			COALESCE(b.estimated_arrival_time, ''), COALESCE(b.special_requests, ''),
			b.total_price_minor,
			COALESCE(ARRAY_AGG(ra.room_number) FILTER (WHERE ra.room_number IS NOT NULL), '{}') AS assigned_rooms
		FROM bookings b
		LEFT JOIN room_types v ON b.room_type_id = v.id
		LEFT JOIN room_assignments ra ON b.id = ra.booking_id
		WHERE b.status = 'confirmed' AND b.check_in::date = $1::date
		GROUP BY b.id, v.name
		ORDER BY b.estimated_arrival_time ASC, b.guest_name ASC;`

	arrRows, err := s.pool.Query(ctx, arrivalsQuery, dateStr)
	if err != nil {
		return nil, fmt.Errorf("frontdesk: query arrivals: %w", err)
	}
	defer arrRows.Close()

	var arrivals []ExpectedArrivalItem
	for arrRows.Next() {
		var item ExpectedArrivalItem
		if err := arrRows.Scan(
			&item.BookingID,
			&item.GuestName,
			&item.GuestPhone,
			&item.RoomTypeID,
			&item.RoomTypeName,
			&item.NumRooms,
			&item.NumGuests,
			&item.EstimatedArrivalTime,
			&item.SpecialRequests,
			&item.TotalPriceMinor,
			&item.AssignedRooms,
		); err != nil {
			return nil, fmt.Errorf("frontdesk: scan arrival item: %w", err)
		}
		arrivals = append(arrivals, item)
	}
	if err := arrRows.Err(); err != nil {
		return nil, fmt.Errorf("frontdesk: arrivals rows: %w", err)
	}

	// 3. Daftar Expected Departures (CheckedIn bookings with check_out = targetDate)
	departuresQuery := `
		SELECT 
			b.id, b.guest_name,
			COALESCE(ARRAY_AGG(ra.room_number) FILTER (WHERE ra.room_number IS NOT NULL), '{}') AS room_numbers,
			to_char(b.check_in, 'YYYY-MM-DD'),
			to_char(b.check_out, 'YYYY-MM-DD')
		FROM bookings b
		LEFT JOIN room_assignments ra ON b.id = ra.booking_id
		WHERE b.status = 'checked_in' AND b.check_out::date = $1::date
		GROUP BY b.id
		ORDER BY b.guest_name ASC;`

	depRows, err := s.pool.Query(ctx, departuresQuery, dateStr)
	if err != nil {
		return nil, fmt.Errorf("frontdesk: query departures: %w", err)
	}
	defer depRows.Close()

	var departures []ExpectedDepartureItem
	for depRows.Next() {
		var item ExpectedDepartureItem
		if err := depRows.Scan(
			&item.BookingID,
			&item.GuestName,
			&item.RoomNumbers,
			&item.CheckInDate,
			&item.CheckOutDate,
		); err != nil {
			return nil, fmt.Errorf("frontdesk: scan departure item: %w", err)
		}
		departures = append(departures, item)
	}
	if err := depRows.Err(); err != nil {
		return nil, fmt.Errorf("frontdesk: departures rows: %w", err)
	}

	// 4. Hitung Jumlah In-House Guests pada tanggal operasional
	inHouseQuery := `
		SELECT COUNT(DISTINCT b.id)
		FROM bookings b
		WHERE b.status = 'checked_in'
		  AND b.check_in::date <= $1::date
		  AND b.check_out::date > $1::date;`

	var inHouseCount int
	if err := s.pool.QueryRow(ctx, inHouseQuery, dateStr).Scan(&inHouseCount); err != nil {
		return nil, fmt.Errorf("frontdesk: count in-house guests: %w", err)
	}

	return &DailyRoster{
		Date:               dateStr,
		Metrics:            metrics,
		ExpectedArrivals:   arrivals,
		ExpectedDepartures: departures,
		InHouseCount:       inHouseCount,
	}, nil
}

// CreateHandoverNote menyimpan catatan serah terima shift baru.
func (s *PostgresStore) CreateHandoverNote(ctx context.Context, note *HandoverNote) error {
	query := `
		INSERT INTO front_desk_handover_notes (
			shift, cash_float_minor, pending_issues, vip_guest_notes, actor_id, actor_role, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, created_at;`

	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}

	return s.pool.QueryRow(ctx, query,
		string(note.Shift),
		note.CashFloatMinor,
		note.PendingIssues,
		note.VIPGuestNotes,
		note.ActorID,
		note.ActorRole,
		note.CreatedAt,
	).Scan(&note.ID, &note.CreatedAt)
}

// ListHandoverNotes mengambil daftar catatan serah terima shift dengan limit dan offset.
func (s *PostgresStore) ListHandoverNotes(ctx context.Context, limit, offset int) ([]HandoverNote, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM front_desk_handover_notes;").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("frontdesk: count notes: %w", err)
	}

	query := `
		SELECT id::text, shift, cash_float_minor, pending_issues, vip_guest_notes, actor_id, actor_role, created_at
		FROM front_desk_handover_notes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2;`

	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("frontdesk: query notes: %w", err)
	}
	defer rows.Close()

	var notes []HandoverNote
	for rows.Next() {
		var n HandoverNote
		var shiftStr string
		if err := rows.Scan(
			&n.ID,
			&shiftStr,
			&n.CashFloatMinor,
			&n.PendingIssues,
			&n.VIPGuestNotes,
			&n.ActorID,
			&n.ActorRole,
			&n.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("frontdesk: scan note: %w", err)
		}
		n.Shift = ShiftType(shiftStr)
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("frontdesk: notes rows: %w", err)
	}

	return notes, total, nil
}
