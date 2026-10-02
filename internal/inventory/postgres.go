package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan inventory.AvailabilityStore (port yang
// didefinisikan domain — adapter hexagonal §8).
type PostgresStore struct{ Pool *pgxpool.Pool }

// GetByDate membaca availability untuk [from, to). Baris yang hilang untuk
// tanggal tertentu menyebabkan ErrNotFound (pemanggil memutuskan artinya).
func (s *PostgresStore) GetByDate(ctx context.Context, roomTypeID string, from, to time.Time) ([]Availability, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT date, total_rooms, available_rooms
		FROM inventory
		WHERE room_type_id = $1 AND date >= $2 AND date < $3
		ORDER BY date ASC`,
		roomTypeID, from, to)
	if err != nil {
		return nil, fmt.Errorf("inventory: query: %w", err)
	}
	defer rows.Close()

	var out []Availability
	for rows.Next() {
		var a Availability
		var d time.Time
		if err := rows.Scan(&d, &a.TotalRooms, &a.AvailableRooms); err != nil {
			return nil, fmt.Errorf("inventory: scan: %w", err)
		}
		a.Date = d
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inventory: rows: %w", err)
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

// EnsureRows mengisi baris inventory yang hilang dengan 0 kamar — dipakai
// admin tooling saat menambah tipe kamar baru. Tidak dipakai di hot path.
func (s *PostgresStore) EnsureRows(ctx context.Context, roomTypeID string, from, to time.Time, total int) error {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
		SELECT $1, d::date, $2, $2
		FROM generate_series($3::date, $4::date - interval '1 day', interval '1 day') d
		ON CONFLICT (room_type_id, date) DO NOTHING`,
		roomTypeID, total, from, to)
	if err != nil {
		return fmt.Errorf("inventory: ensure rows: %w", err)
	}
	_ = tag
	return nil
}

var _ AvailabilityStore = (*PostgresStore)(nil)

// pgxErrNoRows re-export agar pemanggil bisa errors.Is tanpa import pgx.
var ErrNoRows = pgx.ErrNoRows
