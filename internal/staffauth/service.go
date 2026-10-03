package staffauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// TokenPrefix membedakan token staf dari token sesi tamu pada header Authorization.
	TokenPrefix       = "stf_"
	DefaultSessionTTL = 8 * time.Hour
	MaxFailedAttempts = 5
	LockDuration      = 15 * time.Minute
	MinPasswordLen    = 12
	defaultBcryptCost = 12
)

// Service mengimplementasikan login, verifikasi sesi, logout, dan set password.
type Service struct {
	store      Store
	cost       int
	ttl        time.Duration
	now        func() time.Time
	dummyHash  []byte
	dummyReady bool
}

// Option mengubah konfigurasi Service (terutama untuk test).
type Option func(*Service)

func WithBcryptCost(cost int) Option        { return func(s *Service) { s.cost = cost } }
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }
func WithSessionTTL(d time.Duration) Option { return func(s *Service) { s.ttl = d } }

func NewService(store Store, opts ...Option) *Service {
	s := &Service{store: store, cost: defaultBcryptCost, ttl: DefaultSessionTTL, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	// Hash tiruan agar waktu respons user tidak ada ≈ user ada (anti-enumeration).
	if h, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), s.cost); err == nil {
		s.dummyHash, s.dummyReady = h, true
	}
	return s
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Service) burn(password string) {
	if s.dummyReady {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
	}
}

// Login memverifikasi kredensial dan membuat sesi baru.
func (s *Service) Login(ctx context.Context, username, password string) (string, time.Time, Principal, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		s.burn(password)
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	u, err := s.store.GetByUsername(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		s.burn(password)
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, Principal{}, fmt.Errorf("staffauth: load user: %w", err)
	}
	now := s.now()
	if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		return "", time.Time{}, Principal{}, &LockedError{Until: *u.LockedUntil}
	}
	if !u.Active || u.PasswordHash == "" {
		s.burn(password)
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		failed := u.FailedAttempts + 1
		var lockedUntil *time.Time
		if failed >= MaxFailedAttempts {
			t := now.Add(LockDuration)
			lockedUntil = &t
		}
		if err := s.store.RecordFailure(ctx, u.ID, failed, lockedUntil); err != nil {
			return "", time.Time{}, Principal{}, fmt.Errorf("staffauth: record failure: %w", err)
		}
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	if err := s.store.RecordSuccess(ctx, u.ID); err != nil {
		return "", time.Time{}, Principal{}, fmt.Errorf("staffauth: record success: %w", err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, Principal{}, fmt.Errorf("staffauth: token entropy: %w", err)
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	expires := now.Add(s.ttl)
	if err := s.store.CreateSession(ctx, u.ID, hashToken(token), expires); err != nil {
		return "", time.Time{}, Principal{}, fmt.Errorf("staffauth: create session: %w", err)
	}
	return token, expires, Principal{StaffID: u.ID, Username: u.Username, Role: u.Role, FullName: u.FullName}, nil
}

// VerifyStaffToken memvalidasi token ke store. Token tanpa prefix ditolak tanpa menyentuh store.
// Error selain ErrUnauthorized berarti infrastruktur gagal (jangan perlakukan sebagai tamu).
func (s *Service) VerifyStaffToken(ctx context.Context, token string) (Principal, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return Principal{}, ErrUnauthorized
	}
	p, err := s.store.GetSessionPrincipal(ctx, hashToken(token), s.now())
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, fmt.Errorf("staffauth: verify: %w", err)
	}
	return *p, nil
}

// Logout mencabut sesi milik token; idempoten.
func (s *Service) Logout(ctx context.Context, token string) error {
	if !strings.HasPrefix(token, TokenPrefix) {
		return nil
	}
	return s.store.RevokeSession(ctx, hashToken(token))
}

// SetPassword menetapkan password baru (dipakai CLI operator). Sesi lama dicabut oleh store.
func (s *Service) SetPassword(ctx context.Context, username, password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLen {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return fmt.Errorf("staffauth: hash password: %w", err)
	}
	return s.store.SetPassword(ctx, strings.TrimSpace(username), string(hash))
}
