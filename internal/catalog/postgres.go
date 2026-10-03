package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB adalah interface minimal untuk query PostgreSQL (kompatibel dengan *pgxpool.Pool).
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// PostgresStore mengimplementasikan Store untuk PostgreSQL.
type PostgresStore struct {
	db DB
}

// NewPostgresStore membuat instance PostgresStore baru.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: pool}
}

// ListVariants mengambil seluruh daftar varian kamar jual terurut dari harga termurah.
func (s *PostgresStore) ListVariants(ctx context.Context) ([]RoomVariant, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, code, name, family_name, bed_type, room_size_sqm,
		       max_capacity, max_adults, max_children, base_price_minor,
		       description, amenities, photos
		FROM room_types
		ORDER BY base_price_minor ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog list query: %w", err)
	}
	defer rows.Close()

	var list []RoomVariant
	for rows.Next() {
		var v RoomVariant
		var amenitiesJSON, photosJSON []byte
		if err := rows.Scan(
			&v.ID, &v.Code, &v.Name, &v.FamilyName, &v.BedType, &v.RoomSizeSqm,
			&v.MaxCapacity, &v.MaxAdults, &v.MaxChildren, &v.BasePriceMinor,
			&v.Description, &amenitiesJSON, &photosJSON,
		); err != nil {
			return nil, fmt.Errorf("catalog scan: %w", err)
		}
		if len(amenitiesJSON) > 0 {
			_ = json.Unmarshal(amenitiesJSON, &v.Amenities)
		}
		if len(photosJSON) > 0 {
			_ = json.Unmarshal(photosJSON, &v.Photos)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// GetVariant mengambil satu varian kamar berdasarkan ID UUID atau slug code (misal: 'sup-king').
func (s *PostgresStore) GetVariant(ctx context.Context, idOrCode string) (RoomVariant, error) {
	var v RoomVariant
	var amenitiesJSON, photosJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, code, name, family_name, bed_type, room_size_sqm,
		       max_capacity, max_adults, max_children, base_price_minor,
		       description, amenities, photos
		FROM room_types
		WHERE id::text = $1 OR code = $1 OR ($1 = 'std' AND code = 'sup-king')
		LIMIT 1
	`, idOrCode).Scan(
		&v.ID, &v.Code, &v.Name, &v.FamilyName, &v.BedType, &v.RoomSizeSqm,
		&v.MaxCapacity, &v.MaxAdults, &v.MaxChildren, &v.BasePriceMinor,
		&v.Description, &amenitiesJSON, &photosJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoomVariant{}, ErrVariantNotFound
	}
	if err != nil {
		return RoomVariant{}, fmt.Errorf("catalog get: %w", err)
	}
	if len(amenitiesJSON) > 0 {
		_ = json.Unmarshal(amenitiesJSON, &v.Amenities)
	}
	if len(photosJSON) > 0 {
		_ = json.Unmarshal(photosJSON, &v.Photos)
	}
	return v, nil
}

// CreateVariant menyisipkan varian kamar baru ke tabel room_types.
func (s *PostgresStore) CreateVariant(ctx context.Context, v RoomVariant) (RoomVariant, error) {
	if strings.TrimSpace(v.Code) == "" || strings.TrimSpace(v.Name) == "" || v.MaxCapacity < 1 || v.BasePriceMinor <= 0 {
		return RoomVariant{}, ErrInvalidVariant
	}
	if v.ID == "" {
		v.ID = uuid.NewV7().String()
	}
	if v.Amenities == nil {
		v.Amenities = []string{}
	}
	if v.Photos == nil {
		v.Photos = []Photo{}
	}

	amenitiesJSON, err := json.Marshal(v.Amenities)
	if err != nil {
		return RoomVariant{}, fmt.Errorf("amenities marshal: %w", err)
	}
	photosJSON, err := json.Marshal(v.Photos)
	if err != nil {
		return RoomVariant{}, fmt.Errorf("photos marshal: %w", err)
	}

	var insertedID string
	err = s.db.QueryRow(ctx, `
		INSERT INTO room_types (
			id, code, name, family_name, bed_type, room_size_sqm,
			max_capacity, max_adults, max_children, base_price_minor,
			description, amenities, photos
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id::text
	`, v.ID, v.Code, v.Name, v.FamilyName, v.BedType, v.RoomSizeSqm,
		v.MaxCapacity, v.MaxAdults, v.MaxChildren, v.BasePriceMinor,
		v.Description, amenitiesJSON, photosJSON).Scan(&insertedID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return RoomVariant{}, ErrDuplicateCode
	}
	if err != nil {
		return RoomVariant{}, fmt.Errorf("catalog insert: %w", err)
	}
	v.ID = insertedID
	return v, nil
}

// UpdateVariant memperbarui varian kamar yang ada berdasarkan ID UUID atau slug code.
func (s *PostgresStore) UpdateVariant(ctx context.Context, id string, v RoomVariant) (RoomVariant, error) {
	if strings.TrimSpace(v.Code) == "" || strings.TrimSpace(v.Name) == "" || v.MaxCapacity < 1 || v.BasePriceMinor <= 0 {
		return RoomVariant{}, ErrInvalidVariant
	}
	if v.Amenities == nil {
		v.Amenities = []string{}
	}
	if v.Photos == nil {
		v.Photos = []Photo{}
	}

	amenitiesJSON, err := json.Marshal(v.Amenities)
	if err != nil {
		return RoomVariant{}, fmt.Errorf("amenities marshal: %w", err)
	}
	photosJSON, err := json.Marshal(v.Photos)
	if err != nil {
		return RoomVariant{}, fmt.Errorf("photos marshal: %w", err)
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE room_types SET
			code = $2, name = $3, family_name = $4, bed_type = $5, room_size_sqm = $6,
			max_capacity = $7, max_adults = $8, max_children = $9, base_price_minor = $10,
			description = $11, amenities = $12, photos = $13
		WHERE id::text = $1 OR code = $1
	`, id, v.Code, v.Name, v.FamilyName, v.BedType, v.RoomSizeSqm,
		v.MaxCapacity, v.MaxAdults, v.MaxChildren, v.BasePriceMinor,
		v.Description, amenitiesJSON, photosJSON)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return RoomVariant{}, ErrDuplicateCode
	}
	if err != nil {
		return RoomVariant{}, fmt.Errorf("catalog update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return RoomVariant{}, ErrVariantNotFound
	}
	return s.GetVariant(ctx, id)
}

// DeleteVariant menghapus varian kamar jika tidak memiliki keterikatan fisik atau pemesanan aktif.
func (s *PostgresStore) DeleteVariant(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM room_types WHERE id::text = $1 OR code = $1`, id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrCannotDelete
	}
	if err != nil {
		return fmt.Errorf("catalog delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVariantNotFound
	}
	return nil
}
