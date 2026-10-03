package staffauth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// realPool memakai DB *_test nyata; di-skip bila TEST_DATABASE_URL tidak diset (lihat testing/integration).
func realPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || (!strings.HasSuffix(cfg.ConnConfig.Database, "_test") && os.Getenv("ALLOW_DESTRUCTIVE_TESTS") != "1") {
		t.Skip("database is not *_test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestRealDB_StaffAuthFlow memverifikasi seluruh alur terhadap PostgreSQL nyata (BE-R01).
func TestRealDB_StaffAuthFlow(t *testing.T) {
	pool := realPool(t)
	ctx := context.Background()
	svc := NewService(NewPostgresStore(pool), WithBcryptCost(4))
	const user = "fo_receptionist"
	_, _ = pool.Exec(ctx, `UPDATE staff_users SET is_active = TRUE, password_hash = NULL, failed_attempts = 0, locked_until = NULL WHERE username = $1`, user)
	_, _ = pool.Exec(ctx, `DELETE FROM staff_sessions`)

	// Akun seed tanpa password tidak dapat login.
	if _, _, _, err := svc.Login(ctx, user, goodPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login tanpa password err = %v", err)
	}
	if err := svc.SetPassword(ctx, user, goodPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := svc.SetPassword(ctx, "ghost-user", goodPassword); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("SetPassword unknown err = %v", err)
	}

	token, exp, p, err := svc.Login(ctx, user, goodPassword)
	if err != nil || p.Role != "receptionist" || !exp.After(time.Now()) {
		t.Fatalf("login: p=%+v exp=%v err=%v", p, exp, err)
	}
	var stored string
	_ = pool.QueryRow(ctx, `SELECT token_hash FROM staff_sessions LIMIT 1`).Scan(&stored)
	if stored == "" || strings.Contains(stored, token) || stored != hashToken(token) {
		t.Fatalf("token harus tersimpan sebagai hash, got %q", stored)
	}
	if got, err := svc.VerifyStaffToken(ctx, token); err != nil || got.Username != user {
		t.Fatalf("verify: %+v %v", got, err)
	}

	// Akun dinonaktifkan: sesi langsung tidak berlaku.
	_, _ = pool.Exec(ctx, `UPDATE staff_users SET is_active = FALSE WHERE username = $1`, user)
	if _, err := svc.VerifyStaffToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("verify nonaktif err = %v", err)
	}
	_, _ = pool.Exec(ctx, `UPDATE staff_users SET is_active = TRUE WHERE username = $1`, user)

	// Logout mencabut sesi.
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.VerifyStaffToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("verify setelah logout err = %v", err)
	}

	// Ganti password mencabut sesi aktif dan mereset lockout.
	token2, _, _, _ := svc.Login(ctx, user, goodPassword)
	for i := 0; i < MaxFailedAttempts; i++ {
		_, _, _, _ = svc.Login(ctx, user, "wrong-password-123")
	}
	var le *LockedError
	if _, _, _, err := svc.Login(ctx, user, goodPassword); !errors.As(err, &le) {
		t.Fatalf("expected LockedError, got %v", err)
	}
	if err := svc.SetPassword(ctx, user, "another-strong-pass"); err != nil {
		t.Fatalf("SetPassword 2: %v", err)
	}
	if _, err := svc.VerifyStaffToken(ctx, token2); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sesi lama harus dicabut setelah ganti password, err = %v", err)
	}
	if _, _, _, err := svc.Login(ctx, user, "another-strong-pass"); err != nil {
		t.Fatalf("login setelah reset: %v", err)
	}

	// Sesi kedaluwarsa ditolak.
	short := NewService(NewPostgresStore(pool), WithBcryptCost(4), WithSessionTTL(-time.Second))
	tok3, _, _, _ := short.Login(ctx, user, "another-strong-pass")
	if _, err := short.VerifyStaffToken(ctx, tok3); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sesi kedaluwarsa err = %v", err)
	}
}
