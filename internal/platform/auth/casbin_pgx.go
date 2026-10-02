package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB adalah interface minimal untuk eksekusi query PostgreSQL (kompatibel dengan *pgxpool.Pool).
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DefaultRBACModelText adalah model RBAC Casbin standar untuk RESTful API Pulang ke Uttara.
// Mendukung pencocokan path dinamis (keyMatch2) dan role inheritance (g).
const DefaultRBACModelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")
`

// PgxAdapter mengimplementasikan persist.Adapter Casbin menggunakan pgxpool.Pool secara native.
// ponytail: Hindari dependency ORM eksternal (GORM, SQLx); gunakan connection pool pgx yang sudah ada.
type PgxAdapter struct {
	db DB
}

// NewPgxAdapter membuat instance PgxAdapter baru.
func NewPgxAdapter(db DB) *PgxAdapter {
	return &PgxAdapter{db: db}
}

// LoadPolicy memuat seluruh rule dari tabel casbin_rule ke model Casbin in-memory.
func (a *PgxAdapter) LoadPolicy(m model.Model) error {
	ctx := context.Background()
	rows, err := a.db.Query(ctx, "SELECT ptype, v0, v1, v2, v3, v4, v5 FROM casbin_rule ORDER BY id ASC")
	if err != nil {
		return fmt.Errorf("casbin load policy query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ptype, v0, v1, v2, v3, v4, v5 string
		if err := rows.Scan(&ptype, &v0, &v1, &v2, &v3, &v4, &v5); err != nil {
			return fmt.Errorf("casbin scan row: %w", err)
		}
		raw := []string{v0, v1, v2, v3, v4, v5}
		last := len(raw)
		for last > 0 && raw[last-1] == "" {
			last--
		}
		rule := append([]string{ptype}, raw[:last]...)
		if err := persist.LoadPolicyArray(rule, m); err != nil {
			return fmt.Errorf("casbin load policy array: %w", err)
		}
	}
	return rows.Err()
}

// SavePolicy diabaikan karena rule hotel dikelola via migrasi SQL versi terstruktur (read-mostly).
func (a *PgxAdapter) SavePolicy(m model.Model) error {
	return nil
}

// AddPolicy menambahkan satu aturan kebijakan ke tabel casbin_rule.
func (a *PgxAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	vals := make([]string, 6)
	copy(vals, rule)
	ctx := context.Background()
	_, err := a.db.Exec(ctx,
		`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING`,
		ptype, vals[0], vals[1], vals[2], vals[3], vals[4], vals[5],
	)
	return err
}

// RemovePolicy menghapus aturan kebijakan dari tabel casbin_rule.
func (a *PgxAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	vals := make([]string, 6)
	copy(vals, rule)
	ctx := context.Background()
	_, err := a.db.Exec(ctx,
		`DELETE FROM casbin_rule
		 WHERE ptype = $1 AND v0 = $2 AND v1 = $3 AND v2 = $4 AND v3 = $5 AND v4 = $6 AND v5 = $7`,
		ptype, vals[0], vals[1], vals[2], vals[3], vals[4], vals[5],
	)
	return err
}

// RemoveFilteredPolicy menghapus aturan kebijakan berdasarkan filter field.
func (a *PgxAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	if fieldIndex < 0 || fieldIndex > 5 {
		return fmt.Errorf("invalid field index: %d", fieldIndex)
	}
	var conditions []string
	args := []any{ptype}
	for i, val := range fieldValues {
		idx := fieldIndex + i
		if idx > 5 {
			break
		}
		if val != "" {
			conditions = append(conditions, fmt.Sprintf("v%d = $%d", idx, len(args)+1))
			args = append(args, val)
		}
	}
	query := "DELETE FROM casbin_rule WHERE ptype = $1"
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}
	ctx := context.Background()
	_, err := a.db.Exec(ctx, query, args...)
	return err
}

// NewEnforcer menginisialisasi SyncedEnforcer thread-safe dengan adapter pgx.
func NewEnforcer(db DB, modelPath string) (*casbin.SyncedEnforcer, error) {
	var m model.Model
	var err error

	if modelPath != "" {
		m, err = model.NewModelFromFile(modelPath)
		if err != nil {
			// Fallback ke default model text jika file tidak ditemukan
			m, err = model.NewModelFromString(DefaultRBACModelText)
		}
	} else {
		m, err = model.NewModelFromString(DefaultRBACModelText)
	}
	if err != nil {
		return nil, fmt.Errorf("create casbin model: %w", err)
	}

	adapter := NewPgxAdapter(db)
	e, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("new synced enforcer: %w", err)
	}
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}
	return e, nil
}

// NewInMemoryEnforcer membuat SyncedEnforcer murni in-memory untuk pengujian unit cepat tanpa database.
func NewInMemoryEnforcer(policies [][]string) (*casbin.SyncedEnforcer, error) {
	m, err := model.NewModelFromString(DefaultRBACModelText)
	if err != nil {
		return nil, fmt.Errorf("create in-memory model: %w", err)
	}
	e, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("new in-memory enforcer: %w", err)
	}
	for _, p := range policies {
		if len(p) < 2 {
			continue
		}
		ptype := p[0]
		sec := ptype[:1]
		if err := e.GetModel().AddPolicy(sec, ptype, p[1:]); err != nil {
			return nil, fmt.Errorf("add in-memory policy %v: %w", p, err)
		}
	}
	if err := e.BuildRoleLinks(); err != nil {
		return nil, fmt.Errorf("build role links: %w", err)
	}
	return e, nil
}

// DefaultTestEnforcer mengembalikan in-memory enforcer dengan kebijakan default hotel untuk pengujian unit.
func DefaultTestEnforcer() *casbin.SyncedEnforcer {
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
		{"p", "revenue_mgr", "/api/v1/rates", "PUT"},
		{"p", "finance", "/api/v1/reports/*", "GET"},
		{"p", "gm_admin", "/api/v1/*", "*"},
		{"g", "receptionist", "guest"},
	}
	e, _ := NewInMemoryEnforcer(policies)
	return e
}
