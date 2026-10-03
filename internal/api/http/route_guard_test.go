package http

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/example/hotel-booking/internal/platform/auth"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/routes.golden")

var paramRe = regexp.MustCompile(`:[A-Za-z_]+`)

// routeLines mengembalikan "METHOD PATH" terurut untuk seluruh route engine.
func routeLines(t *testing.T) []string {
	t.Helper()
	h, _ := setupTestRouter()
	eng, ok := h.(*gin.Engine)
	if !ok {
		t.Fatalf("setupTestRouter handler is %T, want *gin.Engine", h)
	}
	var lines []string
	for _, r := range eng.Routes() {
		lines = append(lines, r.Method+" "+r.Path)
	}
	sort.Strings(lines)
	return lines
}

// TestRoutesGolden menjaga agar refactor tidak mengubah himpunan route (SRS FR-02).
func TestRoutesGolden(t *testing.T) {
	got := strings.Join(routeLines(t), "\n") + "\n"
	path := filepath.Join("testdata", "routes.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (jalankan dengan -update untuk membuat): %v", err)
	}
	if string(want) != got {
		t.Errorf("route table berubah.\n--- want\n%s\n--- got\n%s", want, got)
	}
}

// rbacRoute true bila route dilewati middleware Authorize (apiGroup).
func rbacRoute(path string) bool {
	if strings.HasPrefix(path, "/fake-pay/") {
		return true
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	for _, p := range []string{"/api/v1/webhooks/", "/api/v1/auth/", "/api/v1/guest/"} {
		if strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}

// migrationPolicies mem-parse tuple policy casbin_rule pada bagian "+goose Up" seluruh migrasi.
func migrationPolicies(t *testing.T) [][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		files, err = filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	}
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations tidak ditemukan: %v", err)
	}
	sort.Strings(files)
	tuple := regexp.MustCompile(`\(\s*'([pg])'\s*,\s*'([^']*)'\s*,\s*'([^']*)'(?:\s*,\s*'([^']*)')?`)
	var out [][]string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if i := strings.Index(src, "+goose Down"); i >= 0 {
			src = src[:i]
		}
		if !strings.Contains(src, "casbin_rule") {
			continue
		}
		for _, m := range tuple.FindAllStringSubmatch(src, -1) {
			rule := []string{m[1], m[2], m[3]}
			if m[4] != "" {
				rule = append(rule, m[4])
			}
			out = append(out, rule)
		}
	}
	return out
}

var guardRoles = []string{"guest", "receptionist", "housekeeping", "revenue_mgr", "finance", "gm_admin"}

// TestEveryProtectedRouteHasPolicy memastikan setiap route RBAC punya policy EKSPLISIT
// (di luar catch-all gm_admin "/api/v1/*") atau terdaftar sebagai gm_admin-only yang disengaja (D-08).
func TestEveryProtectedRouteHasPolicy(t *testing.T) {
	var explicit [][]string
	for _, p := range migrationPolicies(t) {
		if p[0] == "p" && p[1] == "gm_admin" && p[2] == "/api/v1/*" {
			continue
		}
		explicit = append(explicit, p)
	}
	enf, err := auth.NewInMemoryEnforcer(explicit)
	if err != nil {
		t.Fatal(err)
	}

	// Route yang memang hanya boleh diakses gm_admin lewat catch-all (keputusan desain).
	// DELETE katalog: revenue_mgr hanya boleh POST/PUT; hapus varian = gm_admin saja.
	gmOnly := map[string]bool{"DELETE /api/v1/catalog/rooms/:id": true}

	var missing []string
	for _, line := range routeLines(t) {
		method, path, _ := strings.Cut(line, " ")
		if !rbacRoute(path) || gmOnly[line] {
			continue
		}
		sample := paramRe.ReplaceAllString(path, "x1")
		allowed := false
		for _, role := range guardRoles {
			if ok, _ := enf.Enforce(role, sample, method); ok {
				allowed = true
				break
			}
		}
		if !allowed {
			missing = append(missing, line)
		}
	}
	for _, m := range missing {
		t.Errorf("route tanpa policy eksplisit: %s", m)
	}
}

// TestAuthorizeFullPathParity: keputusan Casbin untuk URL.Path contoh == untuk template FullPath (D-18).
func TestAuthorizeFullPathParity(t *testing.T) {
	enf, err := auth.NewInMemoryEnforcer(migrationPolicies(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range routeLines(t) {
		method, tmpl, _ := strings.Cut(line, " ")
		if !rbacRoute(tmpl) {
			continue
		}
		sample := paramRe.ReplaceAllString(tmpl, "x1")
		for _, role := range guardRoles {
			byPath, _ := enf.Enforce(role, sample, method)
			byTmpl, _ := enf.Enforce(role, tmpl, method)
			if byPath != byTmpl {
				t.Errorf("parity gagal: role=%s %s: URL.Path=%v FullPath=%v", role, line, byPath, byTmpl)
			}
		}
	}
}

// BenchmarkMiddlewareChain: baseline biaya rantai middleware (jalankan dengan -benchmem).
func BenchmarkMiddlewareChain(b *testing.B) {
	h, _ := setupTestRouter()
	cases := []struct{ name, method, path string }{
		{"healthz_public", http.MethodGet, "/healthz"},
		{"catalog_rbac_guest", http.MethodGet, "/api/v1/catalog/rooms"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequest(tc.method, tc.path, nil)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
			}
		})
	}
}
