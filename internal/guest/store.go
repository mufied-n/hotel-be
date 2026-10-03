package guest

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidEmail          = errors.New("guest: invalid email format")
	ErrRateLimited           = errors.New("guest: request cooldown active, please wait")
	ErrInvalidOrExpiredCode  = errors.New("guest: invalid or expired verification code")
	ErrMaxAttemptsExceeded   = errors.New("guest: maximum verification attempts exceeded")
	ErrSessionNotFound       = errors.New("guest: session not found or revoked")
	ErrSessionExpired        = errors.New("guest: session has expired")
	ErrBookingNotFound       = errors.New("guest: booking not found or unauthorized")
	ErrReceiptNotAvailable   = errors.New("guest: receipt not available for non-confirmed booking")
)

// Store mendefinisikan interface persistensi database untuk modul guest.
type Store interface {
	CreateChallenge(ctx context.Context, c *Challenge) error
	GetLatestActiveChallenge(ctx context.Context, email string) (*Challenge, error)
	UpdateChallengeAttempts(ctx context.Context, id string, attempts int) error
	MarkChallengeVerified(ctx context.Context, id string, verifiedAt time.Time) error

	// Operasi atomik untuk mitigasi race condition verifikasi dan pembuatan challenge OTP (BE-R03)
	CreateChallengeWithCooldown(ctx context.Context, c *Challenge, cooldown time.Duration) error
	VerifyAndConsumeChallenge(ctx context.Context, email, inputHash string, now time.Time, newSession *GuestSession) (*GuestSession, error)

	CreateSession(ctx context.Context, s *GuestSession) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*GuestSession, error)
	TouchSession(ctx context.Context, id string, lastActiveAt, expiresAt time.Time) error
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error

	CountActiveBookingsByEmail(ctx context.Context, email string) (int, error)
	ListBookingsByEmail(ctx context.Context, email, status string, limit int) ([]BookingSummary, error)
	GetBookingDetailByEmail(ctx context.Context, email, bookingID string) (*BookingDetail, error)
	GetBookingReceiptData(ctx context.Context, email, bookingID string) (*ReceiptDTO, error)
}

