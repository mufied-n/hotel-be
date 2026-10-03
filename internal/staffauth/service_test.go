package staffauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// memStore adalah Store in-memory yang meniru semantik query Postgres (aktif, tidak dicabut, belum kedaluwarsa).
type memStore struct {
	mu       sync.Mutex
	users    map[string]*User // by username
	sessions map[string]memSession
	failSess bool
}

type memSession struct {
	staffID   string
	expiresAt time.Time
	revoked   bool
}

func newMemStore() *memStore {
	return &memStore{users: map[string]*User{}, sessions: map[string]memSession{}}
}

func (m *memStore) addUser(u User) { m.users[u.Username] = &u }

func (m *memStore) GetByUsername(_ context.Context, username string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok {
		return nil, ErrUserNotFound
	}
	c := *u
	return &c, nil
}

func (m *memStore) RecordFailure(_ context.Context, id string, failed int, lockedUntil *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.FailedAttempts, u.LockedUntil = failed, lockedUntil
		}
	}
	return nil
}

func (m *memStore) RecordSuccess(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.FailedAttempts, u.LockedUntil = 0, nil
		}
	}
	return nil
}

func (m *memStore) CreateSession(_ context.Context, staffID, tokenHash string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSess {
		return errors.New("db down")
	}
	m.sessions[tokenHash] = memSession{staffID: staffID, expiresAt: expiresAt}
	return nil
}

func (m *memStore) GetSessionPrincipal(_ context.Context, tokenHash string, now time.Time) (*Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSess {
		return nil, errors.New("db down")
	}
	s, ok := m.sessions[tokenHash]
	if !ok || s.revoked || !now.Before(s.expiresAt) {
		return nil, ErrUnauthorized
	}
	for _, u := range m.users {
		if u.ID == s.staffID && u.Active {
			return &Principal{StaffID: u.ID, Username: u.Username, Role: u.Role, FullName: u.FullName}, nil
		}
	}
	return nil, ErrUnauthorized
}

func (m *memStore) RevokeSession(_ context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[tokenHash]; ok {
		s.revoked = true
		m.sessions[tokenHash] = s
	}
	return nil
}

func (m *memStore) SetPassword(_ context.Context, username, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok {
		return ErrUserNotFound
	}
	u.PasswordHash = hash
	return nil
}

const goodPassword = "correct-horse-battery"

func newTestService(t *testing.T) (*Service, *memStore, *time.Time) {
	t.Helper()
	st := newMemStore()
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	svc := NewService(st, WithBcryptCost(4), WithClock(func() time.Time { return now }))
	st.addUser(User{ID: "u1", Username: "fo", Role: "receptionist", FullName: "Front Office", Active: true})
	if err := svc.SetPassword(context.Background(), "fo", goodPassword); err != nil {
		t.Fatalf("setup password: %v", err)
	}
	st.addUser(User{ID: "u2", Username: "off", Role: "finance", Active: false})
	_ = st.SetPassword(context.Background(), "off", st.users["fo"].PasswordHash)
	st.addUser(User{ID: "u3", Username: "nopass", Role: "gm_admin", Active: true})
	return svc, st, &now
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		password string
		wantErr  error
	}{
		{name: "sukses", user: "fo", password: goodPassword},
		{name: "password salah", user: "fo", password: "wrong-password-123", wantErr: ErrInvalidCredentials},
		{name: "user tidak ada", user: "ghost", password: goodPassword, wantErr: ErrInvalidCredentials},
		{name: "akun nonaktif", user: "off", password: goodPassword, wantErr: ErrInvalidCredentials},
		{name: "akun seed tanpa password", user: "nopass", password: goodPassword, wantErr: ErrInvalidCredentials},
		{name: "username kosong", user: "", password: goodPassword, wantErr: ErrInvalidCredentials},
		{name: "password kosong", user: "fo", password: "", wantErr: ErrInvalidCredentials},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			token, exp, p, err := svc.Login(context.Background(), tc.user, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if token != "" {
					t.Error("token tidak boleh keluar saat gagal")
				}
				return
			}
			if !strings.HasPrefix(token, TokenPrefix) || len(token) < 40 {
				t.Errorf("token format salah: %q", token)
			}
			if want := svc.now().Add(DefaultSessionTTL); !exp.Equal(want) {
				t.Errorf("expires = %v, want %v", exp, want)
			}
			if p.Role != "receptionist" || p.Username != "fo" {
				t.Errorf("principal = %+v", p)
			}
		})
	}
}

func TestLogin_LockoutAndReset(t *testing.T) {
	svc, st, now := newTestService(t)
	ctx := context.Background()
	for i := 0; i < MaxFailedAttempts; i++ {
		if _, _, _, err := svc.Login(ctx, "fo", "wrong-password-123"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d err = %v", i, err)
		}
	}
	// Terkunci: password benar pun ditolak.
	if _, _, _, err := svc.Login(ctx, "fo", goodPassword); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("locked err = %v, want ErrAccountLocked", err)
	}
	// Setelah masa kunci lewat, login sukses dan counter di-reset.
	*now = now.Add(LockDuration + time.Second)
	if _, _, _, err := svc.Login(ctx, "fo", goodPassword); err != nil {
		t.Fatalf("login after lock expiry: %v", err)
	}
	if u := st.users["fo"]; u.FailedAttempts != 0 || u.LockedUntil != nil {
		t.Errorf("counter tidak di-reset: %+v", u)
	}
}

func TestVerifyStaffToken(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		prepare func(svc *Service, st *memStore, now *time.Time) string
		wantErr bool
	}{
		{name: "token valid", prepare: func(svc *Service, _ *memStore, _ *time.Time) string {
			tok, _, _, _ := svc.Login(ctx, "fo", goodPassword)
			return tok
		}},
		{name: "token acak", prepare: func(*Service, *memStore, *time.Time) string { return TokenPrefix + "abc" }, wantErr: true},
		{name: "nama role sebagai token", prepare: func(*Service, *memStore, *time.Time) string { return "gm_admin" }, wantErr: true},
		{name: "token kosong", prepare: func(*Service, *memStore, *time.Time) string { return "" }, wantErr: true},
		{name: "kedaluwarsa", prepare: func(svc *Service, _ *memStore, now *time.Time) string {
			tok, _, _, _ := svc.Login(ctx, "fo", goodPassword)
			*now = now.Add(DefaultSessionTTL + time.Second)
			return tok
		}, wantErr: true},
		{name: "setelah logout", prepare: func(svc *Service, _ *memStore, _ *time.Time) string {
			tok, _, _, _ := svc.Login(ctx, "fo", goodPassword)
			_ = svc.Logout(ctx, tok)
			return tok
		}, wantErr: true},
		{name: "akun dinonaktifkan setelah login", prepare: func(svc *Service, st *memStore, _ *time.Time) string {
			tok, _, _, _ := svc.Login(ctx, "fo", goodPassword)
			st.users["fo"].Active = false
			return tok
		}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, st, now := newTestService(t)
			tok := tc.prepare(svc, st, now)
			p, err := svc.VerifyStaffToken(ctx, tok)
			if tc.wantErr {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("err = %v, want ErrUnauthorized", err)
				}
				return
			}
			if err != nil || p.Role != "receptionist" {
				t.Fatalf("p=%+v err=%v", p, err)
			}
		})
	}
}

func TestVerifyStaffToken_StoreErrorIsNotUnauthorized(t *testing.T) {
	svc, st, _ := newTestService(t)
	tok, _, _, _ := svc.Login(context.Background(), "fo", goodPassword)
	st.failSess = true
	_, err := svc.VerifyStaffToken(context.Background(), tok)
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want infrastruktur error (bukan ErrUnauthorized)", err)
	}
}

func TestSetPassword(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		password string
		wantErr  error
	}{
		{name: "terlalu pendek", user: "fo", password: "short", wantErr: ErrWeakPassword},
		{name: "batas 11 karakter", user: "fo", password: "12345678901", wantErr: ErrWeakPassword},
		{name: "batas 12 karakter", user: "fo", password: "123456789012"},
		{name: "user tidak ada", user: "ghost", password: goodPassword, wantErr: ErrUserNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, st, _ := newTestService(t)
			err := svc.SetPassword(context.Background(), tc.user, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if h := st.users[tc.user].PasswordHash; h == "" || h == tc.password {
					t.Errorf("password harus tersimpan sebagai hash, got %q", h)
				}
			}
		})
	}
}
