// Package staffauth menyediakan autentikasi staf hotel: login username+password,
// sesi bertoken opak (hash di DB), dan verifikasi per request (BE-R01).
package staffauth

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials sengaja seragam untuk user tidak ada, password salah, akun nonaktif, atau tanpa password.
	ErrInvalidCredentials = errors.New("staffauth: invalid credentials")
	ErrAccountLocked      = errors.New("staffauth: account temporarily locked")
	ErrUnauthorized       = errors.New("staffauth: invalid, expired or revoked session")
	ErrWeakPassword       = errors.New("staffauth: password must be at least 12 characters")
	ErrUserNotFound       = errors.New("staffauth: user not found")
)

// LockedError membawa waktu berakhirnya kunci akun; Is(ErrAccountLocked) bernilai true.
type LockedError struct{ Until time.Time }

func (e *LockedError) Error() string { return ErrAccountLocked.Error() }
func (e *LockedError) Unwrap() error { return ErrAccountLocked }

// User adalah baris staff_users yang relevan untuk autentikasi.
type User struct {
	ID             string
	Username       string
	Role           string
	FullName       string
	PasswordHash   string
	Active         bool
	FailedAttempts int
	LockedUntil    *time.Time
}

// Principal adalah identitas staf terverifikasi yang diteruskan ke middleware.
type Principal struct {
	StaffID  string `json:"staff_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	FullName string `json:"full_name"`
}

// Store adalah kontrak persistensi. GetSessionPrincipal wajib mengembalikan ErrUnauthorized
// bila sesi tidak ada/dicabut/kedaluwarsa atau akun nonaktif; error lain berarti infrastruktur gagal.
type Store interface {
	GetByUsername(ctx context.Context, username string) (*User, error)
	RecordFailure(ctx context.Context, id string, failedAttempts int, lockedUntil *time.Time) error
	RecordSuccess(ctx context.Context, id string) error
	CreateSession(ctx context.Context, staffID, tokenHash string, expiresAt time.Time) error
	GetSessionPrincipal(ctx context.Context, tokenHash string, now time.Time) (*Principal, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	SetPassword(ctx context.Context, username, passwordHash string) error
}
