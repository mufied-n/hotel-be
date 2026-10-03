package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/platform/auth"
	"github.com/example/hotel-booking/internal/staffauth"
)

// fakeStaffAuth meniru staffauth.Service: hanya token terdaftar yang valid.
type fakeStaffAuth struct {
	tokens   map[string]staffauth.Principal
	loginErr error
	verErr   error // error infrastruktur pada Verify
	revoked  []string
}

func (f *fakeStaffAuth) Login(_ context.Context, username, password string) (string, time.Time, staffauth.Principal, error) {
	if f.loginErr != nil {
		return "", time.Time{}, staffauth.Principal{}, f.loginErr
	}
	if username == "fo" && password == "correct-horse-battery" {
		return "stf_issued", time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC), staffauth.Principal{Username: "fo", Role: "receptionist", FullName: "Front Office"}, nil
	}
	return "", time.Time{}, staffauth.Principal{}, staffauth.ErrInvalidCredentials
}

func (f *fakeStaffAuth) VerifyStaffToken(_ context.Context, token string) (staffauth.Principal, error) {
	if f.verErr != nil {
		return staffauth.Principal{}, f.verErr
	}
	if p, ok := f.tokens[token]; ok {
		return p, nil
	}
	return staffauth.Principal{}, staffauth.ErrUnauthorized
}

func (f *fakeStaffAuth) Logout(_ context.Context, token string) error {
	f.revoked = append(f.revoked, token)
	delete(f.tokens, token)
	return nil
}

func newFakeStaffAuth() *fakeStaffAuth {
	return &fakeStaffAuth{tokens: map[string]staffauth.Principal{
		"stf_valid_gm": {StaffID: "1", Username: "admin_gm", Role: "gm_admin"},
		"stf_valid_fo": {StaffID: "2", Username: "fo", Role: "receptionist"},
	}}
}

func probeRouter(v StaffVerifier) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	g := r.Group("")
	g.Use(IdentifySubject(v), Authorize(auth.DefaultTestEnforcer()))
	g.GET("/api/v1/housekeeping/rooms", func(c *gin.Context) {
		ac := GetAuthContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"role": ac.Role, "subject": ac.Subject})
	})
	return r
}

// BE-R01: kredensial palsu tidak memberi hak staf; token sesi valid memberi role dari server.
func TestIdentifySubject_StaffTokens(t *testing.T) {
	tests := []struct {
		name       string
		verifier   StaffVerifier
		headers    map[string]string
		wantStatus int
		wantBody   string
	}{
		{name: "Bearer nama role literal ditolak", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer gm_admin"}, wantStatus: http.StatusForbidden},
		{name: "Bearer staff:role ditolak", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer staff:gm_admin:x"}, wantStatus: http.StatusForbidden},
		{name: "X-User-Role ditolak", verifier: newFakeStaffAuth(), headers: map[string]string{"X-User-Role": "gm_admin"}, wantStatus: http.StatusForbidden},
		{name: "X-User-Role + secret internal ditolak", verifier: newFakeStaffAuth(), headers: map[string]string{"X-User-Role": "gm_admin", "X-Internal-Secret": "internal-service-secret"}, wantStatus: http.StatusForbidden},
		{name: "X-Testing-Role ditolak", verifier: newFakeStaffAuth(), headers: map[string]string{"X-User-Role": "gm_admin", "X-Testing-Role": "true"}, wantStatus: http.StatusForbidden},
		{name: "token valid gm_admin", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer stf_valid_gm"}, wantStatus: http.StatusOK, wantBody: `"role":"gm_admin"`},
		{name: "subject berasal dari username server", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer stf_valid_gm", "X-User-ID": "attacker"}, wantStatus: http.StatusOK, wantBody: `"subject":"staff:admin_gm"`},
		{name: "token valid receptionist tidak boleh akses gm-only", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer stf_valid_fo"}, wantStatus: http.StatusForbidden},
		{name: "token stf_ tidak dikenal → 401", verifier: newFakeStaffAuth(), headers: map[string]string{"Authorization": "Bearer stf_forged"}, wantStatus: http.StatusUnauthorized, wantBody: "AUTHENTICATION_REQUIRED"},
		{name: "verifier error infrastruktur → 503", verifier: &fakeStaffAuth{verErr: errors.New("db down")}, headers: map[string]string{"Authorization": "Bearer stf_valid_gm"}, wantStatus: http.StatusServiceUnavailable, wantBody: "AUTH_UNAVAILABLE"},
		{name: "verifier tidak terpasang + token stf_ → 503 (fail-closed)", verifier: nil, headers: map[string]string{"Authorization": "Bearer stf_valid_gm"}, wantStatus: http.StatusServiceUnavailable},
		{name: "tanpa credential → tamu → 403", verifier: newFakeStaffAuth(), headers: nil, wantStatus: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := probeRouter(tc.verifier)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/housekeeping/rooms", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body %s tidak memuat %s", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func staffRouter(svc StaffAuthService) http.Handler {
	return NewRouter(Deps{StaffAuth: svc, Enforcer: auth.DefaultTestEnforcer(), IsDevelopment: true})
}

func doJSON(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestStaffLoginHandler(t *testing.T) {
	tests := []struct {
		name       string
		svc        *fakeStaffAuth
		body       string
		wantStatus int
		wantBody   string
		wantHeader map[string]string
	}{
		{name: "sukses", svc: newFakeStaffAuth(), body: `{"username":"fo","password":"correct-horse-battery"}`, wantStatus: 200,
			wantBody: `"token":"stf_issued"`, wantHeader: map[string]string{"Cache-Control": "no-store"}},
		{name: "role dikembalikan", svc: newFakeStaffAuth(), body: `{"username":"fo","password":"correct-horse-battery"}`, wantStatus: 200, wantBody: `"role":"receptionist"`},
		{name: "kredensial salah", svc: newFakeStaffAuth(), body: `{"username":"fo","password":"nope"}`, wantStatus: 401, wantBody: "INVALID_CREDENTIALS"},
		{name: "JSON rusak", svc: newFakeStaffAuth(), body: `{`, wantStatus: 400, wantBody: "INVALID_REQUEST"},
		{name: "field kosong", svc: newFakeStaffAuth(), body: `{"username":"","password":""}`, wantStatus: 400, wantBody: "INVALID_REQUEST"},
		{name: "akun terkunci", svc: &fakeStaffAuth{loginErr: &staffauth.LockedError{Until: time.Now().Add(10 * time.Minute)}}, body: `{"username":"fo","password":"x"}`, wantStatus: 429, wantBody: "ACCOUNT_LOCKED", wantHeader: map[string]string{"Retry-After": "600"}},
		{name: "store error", svc: &fakeStaffAuth{loginErr: errors.New("db down")}, body: `{"username":"fo","password":"x"}`, wantStatus: 503, wantBody: "AUTH_UNAVAILABLE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(staffRouter(tc.svc), http.MethodPost, "/api/v1/auth/staff/login", tc.body, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body %s tidak memuat %s", rec.Body.String(), tc.wantBody)
			}
			for k, v := range tc.wantHeader {
				got := rec.Header().Get(k)
				if k == "Retry-After" {
					if n, err := strconv.Atoi(got); err != nil || n < 590 || n > 600 {
						t.Errorf("header %s = %q, want 590..600", k, got)
					}
					continue
				}
				if got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestStaffMeAndLogout(t *testing.T) {
	svc := newFakeStaffAuth()
	router := staffRouter(svc)
	auth := map[string]string{"Authorization": "Bearer stf_valid_fo"}

	if rec := doJSON(router, http.MethodGet, "/api/v1/auth/staff/me", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me tanpa token = %d, want 401", rec.Code)
	}
	if rec := doJSON(router, http.MethodGet, "/api/v1/auth/staff/me", "", map[string]string{"Authorization": "Bearer gm_admin"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me dengan role literal = %d, want 401", rec.Code)
	}
	rec := doJSON(router, http.MethodGet, "/api/v1/auth/staff/me", "", auth)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"username":"fo"`) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("me = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(router, http.MethodPost, "/api/v1/auth/staff/logout", "", auth); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", rec.Code)
	}
	if len(svc.revoked) != 1 || svc.revoked[0] != "stf_valid_fo" {
		t.Errorf("revoked = %v", svc.revoked)
	}
	if rec := doJSON(router, http.MethodGet, "/api/v1/auth/staff/me", "", auth); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me setelah logout = %d, want 401", rec.Code)
	}
	if rec := doJSON(router, http.MethodPost, "/api/v1/auth/staff/logout", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("logout tanpa token = %d, want 401", rec.Code)
	}
}
