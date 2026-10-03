package featureflag

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleContext(t *testing.T) {
	ctx := context.Background()
	if r := RoleFromContext(ctx); r != "" {
		t.Fatalf("expected empty role, got %s", r)
	}

	ctx = WithRole(ctx, "gm_admin")
	if r := RoleFromContext(ctx); r != "gm_admin" {
		t.Fatalf("expected gm_admin, got %s", r)
	}

	// Test fallback string key
	ctx2 := context.WithValue(context.Background(), "user_role", "receptionist")
	if r := RoleFromContext(ctx2); r != "receptionist" {
		t.Fatalf("expected receptionist, got %s", r)
	}
}

func TestMemoryManager_Evaluation(t *testing.T) {
	flags := map[string]Flag{
		"flag_disabled": {
			Key:          "flag_disabled",
			Enabled:      false,
			AllowedRoles: []string{},
		},
		"flag_global": {
			Key:          "flag_global",
			Enabled:      true,
			AllowedRoles: []string{},
		},
		"flag_roles": {
			Key:          "flag_roles",
			Enabled:      true,
			AllowedRoles: []string{"gm_admin", "finance"},
		},
		"flag_roles_disabled": {
			Key:          "flag_roles_disabled",
			Enabled:      false,
			AllowedRoles: []string{"gm_admin"},
		},
	}

	mgr := NewMemoryManager(flags)

	tests := []struct {
		name     string
		flagKey  string
		role     string
		expected bool
	}{
		{
			name:     "unknown flag returns false",
			flagKey:  "non_existent",
			role:     "gm_admin",
			expected: false,
		},
		{
			name:     "disabled global flag returns false",
			flagKey:  "flag_disabled",
			role:     "gm_admin",
			expected: false,
		},
		{
			name:     "enabled global flag returns true for empty role",
			flagKey:  "flag_global",
			role:     "",
			expected: true,
		},
		{
			name:     "enabled global flag returns true for guest",
			flagKey:  "flag_global",
			role:     "guest",
			expected: true,
		},
		{
			name:     "role-scoped flag returns true for matching role gm_admin",
			flagKey:  "flag_roles",
			role:     "gm_admin",
			expected: true,
		},
		{
			name:     "role-scoped flag returns true for matching role finance",
			flagKey:  "flag_roles",
			role:     "finance",
			expected: true,
		},
		{
			name:     "role-scoped flag returns false for non-matching role receptionist",
			flagKey:  "flag_roles",
			role:     "receptionist",
			expected: false,
		},
		{
			name:     "role-scoped flag returns false for empty role",
			flagKey:  "flag_roles",
			role:     "",
			expected: false,
		},
		{
			name:     "disabled role-scoped flag returns false even for gm_admin",
			flagKey:  "flag_roles_disabled",
			role:     "gm_admin",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.role != "" {
				ctx = WithRole(ctx, tc.role)
			}
			actual := mgr.IsEnabled(ctx, tc.flagKey)
			if actual != tc.expected {
				t.Errorf("IsEnabled(%q, role=%q) = %v; want %v", tc.flagKey, tc.role, actual, tc.expected)
			}
		})
	}
}

func TestMemoryManager_CRUD(t *testing.T) {
	mgr := NewMemoryManager(nil) // default flags

	ctx := context.Background()

	// List
	list := mgr.List(ctx)
	if len(list) != 17 {
		t.Fatalf("expected 17 default flags, got %d", len(list))
	}

	// Get
	flag, ok := mgr.Get(ctx, "ff_xendit_payment_gateway")
	if !ok {
		t.Fatalf("expected flag ff_xendit_payment_gateway to exist")
	}
	if !flag.Enabled {
		t.Errorf("expected ff_xendit_payment_gateway to be enabled by default")
	}

	// Update existing flag
	updated, err := mgr.Update(ctx, "ff_xendit_payment_gateway", false, []string{"gm_admin"}, "test_admin")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Enabled {
		t.Errorf("expected updated flag to be disabled")
	}
	if len(updated.AllowedRoles) != 1 || updated.AllowedRoles[0] != "gm_admin" {
		t.Errorf("expected updated allowed_roles to be [gm_admin], got %v", updated.AllowedRoles)
	}

	// Verify IsEnabled reflects update
	if mgr.IsEnabled(ctx, "ff_xendit_payment_gateway") {
		t.Errorf("expected ff_xendit_payment_gateway to be disabled globally")
	}

	// Update non-existent flag
	_, err = mgr.Update(ctx, "invalid_flag_key", false, nil, "admin")
	if err == nil {
		t.Errorf("expected error updating non-existent flag, got nil")
	}

	// Reload & Close
	if err := mgr.Reload(ctx); err != nil {
		t.Errorf("Reload failed: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestMemoryManager_ConcurrentAccess(t *testing.T) {
	mgr := NewMemoryManager(nil)
	ctx := WithRole(context.Background(), "gm_admin")

	var wg sync.WaitGroup
	numWorkers := 20
	iterations := 100

	// Readers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = mgr.IsEnabled(ctx, "ff_catalog_write")
				_ = mgr.List(ctx)
				_, _ = mgr.Get(ctx, "ff_multi_variant_search")
			}
		}()
	}

	// Writers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(wId int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				enabled := (j % 2) == 0
				_, _ = mgr.Update(ctx, "ff_catalog_write", enabled, []string{"gm_admin"}, "concurrent_tester")
			}
		}(i)
	}

	wg.Wait()
}

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:dev@172.24.0.3:5432/booking_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping postgres manager test: %v", err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgres manager test: ping failed: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPostgresManager_Integration(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}

	ctx := context.Background()
	mgr, err := NewPostgresManager(ctx, pool, nil, 0, nil)
	if err != nil {
		t.Fatalf("NewPostgresManager failed: %v", err)
	}
	defer mgr.Close()

	list := mgr.List(ctx)
	if len(list) < 17 {
		t.Fatalf("expected at least 17 flags loaded from DB, got %d", len(list))
	}

	flag, ok := mgr.Get(ctx, "ff_catalog_write")
	if !ok {
		t.Fatalf("flag ff_catalog_write not found in postgres manager")
	}

	// Update in DB and verify reload
	origEnabled := flag.Enabled
	origRoles := flag.AllowedRoles

	updated, err := mgr.Update(ctx, "ff_catalog_write", !origEnabled, []string{"gm_admin"}, "integration_test")
	if err != nil {
		t.Fatalf("mgr.Update failed: %v", err)
	}
	if updated.Enabled == origEnabled {
		t.Errorf("expected updated.Enabled = %v, got %v", !origEnabled, updated.Enabled)
	}
	// Test IsEnabled on PostgresManager
	ctxGM := WithRole(ctx, "gm_admin")
	ctxGuest := WithRole(ctx, "guest")
	if !mgr.IsEnabled(ctxGM, "ff_catalog_write") {
		// Since we just updated allowed_roles to ["gm_admin"] and Enabled to !origEnabled:
		// Let's test IsEnabled with explicit values
	}

	// Set to known state: enabled=true, allowed_roles=["gm_admin"]
	_, _ = mgr.Update(ctx, "ff_catalog_write", true, []string{"gm_admin"}, "test")
	if !mgr.IsEnabled(ctxGM, "ff_catalog_write") {
		t.Errorf("expected IsEnabled=true for gm_admin")
	}
	if mgr.IsEnabled(ctxGuest, "ff_catalog_write") {
		t.Errorf("expected IsEnabled=false for guest")
	}
	if mgr.IsEnabled(context.Background(), "ff_catalog_write") {
		t.Errorf("expected IsEnabled=false for empty role")
	}

	// Global flag check
	if !mgr.IsEnabled(ctxGuest, "ff_multi_variant_search") {
		t.Errorf("expected IsEnabled=true for global flag on guest")
	}

	// Disabled flag check
	_, _ = mgr.Update(ctx, "ff_multi_variant_search", false, nil, "test")
	if mgr.IsEnabled(ctxGuest, "ff_multi_variant_search") {
		t.Errorf("expected IsEnabled=false for disabled global flag")
	}
	// Restore
	_, _ = mgr.Update(ctx, "ff_multi_variant_search", true, []string{}, "cleanup")

	// Unknown flag
	if mgr.IsEnabled(ctxGM, "unknown_key_xyz") {
		t.Errorf("expected IsEnabled=false for unknown flag")
	}

	// Update non-existent key returns error
	_, err = mgr.Update(ctx, "unknown_key_xyz", true, nil, "test")
	if err == nil {
		t.Errorf("expected error updating unknown key")
	}

	// Test short polling
	mgrWithPolling, err := NewPostgresManager(ctx, pool, nil, 10*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("NewPostgresManager with polling failed: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	_ = mgrWithPolling.Close()

	// Restore original state
	_, _ = mgr.Update(ctx, "ff_catalog_write", origEnabled, origRoles, "cleanup")
}

