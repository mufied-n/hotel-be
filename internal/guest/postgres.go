package guest

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore mengimplementasikan guest.Store menggunakan connection pool pgxpool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore membuat instance baru PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateChallenge(ctx context.Context, c *Challenge) error {
	query := `
		INSERT INTO guest_auth_challenges (
			email, code_hash, attempts, max_attempts, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		c.Email, c.CodeHash, c.Attempts, c.MaxAttempts, c.ExpiresAt, c.CreatedAt,
	).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("guest_store.create_challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetLatestActiveChallenge(ctx context.Context, email string) (*Challenge, error) {
	query := `
		SELECT id, email, code_hash, attempts, max_attempts, expires_at, verified_at, created_at
		FROM guest_auth_challenges
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var c Challenge
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&c.ID, &c.Email, &c.CodeHash, &c.Attempts, &c.MaxAttempts, &c.ExpiresAt, &c.VerifiedAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("guest_store.get_latest_challenge: %w", err)
	}
	return &c, nil
}

func (s *PostgresStore) UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error {
	query := `UPDATE guest_auth_challenges SET attempts = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, attempts)
	if err != nil {
		return fmt.Errorf("guest_store.update_challenge_attempts: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkChallengeVerified(ctx context.Context, id string, verifiedAt time.Time) error {
	query := `UPDATE guest_auth_challenges SET verified_at = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, verifiedAt)
	if err != nil {
		return fmt.Errorf("guest_store.mark_challenge_verified: %w", err)
	}
	return nil
}

// CreateChallengeWithCooldown membuat challenge OTP baru secara atomik dengan proteksi cooldown per-email (BE-R03).
// Menggunakan pg_advisory_xact_lock untuk mencegah bypass cooldown pada request konkuren.
func (s *PostgresStore) CreateChallengeWithCooldown(ctx context.Context, c *Challenge, cooldown time.Duration) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("guest_store.begin_tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock advisory transaction-level berbasis hash email
	lockQuery := `SELECT pg_advisory_xact_lock(hashtext('guest_challenge:' || $1))`
	if _, err := tx.Exec(ctx, lockQuery, c.Email); err != nil {
		return fmt.Errorf("guest_store.advisory_lock: %w", err)
	}

	checkQuery := `
		SELECT created_at
		FROM guest_auth_challenges
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var latestCreatedAt time.Time
	err = tx.QueryRow(ctx, checkQuery, c.Email).Scan(&latestCreatedAt)
	if err == nil {
		if c.CreatedAt.Sub(latestCreatedAt) < cooldown {
			return ErrRateLimited
		}
	} else if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("guest_store.check_cooldown: %w", err)
	}

	insertQuery := `
		INSERT INTO guest_auth_challenges (
			email, code_hash, attempts, max_attempts, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	if err := tx.QueryRow(ctx, insertQuery,
		c.Email, c.CodeHash, c.Attempts, c.MaxAttempts, c.ExpiresAt, c.CreatedAt,
	).Scan(&c.ID); err != nil {
		return fmt.Errorf("guest_store.create_challenge: %w", err)
	}

	// BE-R17: Masukkan event ke tabel outbox dalam transaksi atomik yang sama
	if c.PlainCode != "" {
		payloadBytes, mErr := json.Marshal(map[string]any{
			"challenge_id": c.ID,
			"email":        c.Email,
			"otp_code":     c.PlainCode,
			"expires_at":   c.ExpiresAt,
		})
		if mErr != nil {
			return fmt.Errorf("guest_store.marshal_outbox: %w", mErr)
		}
		outboxQuery := `
			INSERT INTO outbox (topic, payload, status, attempts, next_retry_at, created_at)
			VALUES ('guest.otp_dispatch', $1, 'pending', 0, now(), now())
		`
		if _, err := tx.Exec(ctx, outboxQuery, payloadBytes); err != nil {
			return fmt.Errorf("guest_store.insert_outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("guest_store.commit_tx: %w", err)
	}
	return nil
}

// HasOutbox menandakan bahwa store mengeksekusi outbox queuing secara atomik (BE-R17).
func (s *PostgresStore) HasOutbox() bool {
	return true
}

// VerifyAndConsumeChallenge mengeksekusi verifikasi kode, penambahan attempts, dan pembuatan sesi dalam satu transaksi atomik ber-row lock (BE-R03).
func (s *PostgresStore) VerifyAndConsumeChallenge(ctx context.Context, email, inputHash string, now time.Time, newSession *GuestSession) (*GuestSession, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("guest_store.begin_tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id, email, code_hash, attempts, max_attempts, expires_at, verified_at
		FROM guest_auth_challenges
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`
	var c Challenge
	err = tx.QueryRow(ctx, query, email).Scan(
		&c.ID, &c.Email, &c.CodeHash, &c.Attempts, &c.MaxAttempts, &c.ExpiresAt, &c.VerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidOrExpiredCode
		}
		return nil, fmt.Errorf("guest_store.lock_challenge: %w", err)
	}

	if c.VerifiedAt != nil || now.After(c.ExpiresAt) {
		return nil, ErrInvalidOrExpiredCode
	}

	if c.Attempts >= c.MaxAttempts {
		return nil, ErrMaxAttemptsExceeded
	}

	// Bandingkan kode constant-time
	match := subtle.ConstantTimeCompare([]byte(c.CodeHash), []byte(inputHash)) == 1
	if !match {
		incQuery := `
			UPDATE guest_auth_challenges
			SET attempts = attempts + 1
			WHERE id = $1
			RETURNING attempts
		`
		var updatedAttempts int
		if err := tx.QueryRow(ctx, incQuery, c.ID).Scan(&updatedAttempts); err != nil {
			return nil, fmt.Errorf("guest_store.inc_attempts: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("guest_store.commit_inc: %w", err)
		}
		if updatedAttempts >= c.MaxAttempts {
			return nil, ErrMaxAttemptsExceeded
		}
		return nil, ErrInvalidOrExpiredCode
	}

	// Tandai verified
	updateVerifiedQuery := `
		UPDATE guest_auth_challenges
		SET verified_at = $2
		WHERE id = $1 AND verified_at IS NULL
	`
	cmdTag, err := tx.Exec(ctx, updateVerifiedQuery, c.ID, now)
	if err != nil {
		return nil, fmt.Errorf("guest_store.mark_verified: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return nil, ErrInvalidOrExpiredCode
	}

	// Buat sesi dalam transaksi yang sama
	insertSessQuery := `
		INSERT INTO guest_sessions (
			guest_email, token_hash, expires_at, last_active_at, created_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	if err := tx.QueryRow(ctx, insertSessQuery,
		newSession.GuestEmail, newSession.TokenHash, newSession.ExpiresAt, newSession.LastActiveAt, newSession.CreatedAt,
	).Scan(&newSession.ID); err != nil {
		return nil, fmt.Errorf("guest_store.create_session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("guest_store.commit_verified: %w", err)
	}

	return newSession, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, sess *GuestSession) error {
	query := `
		INSERT INTO guest_sessions (
			guest_email, token_hash, expires_at, last_active_at, created_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query,
		sess.GuestEmail, sess.TokenHash, sess.ExpiresAt, sess.LastActiveAt, sess.CreatedAt,
	).Scan(&sess.ID)
	if err != nil {
		return fmt.Errorf("guest_store.create_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*GuestSession, error) {
	query := `
		SELECT id, guest_email, token_hash, expires_at, last_active_at, created_at
		FROM guest_sessions
		WHERE token_hash = $1
	`
	var sess GuestSession
	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&sess.ID, &sess.GuestEmail, &sess.TokenHash, &sess.ExpiresAt, &sess.LastActiveAt, &sess.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("guest_store.get_session: %w", err)
	}
	return &sess, nil
}

func (s *PostgresStore) TouchSession(ctx context.Context, id string, lastActiveAt, expiresAt time.Time) error {
	query := `UPDATE guest_sessions SET last_active_at = $2, expires_at = $3 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, lastActiveAt, expiresAt)
	if err != nil {
		return fmt.Errorf("guest_store.touch_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM guest_sessions WHERE token_hash = $1`
	_, err := s.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("guest_store.delete_session: %w", err)
	}
	return nil
}

func (s *PostgresStore) CountActiveBookingsByEmail(ctx context.Context, email string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM bookings
		WHERE LOWER(TRIM(guest_email)) = $1 AND status IN ('pending', 'confirmed', 'checked_in')
	`
	var count int
	err := s.pool.QueryRow(ctx, query, email).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("guest_store.count_active_bookings: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]BookingSummary, error) {
	baseQuery := `
		SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
		       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
		       b.num_rooms, b.num_guests, b.status, b.total_price_minor, b.currency, b.created_at
		FROM bookings b
		LEFT JOIN room_types r ON r.id = b.room_type_id
		WHERE LOWER(TRIM(b.guest_email)) = $1
	`
	var rows pgx.Rows
	var err error

	switch status {
	case "upcoming":
		q := baseQuery + ` AND b.status IN ('pending', 'confirmed') AND b.check_out >= CURRENT_DATE ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	case "completed":
		q := baseQuery + ` AND (b.status = 'checked_out' OR (b.status = 'confirmed' AND b.check_out < CURRENT_DATE)) ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	case "cancelled":
		q := baseQuery + ` AND b.status IN ('cancelled', 'expired', 'failed', 'no_show') ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	default: // "all" atau kosong
		q := baseQuery + ` ORDER BY b.created_at DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, q, email, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("guest_store.list_bookings: %w", err)
	}
	defer rows.Close()

	var results []BookingSummary
	for rows.Next() {
		var item BookingSummary
		if err := rows.Scan(
			&item.ID, &item.RoomTypeID, &item.RoomTypeName,
			&item.CheckIn, &item.CheckOut, &item.NumRooms, &item.NumGuests,
			&item.Status, &item.TotalPriceMinor, &item.Currency, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("guest_store.scan_booking: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("guest_store.rows_err: %w", err)
	}

	if results == nil {
		results = []BookingSummary{}
	}
	return results, nil
}

func (s *PostgresStore) GetBookingDetailByEmail(ctx context.Context, email, bookingID string) (*BookingDetail, error) {
	query := `
		SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
		       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
		       b.num_rooms, b.num_guests, b.status, b.total_price_minor, b.currency,
		       b.guest_name, b.guest_email, COALESCE(b.guest_phone, ''),
		       COALESCE(b.estimated_arrival_time, ''), COALESCE(b.special_requests, ''),
		       COALESCE(b.cancellation_policy, ''), COALESCE(b.rate_plan_code, ''),
		       b.expires_at, b.created_at
		FROM bookings b
		LEFT JOIN room_types r ON r.id = b.room_type_id
		WHERE b.id = $1 AND LOWER(TRIM(b.guest_email)) = $2
	`
	var d BookingDetail
	err := s.pool.QueryRow(ctx, query, bookingID, email).Scan(
		&d.ID, &d.RoomTypeID, &d.RoomTypeName,
		&d.CheckIn, &d.CheckOut, &d.NumRooms, &d.NumGuests,
		&d.Status, &d.TotalPriceMinor, &d.Currency,
		&d.GuestName, &d.GuestEmail, &d.GuestPhone,
		&d.EstimatedArrivalTime, &d.SpecialRequests,
		&d.CancellationPolicy, &d.RatePlanCode,
		&d.ExpiresAt, &d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil // IDOR-safe: mengembalikan nil agar service mengonversi ke ErrBookingNotFound
		}
		return nil, fmt.Errorf("guest_store.get_booking_detail: %w", err)
	}

	// BE-R15: Resolusi tautan pembayaran dari attempt aktif jika reservasi berstatus pending
	if d.Status == "pending" && d.ExpiresAt != nil && time.Now().UTC().Before(*d.ExpiresAt) {
		var payloadJSON []byte
		pQuery := `
			SELECT payload
			FROM payment_attempts
			WHERE booking_id = $1 AND status IN ('initiated', 'unknown_timeout') AND payload ? 'payment_url'
			ORDER BY created_at DESC
			LIMIT 1
		`
		if pErr := s.pool.QueryRow(ctx, pQuery, bookingID).Scan(&payloadJSON); pErr == nil {
			var pMap map[string]any
			if json.Unmarshal(payloadJSON, &pMap) == nil {
				if url, ok := pMap["payment_url"].(string); ok {
					d.PaymentURL = url
				}
			}
		}
	}

	return &d, nil
}

func (s *PostgresStore) GetBookingReceiptData(ctx context.Context, email, bookingID string) (*ReceiptDTO, error) {
	cleanID := strings.TrimSpace(bookingID)
	if strings.HasPrefix(strings.ToUpper(cleanID), "PKU-") {
		parts := strings.Split(cleanID, "-")
		if len(parts) >= 3 {
			cleanID = parts[2]
		}
	}

	var row pgx.Row
	if strings.TrimSpace(email) == "" {
		query := `
			SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
			       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
			       (b.check_out - b.check_in) AS total_nights,
			       b.num_rooms, b.num_guests, b.status,
			       b.room_subtotal_minor, b.breakfast_charge_minor, b.discount_minor, b.tax_minor, b.total_price_minor,
			       b.currency, b.guest_name, b.guest_email, COALESCE(b.guest_phone, ''),
			       COALESCE(b.special_requests, ''),
			       b.rate_plan_code, b.cancellation_policy, b.cancellation_desc,
			       b.created_at
			FROM bookings b
			LEFT JOIN room_types r ON r.id = b.room_type_id
			WHERE (b.id::text = $1 OR b.id::text ILIKE $1 || '%')
		`
		row = s.pool.QueryRow(ctx, query, cleanID)
	} else {
		query := `
			SELECT b.id, b.room_type_id, COALESCE(r.name, 'Room'),
			       to_char(b.check_in, 'YYYY-MM-DD'), to_char(b.check_out, 'YYYY-MM-DD'),
			       (b.check_out - b.check_in) AS total_nights,
			       b.num_rooms, b.num_guests, b.status,
			       b.room_subtotal_minor, b.breakfast_charge_minor, b.discount_minor, b.tax_minor, b.total_price_minor,
			       b.currency, b.guest_name, b.guest_email, COALESCE(b.guest_phone, ''),
			       COALESCE(b.special_requests, ''),
			       b.rate_plan_code, b.cancellation_policy, b.cancellation_desc,
			       b.created_at
			FROM bookings b
			LEFT JOIN room_types r ON r.id = b.room_type_id
			WHERE (b.id::text = $1 OR b.id::text ILIKE $1 || '%') AND LOWER(TRIM(b.guest_email)) = $2
		`
		row = s.pool.QueryRow(ctx, query, cleanID, strings.ToLower(strings.TrimSpace(email)))
	}
	var (
		id                   string
		roomTypeID           string
		roomTypeName         string
		checkInDate          string
		checkOutDate         string
		totalNights          int
		numRooms             int
		numGuests            int
		status               string
		roomSubtotalMinor    int64
		breakfastChargeMinor int64
		discountMinor        int64
		taxMinor             int64
		totalPriceMinor      int64
		currency             string
		guestName            string
		guestEmail           string
		guestPhone           string
		specialRequests      string
		ratePlanCode         string
		cancellationPolicy   string
		cancellationDesc     string
		createdAt            time.Time
	)

	err := row.Scan(
		&id, &roomTypeID, &roomTypeName,
		&checkInDate, &checkOutDate,
		&totalNights,
		&numRooms, &numGuests, &status,
		&roomSubtotalMinor, &breakfastChargeMinor, &discountMinor, &taxMinor, &totalPriceMinor,
		&currency, &guestName, &guestEmail, &guestPhone,
		&specialRequests,
		&ratePlanCode, &cancellationPolicy, &cancellationDesc,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil // IDOR safe: nil -> ErrBookingNotFound
		}
		return nil, fmt.Errorf("guest_store.get_receipt: %w", err)
	}

	// 2. Baca informasi pelunasan dari payment_attempts (jika ada)
	var (
		provider          = "Xendit"
		providerReference = ""
		paymentStatus     = "PAID"
		paidAt            = createdAt.UTC().Format(time.RFC3339)
	)

	pQuery := `
		SELECT provider, provider_reference, status, created_at
		FROM payment_attempts
		WHERE booking_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var (
		pProv string
		pRef  string
		pStat string
		pTime time.Time
	)
	if pErr := s.pool.QueryRow(ctx, pQuery, bookingID).Scan(&pProv, &pRef, &pStat, &pTime); pErr == nil {
		if pProv != "" {
			provider = pProv
		}
		providerReference = pRef
		if pStat == "success" || pStat == "paid" {
			paymentStatus = "PAID"
		} else {
			paymentStatus = strings.ToUpper(pStat)
		}
		paidAt = pTime.UTC().Format(time.RFC3339)
	}

	mealPlan := "Room Only"
	if strings.Contains(strings.ToLower(ratePlanCode), "breakfast") || ratePlanCode == "BB" || breakfastChargeMinor > 0 {
		mealPlan = "Sarapan Termasuk (Breakfast Included)"
	}

	nightlyRateMinor := int64(0)
	if totalNights > 0 && numRooms > 0 {
		nightlyRateMinor = roomSubtotalMinor / int64(totalNights*numRooms)
	}

	cleanRef := strings.ToUpper(strings.ReplaceAll(id, "-", ""))
	if len(cleanRef) > 8 {
		cleanRef = cleanRef[:8]
	}
	bookingRef := fmt.Sprintf("PKU-%s-%s", createdAt.Format("20060102"), cleanRef)
	invoiceNumber := fmt.Sprintf("INV/PKU/%s/%s", createdAt.Format("200601"), cleanRef)

	cancelPolicyText := cancellationDesc
	if cancelPolicyText == "" {
		cancelPolicyText = "Pembatalan fleksibel sebelum H-1 pukul 14:00 WIB. Pembatalan setelah cutoff dikenakan biaya penuh."
	}

	dto := &ReceiptDTO{
		InvoiceNumber:    invoiceNumber,
		InvoiceDate:      createdAt.UTC().Format(time.RFC3339),
		BookingID:        id,
		BookingReference: bookingRef,
		Status:           status,
		HotelInfo: HotelInfo{
			Name:    "Pulang ke Uttara",
			Tagline: "Urban Boutique Hotel & Residence",
			Address: "Jl. Kaliurang Km 5.6 No. 1, Caturtunggal, Depok, Sleman, D.I. Yogyakarta 55281",
			Phone:   "+62 274 5022888",
			Email:   "stay@pulangkeuttara.id",
			Website: "https://pulangkeuttara.id",
		},
		StayDetails: StayDetails{
			CheckInDate:  checkInDate,
			CheckInTime:  "14:00 WIB",
			CheckOutDate: checkOutDate,
			CheckOutTime: "12:00 WIB",
			TotalNights:  totalNights,
			Timezone:     "Asia/Jakarta",
		},
		GuestDetails: GuestDetails{
			Name:            guestName,
			Email:           guestEmail,
			Phone:           guestPhone,
			NumRooms:        numRooms,
			NumGuests:       numGuests,
			SpecialRequests: specialRequests,
		},
		RoomItem: RoomItemReceipt{
			RoomTypeID:       roomTypeID,
			RoomTypeName:     roomTypeName,
			RatePlanCode:     ratePlanCode,
			MealPlan:         mealPlan,
			NumRooms:         numRooms,
			TotalNights:      totalNights,
			NightlyRateMinor: nightlyRateMinor,
			SubtotalMinor:    roomSubtotalMinor,
		},
		PricingBreakdown: PricingBreakdown{
			Currency:             currency,
			RoomSubtotalMinor:    roomSubtotalMinor,
			BreakfastChargeMinor: breakfastChargeMinor,
			DiscountMinor:        discountMinor,
			TaxMinor:             taxMinor,
			TotalPriceMinor:      totalPriceMinor,
		},
		PaymentSummary: PaymentSummary{
			Status:            paymentStatus,
			Provider:          provider,
			ProviderReference: providerReference,
			PaidAt:            paidAt,
		},
		Policies: PoliciesReceipt{
			CheckInPolicy:      "Wajib menunjukkan identitas diri yang berlaku (KTP/Paspor) saat check-in. Waktu check-in mulai 14:00 WIB.",
			CancellationPolicy: cancelPolicyText,
		},
		QRPayload: fmt.Sprintf("https://pulangkeuttara.id/verify/booking/%s", id),
	}

	return dto, nil
}

var _ Store = (*PostgresStore)(nil)

