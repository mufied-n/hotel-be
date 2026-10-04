package rates

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// POSTGRES CALENDAR STORE (Rate Calendar & Restrictions)
// ============================================================================

// PostgresCalendarStore mengimplementasikan RateCalendarStore dengan pgxpool.
type PostgresCalendarStore struct {
	pool *pgxpool.Pool
}

// NewPostgresCalendarStore membuat instance baru PostgresCalendarStore.
func NewPostgresCalendarStore(pool *pgxpool.Pool) *PostgresCalendarStore {
	return &PostgresCalendarStore{pool: pool}
}

// GetCalendar mengambil kalender tarif dan restriksi untuk rentang tanggal tertentu.
// Menggunakan generate_series agar setiap tanggal memiliki representasi (fallback ke base price katalog jika tidak ada override).
func (s *PostgresCalendarStore) GetCalendar(ctx context.Context, start, end time.Time, planCode string, roomTypeID *string) ([]CalendarOverride, error) {
	if planCode == "" {
		planCode = "RO"
	}

	query := `
		SELECT 
			d.day::date AS cal_date,
			rt.id AS room_type_id,
			rt.name AS room_type_name,
			$3::varchar AS rate_plan,
			rt.base_price_minor AS base_price_idr,
			rco.price_override_idr,
			COALESCE(rco.price_override_idr, rt.base_price_minor) AS effective_price,
			COALESCE(rco.is_stop_sell, FALSE) AS is_stop_sell,
			COALESCE(rco.is_cta, FALSE) AS is_cta,
			COALESCE(rco.is_ctd, FALSE) AS is_ctd,
			COALESCE(rco.min_los, 1) AS min_los,
			COALESCE(rco.max_los, 30) AS max_los,
			rco.allotment_limit
		FROM generate_series($1::date, $2::date, interval '1 day') AS d(day)
		CROSS JOIN room_types rt
		LEFT JOIN rate_calendar_overrides rco 
			ON rco.room_type_id = rt.id 
		   AND rco.date = d.day::date 
		   AND rco.rate_plan_code = $3
		WHERE ($4::uuid IS NULL OR rt.id = $4::uuid)
		ORDER BY d.day, rt.name;
	`

	rows, err := s.pool.Query(ctx, query, start, end, planCode, roomTypeID)
	if err != nil {
		return nil, fmt.Errorf("postgres_calendar.GetCalendar: %w", err)
	}
	defer rows.Close()

	var results []CalendarOverride
	for rows.Next() {
		var item CalendarOverride
		var priceOverride *int64
		var allotment *int
		err := rows.Scan(
			&item.Date,
			&item.RoomTypeID,
			&item.RoomTypeName,
			&item.RatePlanCode,
			&item.BasePriceIDR,
			&priceOverride,
			&item.EffectivePrice,
			&item.IsStopSell,
			&item.IsCTA,
			&item.IsCTD,
			&item.MinLOS,
			&item.MaxLOS,
			&allotment,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres_calendar.Scan: %w", err)
		}
		item.PriceOverrideIDR = priceOverride
		item.AllotmentLimit = allotment
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres_calendar.RowsErr: %w", err)
	}

	return results, nil
}

// BulkUpsertOverrides menyimpan atau memperbarui aturan kalender secara atomik menggunakan pgx.Batch pipeline.
func (s *PostgresCalendarStore) BulkUpsertOverrides(ctx context.Context, req BulkCalendarUpdateRequest) (int64, error) {
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return 0, fmt.Errorf("invalid start_date format (expected YYYY-MM-DD): %w", err)
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return 0, fmt.Errorf("invalid end_date format (expected YYYY-MM-DD): %w", err)
	}
	if endDate.Before(startDate) {
		return 0, fmt.Errorf("end_date must be greater than or equal to start_date")
	}

	if req.RatePlanCode == "" {
		req.RatePlanCode = "RO"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO rate_calendar_overrides (
			room_type_id, rate_plan_code, date, price_override_idr,
			is_stop_sell, is_cta, is_ctd, min_los, max_los, allotment_limit, updated_at
		) VALUES (
			$1, $2, $3, $4,
			COALESCE($5, FALSE), COALESCE($6, FALSE), COALESCE($7, FALSE),
			COALESCE($8, 1), COALESCE($9, 30), $10, NOW()
		)
		ON CONFLICT (room_type_id, date, rate_plan_code) DO UPDATE SET
			price_override_idr = COALESCE(EXCLUDED.price_override_idr, rate_calendar_overrides.price_override_idr),
			is_stop_sell = COALESCE($5, rate_calendar_overrides.is_stop_sell),
			is_cta = COALESCE($6, rate_calendar_overrides.is_cta),
			is_ctd = COALESCE($7, rate_calendar_overrides.is_ctd),
			min_los = COALESCE($8, rate_calendar_overrides.min_los),
			max_los = COALESCE($9, rate_calendar_overrides.max_los),
			allotment_limit = COALESCE($10, rate_calendar_overrides.allotment_limit),
			updated_at = NOW();
	`

	batch := &pgx.Batch{}
	for _, roomID := range req.RoomTypeIDs {
		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			batch.Queue(query,
				roomID, req.RatePlanCode, d, req.PriceOverrideIDR,
				req.IsStopSell, req.IsCTA, req.IsCTD,
				req.MinLOS, req.MaxLOS, req.AllotmentLimit,
			)
		}
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	var affected int64
	for i := 0; i < batch.Len(); i++ {
		tag, err := br.Exec()
		if err != nil {
			return 0, fmt.Errorf("upsert override batch item %d: %w", i, err)
		}
		affected += tag.RowsAffected()
	}
	if err := br.Close(); err != nil {
		return 0, fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return affected, nil
}

// ============================================================================
// POSTGRES PROMO STORE (Dynamic Promo Campaigns)
// ============================================================================

const promoCampaignColumns = `id, code, name, discount_type, discount_value, max_discount_idr,
	min_stay_nights, quota_total, quota_used, valid_from, valid_to,
	COALESCE(applicable_room_types, '{}'), is_active, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPromoCampaign(scanner rowScanner) (PromoCampaign, error) {
	var p PromoCampaign
	var maxDiscount *int64
	var roomTypes []string

	err := scanner.Scan(
		&p.ID,
		&p.Code,
		&p.Name,
		&p.DiscountType,
		&p.DiscountValue,
		&maxDiscount,
		&p.MinStayNights,
		&p.QuotaTotal,
		&p.QuotaUsed,
		&p.ValidFrom,
		&p.ValidTo,
		&roomTypes,
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return PromoCampaign{}, err
	}

	p.MaxDiscountIDR = maxDiscount
	p.ApplicableRoomTypes = roomTypes
	return p, nil
}

// PostgresPromoStore mengimplementasikan PromoStore dengan pgxpool.
type PostgresPromoStore struct {
	pool *pgxpool.Pool
}

// NewPostgresPromoStore membuat instance baru PostgresPromoStore.
func NewPostgresPromoStore(pool *pgxpool.Pool) *PostgresPromoStore {
	return &PostgresPromoStore{pool: pool}
}

// GetByCode mengambil kampanye promo berdasarkan kodenya (case-insensitive).
func (s *PostgresPromoStore) GetByCode(ctx context.Context, code string) (PromoCampaign, error) {
	code = strings.TrimSpace(strings.ToUpper(code))
	query := `SELECT ` + promoCampaignColumns + ` FROM promo_campaigns WHERE UPPER(code) = $1;`
	row := s.pool.QueryRow(ctx, query, code)
	p, err := scanPromoCampaign(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return PromoCampaign{}, ErrPromoNotFound
		}
		return PromoCampaign{}, fmt.Errorf("postgres_promo.GetByCode: %w", err)
	}
	return p, nil
}

// ListCampaigns mengambil seluruh daftar promo.
func (s *PostgresPromoStore) ListCampaigns(ctx context.Context) ([]PromoCampaign, error) {
	query := `SELECT ` + promoCampaignColumns + ` FROM promo_campaigns ORDER BY created_at DESC;`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres_promo.ListCampaigns: %w", err)
	}
	defer rows.Close()

	var list []PromoCampaign
	for rows.Next() {
		p, err := scanPromoCampaign(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres_promo.Scan: %w", err)
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres_promo.RowsErr: %w", err)
	}
	return list, nil
}

// CreateCampaign menyimpan promo campaign baru.
func (s *PostgresPromoStore) CreateCampaign(ctx context.Context, p PromoCampaign) (string, error) {
	query := `
		INSERT INTO promo_campaigns (
			code, name, discount_type, discount_value, max_discount_idr,
			min_stay_nights, quota_total, quota_used, valid_from, valid_to,
			applicable_room_types, is_active
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, 0, $8, $9,
			$10, $11
		) RETURNING id;
	`
	var id string
	code := strings.TrimSpace(strings.ToUpper(p.Code))
	err := s.pool.QueryRow(ctx, query,
		code, p.Name, p.DiscountType, p.DiscountValue, p.MaxDiscountIDR,
		p.MinStayNights, p.QuotaTotal, p.ValidFrom, p.ValidTo,
		p.ApplicableRoomTypes, p.IsActive,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("postgres_promo.CreateCampaign: %w", err)
	}
	return id, nil
}

// UpdateCampaign memperbarui status aktif atau kuota promo.
func (s *PostgresPromoStore) UpdateCampaign(ctx context.Context, id string, isActive bool, quotaTotal int) error {
	query := `
		UPDATE promo_campaigns
		SET is_active = $1, quota_total = GREATEST(quota_used, $2), updated_at = NOW()
		WHERE id = $3;
	`
	tag, err := s.pool.Exec(ctx, query, isActive, quotaTotal, id)
	if err != nil {
		return fmt.Errorf("postgres_promo.UpdateCampaign: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPromoNotFound
	}
	return nil
}

// ReserveQuotaAtomic melakukan penambahan kuota terpakai secara atomik dan thread-safe.
func (s *PostgresPromoStore) ReserveQuotaAtomic(ctx context.Context, code string) error {
	code = strings.TrimSpace(strings.ToUpper(code))
	query := `
		UPDATE promo_campaigns
		SET quota_used = quota_used + 1, updated_at = NOW()
		WHERE UPPER(code) = $1
		  AND is_active = TRUE
		  AND quota_used < quota_total
		  AND valid_from <= NOW()
		  AND valid_to >= NOW()
		RETURNING id;
	`
	var id string
	err := s.pool.QueryRow(ctx, query, code).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return ErrPromoQuotaExhausted
		}
		return fmt.Errorf("postgres_promo.ReserveQuotaAtomic: %w", err)
	}
	return nil
}

// ReleaseQuotaAtomic mengembalikan kuota pemakaian saat booking dibatalkan atau checkout gagal.
func (s *PostgresPromoStore) ReleaseQuotaAtomic(ctx context.Context, code string) error {
	code = strings.TrimSpace(strings.ToUpper(code))
	query := `
		UPDATE promo_campaigns
		SET quota_used = GREATEST(0, quota_used - 1), updated_at = NOW()
		WHERE UPPER(code) = $1;
	`
	_, err := s.pool.Exec(ctx, query, code)
	if err != nil {
		return fmt.Errorf("postgres_promo.ReleaseQuotaAtomic: %w", err)
	}
	return nil
}
