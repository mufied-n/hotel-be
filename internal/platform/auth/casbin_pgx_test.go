package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/casbin/casbin/v2/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockRows struct {
	rows [][]string
	idx  int
	err  error
}

func (m *mockRows) Close() {}
func (m *mockRows) Err() error { return m.err }
func (m *mockRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) Next() bool {
	if m.idx < len(m.rows) {
		m.idx++
		return true
	}
	return false
}
func (m *mockRows) Scan(dest ...any) error {
	row := m.rows[m.idx-1]
	for i := range dest {
		if i < len(row) {
			if s, ok := dest[i].(*string); ok {
				*s = row[i]
			}
		}
	}
	return nil
}
func (m *mockRows) Values() ([]any, error) { return nil, nil }
func (m *mockRows) RawValues() [][]byte    { return nil }
func (m *mockRows) Conn() *pgx.Conn        { return nil }

type mockDB struct {
	rows     [][]string
	queryErr error
	execErr  error
	lastSQL  string
	lastArgs []any
}

func (m *mockDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	m.lastSQL = sql
	m.lastArgs = args
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return &mockRows{rows: m.rows}, nil
}

func (m *mockDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.lastSQL = sql
	m.lastArgs = args
	return pgconn.CommandTag{}, m.execErr
}

func TestCasbinRBACPolicyEvaluation(t *testing.T) {
	// Sample seed policies for Pulang ke Uttara
	policies := [][]string{
		{"p", "guest", "/api/v1/availability", "GET"},
		{"p", "guest", "/api/v1/bookings", "POST"},
		{"p", "guest", "/api/v1/bookings/:id", "GET"},
		{"p", "guest", "/api/v1/bookings/:id/cancel", "POST"},
		{"p", "guest", "/fake-pay/:ref", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/check-out", "POST"},
		{"p", "receptionist", "/api/v1/bookings/:id/no-show", "POST"},
		{"p", "housekeeping", "/api/v1/rooms/housekeeping", "GET"},
		{"p", "housekeeping", "/api/v1/rooms/:room_number/housekeeping", "PUT"},
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
	}

	e, err := NewInMemoryEnforcer(policies)
	if err != nil {
		t.Fatalf("failed to create in-memory enforcer: %v", err)
	}

	tests := []struct {
		name     string
		sub      string
		obj      string
		act      string
		expected bool
	}{
		// Guest tests
		{"guest can view availability", "guest", "/api/v1/availability", "GET", true},
		{"guest can create booking", "guest", "/api/v1/bookings", "POST", true},
		{"guest can view booking by id", "guest", "/api/v1/bookings/01900000-0000-7000-8000-000000000001", "GET", true},
		{"guest can cancel booking", "guest", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/cancel", "POST", true},
		{"guest CANNOT check-in", "guest", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in", "POST", false},
		{"guest CANNOT check-out", "guest", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-out", "POST", false},
		{"guest CANNOT mark no-show", "guest", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/no-show", "POST", false},
		{"guest CANNOT access housekeeping", "guest", "/api/v1/rooms/housekeeping", "GET", false},

		// Receptionist tests (with role inheritance: g, receptionist, guest)
		{"receptionist can check-in", "receptionist", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-in", "POST", true},
		{"receptionist can check-out", "receptionist", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/check-out", "POST", true},
		{"receptionist can mark no-show", "receptionist", "/api/v1/bookings/01900000-0000-7000-8000-000000000001/no-show", "POST", true},
		{"receptionist inherits guest availability access", "receptionist", "/api/v1/availability", "GET", true},
		{"receptionist inherits guest booking access", "receptionist", "/api/v1/bookings/01900000-0000-7000-8000-000000000001", "GET", true},
		{"receptionist CANNOT update rates", "receptionist", "/api/v1/rates", "PUT", false},
		{"receptionist CANNOT access finance reports", "receptionist", "/api/v1/reports/revenue", "GET", false},

		// Housekeeping tests
		{"housekeeping can view status", "housekeeping", "/api/v1/rooms/housekeeping", "GET", true},
		{"housekeeping can update room", "housekeeping", "/api/v1/rooms/301/housekeeping", "PUT", true},
		{"housekeeping CANNOT check-in", "housekeeping", "/api/v1/bookings/123/check-in", "POST", false},

		// Revenue Manager tests
		{"revenue_mgr can update rates", "revenue_mgr", "/api/v1/rates", "PUT", true},
		{"revenue_mgr CANNOT check-in", "revenue_mgr", "/api/v1/bookings/123/check-in", "POST", false},

		// Finance tests
		{"finance can access reports", "finance", "/api/v1/reports/daily", "GET", true},
		{"finance CANNOT update rates", "finance", "/api/v1/rates", "PUT", false},

		// General Manager (Super Admin) tests
		{"gm_admin can check-in", "gm_admin", "/api/v1/bookings/123/check-in", "POST", true},
		{"gm_admin can update rates", "gm_admin", "/api/v1/rates", "PUT", true},
		{"gm_admin can access reports", "gm_admin", "/api/v1/reports/annual", "GET", true},
		{"gm_admin can do anything", "gm_admin", "/api/v1/custom-admin-path", "DELETE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := e.Enforce(tt.sub, tt.obj, tt.act)
			if err != nil {
				t.Fatalf("enforce error: %v", err)
			}
			if ok != tt.expected {
				t.Errorf("Enforce(%q, %q, %q) = %v; want %v", tt.sub, tt.obj, tt.act, ok, tt.expected)
			}
		})
	}
}

func TestPgxAdapterOperations(t *testing.T) {
	db := &mockDB{
		rows: [][]string{
			{"p", "guest", "/api/v1/availability", "GET", "", "", ""},
			{"p", "receptionist", "/api/v1/bookings/:id/check-in", "POST", "", "", ""},
			{"g", "receptionist", "guest", "", "", "", ""},
		},
	}

	adapter := NewPgxAdapter(db)

	m, err := model.NewModelFromString(DefaultRBACModelText)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// 1. Test LoadPolicy
	if err := adapter.LoadPolicy(m); err != nil {
		t.Fatalf("LoadPolicy failed: %v", err)
	}

	// 2. Test SavePolicy (no-op)
	if err := adapter.SavePolicy(m); err != nil {
		t.Errorf("SavePolicy expected nil, got %v", err)
	}

	// 3. Test AddPolicy
	if err := adapter.AddPolicy("p", "p", []string{"finance", "/api/v1/reports/*", "GET"}); err != nil {
		t.Errorf("AddPolicy failed: %v", err)
	}

	// 4. Test RemovePolicy
	if err := adapter.RemovePolicy("p", "p", []string{"finance", "/api/v1/reports/*", "GET"}); err != nil {
		t.Errorf("RemovePolicy failed: %v", err)
	}

	// 5. Test RemoveFilteredPolicy
	if err := adapter.RemoveFilteredPolicy("p", "p", 0, "finance"); err != nil {
		t.Errorf("RemoveFilteredPolicy failed: %v", err)
	}
	if err := adapter.RemoveFilteredPolicy("p", "p", 99, "invalid"); err == nil {
		t.Error("RemoveFilteredPolicy with invalid index should return error")
	}

	// 6. Test LoadPolicy error handling
	errDB := &mockDB{queryErr: errors.New("connection failed")}
	errAdapter := NewPgxAdapter(errDB)
	if err := errAdapter.LoadPolicy(m); err == nil {
		t.Error("LoadPolicy should fail when query fails")
	}

	// 7. Test NewEnforcer factory
	enforcer, err := NewEnforcer(db, "")
	if err != nil {
		t.Fatalf("NewEnforcer failed: %v", err)
	}
	allowed, err := enforcer.Enforce("guest", "/api/v1/availability", "GET")
	if err != nil || !allowed {
		t.Errorf("Enforce through NewEnforcer failed: %v (allowed: %v)", err, allowed)
	}

	// 8. Test NewEnforcer with non-existent file (falls back to default model string)
	enforcerFallback, err := NewEnforcer(db, "non_existent_path.conf")
	if err != nil {
		t.Fatalf("NewEnforcer fallback failed: %v", err)
	}
	if enforcerFallback == nil {
		t.Error("expected valid enforcer from fallback")
	}
}
