package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCatalog_MemoryStore(t *testing.T) {
	variants := DefaultVariants()
	store := NewMemoryStore(variants)

	ctx := context.Background()

	t.Run("list returns all 7 variants", func(t *testing.T) {
		list, err := store.ListVariants(ctx)
		if err != nil {
			t.Fatalf("ListVariants() error = %v", err)
		}
		if len(list) != 7 {
			t.Errorf("len(list) = %d, want 7", len(list))
		}
	})

	tests := []struct {
		name       string
		idOrCode   string
		wantCode   string
		wantErr    bool
		expectedErr error
	}{
		{
			name:     "get by ID: sup-king",
			idOrCode: "01900000-0000-7000-8000-000000000001",
			wantCode: "sup-king",
			wantErr:  false,
		},
		{
			name:     "get by Code: dlx-twin",
			idOrCode: "dlx-twin",
			wantCode: "dlx-twin",
			wantErr:  false,
		},
		{
			name:     "get by Code: pste-suite",
			idOrCode: "pste-suite",
			wantCode: "pste-suite",
			wantErr:  false,
		},
		{
			name:        "not found returns ErrVariantNotFound",
			idOrCode:    "unknown-room",
			wantErr:     true,
			expectedErr: ErrVariantNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := store.GetVariant(ctx, tt.idOrCode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("err = %v, want %v", err, tt.expectedErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Code != tt.wantCode {
				t.Errorf("v.Code = %s, want %s", v.Code, tt.wantCode)
			}
		})
	}
}

func TestDefaultVariants_PropertyConstraints(t *testing.T) {
	variants := DefaultVariants()

	families := make(map[string]bool)
	codes := make(map[string]bool)
	ids := make(map[string]bool)

	for _, v := range variants {
		if ids[v.ID] {
			t.Errorf("duplicate variant ID: %s", v.ID)
		}
		ids[v.ID] = true

		if codes[v.Code] {
			t.Errorf("duplicate variant code: %s", v.Code)
		}
		codes[v.Code] = true

		families[v.FamilyName] = true

		if v.MaxCapacity <= 0 {
			t.Errorf("variant %s max capacity must be > 0", v.Code)
		}
		if v.MaxAdults <= 0 {
			t.Errorf("variant %s max adults must be > 0", v.Code)
		}
		if v.BasePriceMinor <= 0 {
			t.Errorf("variant %s base price must be > 0", v.Code)
		}
		if len(v.Amenities) == 0 {
			t.Errorf("variant %s must have amenities", v.Code)
		}
		if len(v.Photos) == 0 {
			t.Errorf("variant %s must have photos", v.Code)
		}
	}

	if len(families) != 5 {
		t.Errorf("len(families) = %d, want 5 (Superior, Deluxe, Executive, Junior Suite, Presidential Suite)", len(families))
	}
}

type mockCatalogRow struct {
	v   RoomVariant
	err error
}

func (r *mockCatalogRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.v.ID
	*dest[1].(*string) = r.v.Code
	*dest[2].(*string) = r.v.Name
	*dest[3].(*string) = r.v.FamilyName
	*dest[4].(*string) = r.v.BedType
	*dest[5].(*int) = r.v.RoomSizeSqm
	*dest[6].(*int) = r.v.MaxCapacity
	*dest[7].(*int) = r.v.MaxAdults
	*dest[8].(*int) = r.v.MaxChildren
	*dest[9].(*int64) = r.v.BasePriceMinor
	*dest[10].(*string) = r.v.Description
	*dest[11].(*[]byte) = []byte(`["Wi-Fi", "AC"]`)
	*dest[12].(*[]byte) = []byte(`[{"url":"http://test.jpg","alt":"Test"}]`)
	return nil
}

type mockCatalogRows struct {
	variants []RoomVariant
	idx      int
	err      error
}

func (r *mockCatalogRows) Close() {}
func (r *mockCatalogRows) Err() error { return r.err }
func (r *mockCatalogRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *mockCatalogRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockCatalogRows) Next() bool {
	if r.idx < len(r.variants) {
		r.idx++
		return true
	}
	return false
}
func (r *mockCatalogRows) Scan(dest ...any) error {
	v := r.variants[r.idx-1]
	*dest[0].(*string) = v.ID
	*dest[1].(*string) = v.Code
	*dest[2].(*string) = v.Name
	*dest[3].(*string) = v.FamilyName
	*dest[4].(*string) = v.BedType
	*dest[5].(*int) = v.RoomSizeSqm
	*dest[6].(*int) = v.MaxCapacity
	*dest[7].(*int) = v.MaxAdults
	*dest[8].(*int) = v.MaxChildren
	*dest[9].(*int64) = v.BasePriceMinor
	*dest[10].(*string) = v.Description
	*dest[11].(*[]byte) = []byte(`["Wi-Fi", "AC"]`)
	*dest[12].(*[]byte) = []byte(`[{"url":"http://test.jpg","alt":"Test"}]`)
	return nil
}
func (r *mockCatalogRows) Values() ([]any, error) { return nil, nil }
func (r *mockCatalogRows) RawValues() [][]byte    { return nil }
func (r *mockCatalogRows) Conn() *pgx.Conn        { return nil }

type mockIDRow struct {
	id  string
	err error
}

func (r *mockIDRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.id
	return nil
}

type mockCatalogDB struct {
	variants []RoomVariant
	queryErr error
	rowErr   error
	rowID    string
	execTag  pgconn.CommandTag
	execErr  error
}

func (m *mockCatalogDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if m.execErr != nil {
		return pgconn.CommandTag{}, m.execErr
	}
	return m.execTag, nil
}

func (m *mockCatalogDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return &mockCatalogRows{variants: m.variants}, nil
}

func (m *mockCatalogDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if m.rowErr != nil {
		return &mockCatalogRow{err: m.rowErr}
	}
	if m.rowID != "" {
		return &mockIDRow{id: m.rowID}
	}
	if len(m.variants) > 0 {
		return &mockCatalogRow{v: m.variants[0]}
	}
	return &mockCatalogRow{err: pgx.ErrNoRows}
}

func TestMemoryStore_CRUD(t *testing.T) {
	store := NewMemoryStore([]RoomVariant{
		{
			ID:             "id-1",
			Code:           "std-king",
			Name:           "Standard King",
			FamilyName:     "Standard",
			MaxCapacity:    2,
			BasePriceMinor: 500_000,
		},
	})
	ctx := context.Background()

	// 1. Create success
	newVar, err := store.CreateVariant(ctx, RoomVariant{
		Code:           "std-twin",
		Name:           "Standard Twin",
		FamilyName:     "Standard",
		MaxCapacity:    2,
		BasePriceMinor: 500_000,
	})
	if err != nil {
		t.Fatalf("CreateVariant error = %v", err)
	}
	if newVar.ID == "" || newVar.Code != "std-twin" {
		t.Errorf("unexpected created variant: %+v", newVar)
	}

	// 2. Create duplicate code
	_, errDup := store.CreateVariant(ctx, RoomVariant{
		Code:           "std-king",
		Name:           "Another King",
		MaxCapacity:    2,
		BasePriceMinor: 500_000,
	})
	if !errors.Is(errDup, ErrDuplicateCode) {
		t.Errorf("expected ErrDuplicateCode, got %v", errDup)
	}

	// 3. Create invalid data
	invalidCases := []RoomVariant{
		{Code: "", Name: "No Code", MaxCapacity: 2, BasePriceMinor: 500_000},
		{Code: "c", Name: "", MaxCapacity: 2, BasePriceMinor: 500_000},
		{Code: "c", Name: "Zero Cap", MaxCapacity: 0, BasePriceMinor: 500_000},
		{Code: "c", Name: "Zero Price", MaxCapacity: 2, BasePriceMinor: 0},
	}
	for i, c := range invalidCases {
		if _, err := store.CreateVariant(ctx, c); !errors.Is(err, ErrInvalidVariant) {
			t.Errorf("case %d expected ErrInvalidVariant, got %v", i, err)
		}
	}

	// 4. Update success
	updated, err := store.UpdateVariant(ctx, "id-1", RoomVariant{
		Code:           "std-king-renovated",
		Name:           "Standard King Renovated",
		FamilyName:     "Standard",
		MaxCapacity:    3,
		BasePriceMinor: 600_000,
	})
	if err != nil {
		t.Fatalf("UpdateVariant error = %v", err)
	}
	if updated.Code != "std-king-renovated" || updated.BasePriceMinor != 600_000 {
		t.Errorf("unexpected updated: %+v", updated)
	}

	// 5. Update not found
	if _, err := store.UpdateVariant(ctx, "non-existent", RoomVariant{Code: "c", Name: "n", MaxCapacity: 2, BasePriceMinor: 100}); !errors.Is(err, ErrVariantNotFound) {
		t.Errorf("expected ErrVariantNotFound, got %v", err)
	}

	// 6. Update duplicate code against another variant
	if _, err := store.UpdateVariant(ctx, "id-1", RoomVariant{Code: "std-twin", Name: "n", MaxCapacity: 2, BasePriceMinor: 100}); !errors.Is(err, ErrDuplicateCode) {
		t.Errorf("expected ErrDuplicateCode, got %v", err)
	}

	// 7. Update invalid variant
	if _, err := store.UpdateVariant(ctx, "id-1", RoomVariant{Code: "", Name: "", MaxCapacity: 0}); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("expected ErrInvalidVariant, got %v", err)
	}

	// 8. Delete success
	if err := store.DeleteVariant(ctx, "id-1"); err != nil {
		t.Fatalf("DeleteVariant error = %v", err)
	}

	// 9. Delete not found
	if err := store.DeleteVariant(ctx, "id-1"); !errors.Is(err, ErrVariantNotFound) {
		t.Errorf("expected ErrVariantNotFound, got %v", err)
	}
}

func TestPostgresStore_Operations(t *testing.T) {
	v := DefaultVariants()[0]
	db := &mockCatalogDB{variants: []RoomVariant{v}}
	store := &PostgresStore{db: db}

	ctx := context.Background()

	// 1. ListVariants success
	list, err := store.ListVariants(ctx)
	if err != nil {
		t.Fatalf("ListVariants() error = %v", err)
	}
	if len(list) != 1 || list[0].Code != "sup-king" {
		t.Errorf("unexpected list: %+v", list)
	}

	// 2. ListVariants query error
	errDB := &mockCatalogDB{queryErr: errors.New("db error")}
	errStore := &PostgresStore{db: errDB}
	if _, err := errStore.ListVariants(ctx); err == nil {
		t.Errorf("expected error, got nil")
	}

	// 3. GetVariant success
	got, err := store.GetVariant(ctx, "sup-king")
	if err != nil {
		t.Fatalf("GetVariant() error = %v", err)
	}
	if got.Code != "sup-king" {
		t.Errorf("got code %s, want sup-king", got.Code)
	}

	// 4. GetVariant not found
	emptyDB := &mockCatalogDB{}
	emptyStore := &PostgresStore{db: emptyDB}
	if _, err := emptyStore.GetVariant(ctx, "unknown"); !errors.Is(err, ErrVariantNotFound) {
		t.Errorf("expected ErrVariantNotFound, got %v", err)
	}

	// 5. GetVariant DB error
	rowErrDB := &mockCatalogDB{rowErr: errors.New("query row error")}
	rowErrStore := &PostgresStore{db: rowErrDB}
	if _, err := rowErrStore.GetVariant(ctx, "any"); err == nil {
		t.Errorf("expected error, got nil")
	}

	// 6. PostgresStore CreateVariant validation error
	if _, err := store.CreateVariant(ctx, RoomVariant{Code: ""}); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("expected ErrInvalidVariant, got %v", err)
	}

	// 7. PostgresStore CreateVariant success
	createDB := &mockCatalogDB{rowID: "new-uuid-123"}
	createStore := &PostgresStore{db: createDB}
	created, err := createStore.CreateVariant(ctx, RoomVariant{
		Code:           "new-room",
		Name:           "New Room",
		MaxCapacity:    2,
		BasePriceMinor: 500_000,
	})
	if err != nil {
		t.Fatalf("CreateVariant() error = %v", err)
	}
	if created.ID != "new-uuid-123" {
		t.Errorf("created.ID = %s, want new-uuid-123", created.ID)
	}

	// 8. PostgresStore CreateVariant duplicate code error (23505)
	dupDB := &mockCatalogDB{rowErr: &pgconn.PgError{Code: "23505"}}
	dupStore := &PostgresStore{db: dupDB}
	if _, err := dupStore.CreateVariant(ctx, RoomVariant{Code: "dup", Name: "Dup", MaxCapacity: 2, BasePriceMinor: 100}); !errors.Is(err, ErrDuplicateCode) {
		t.Errorf("expected ErrDuplicateCode, got %v", err)
	}

	// 9. PostgresStore UpdateVariant validation error
	if _, err := store.UpdateVariant(ctx, "id-1", RoomVariant{Code: ""}); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("expected ErrInvalidVariant, got %v", err)
	}

	// 10. PostgresStore UpdateVariant success
	updateDB := &mockCatalogDB{
		execTag:  pgconn.NewCommandTag("UPDATE 1"),
		variants: []RoomVariant{{ID: "id-1", Code: "updated-code"}},
	}
	updateStore := &PostgresStore{db: updateDB}
	up, err := updateStore.UpdateVariant(ctx, "id-1", RoomVariant{Code: "updated-code", Name: "Updated", MaxCapacity: 2, BasePriceMinor: 100})
	if err != nil {
		t.Fatalf("UpdateVariant error = %v", err)
	}
	if up.Code != "updated-code" {
		t.Errorf("up.Code = %s, want updated-code", up.Code)
	}

	// 11. PostgresStore UpdateVariant not found
	upNotFoundDB := &mockCatalogDB{execTag: pgconn.NewCommandTag("UPDATE 0")}
	upNotFoundStore := &PostgresStore{db: upNotFoundDB}
	if _, err := upNotFoundStore.UpdateVariant(ctx, "id-missing", RoomVariant{Code: "c", Name: "n", MaxCapacity: 2, BasePriceMinor: 100}); !errors.Is(err, ErrVariantNotFound) {
		t.Errorf("expected ErrVariantNotFound, got %v", err)
	}

	// 12. PostgresStore DeleteVariant success
	delDB := &mockCatalogDB{execTag: pgconn.NewCommandTag("DELETE 1")}
	delStore := &PostgresStore{db: delDB}
	if err := delStore.DeleteVariant(ctx, "id-1"); err != nil {
		t.Fatalf("DeleteVariant error = %v", err)
	}

	// 13. PostgresStore DeleteVariant foreign key in use (23503)
	fkDB := &mockCatalogDB{execErr: &pgconn.PgError{Code: "23503"}}
	fkStore := &PostgresStore{db: fkDB}
	if err := fkStore.DeleteVariant(ctx, "id-in-use"); !errors.Is(err, ErrCannotDelete) {
		t.Errorf("expected ErrCannotDelete, got %v", err)
	}

	// 14. PostgresStore DeleteVariant not found
	delNotFoundDB := &mockCatalogDB{execTag: pgconn.NewCommandTag("DELETE 0")}
	delNotFoundStore := &PostgresStore{db: delNotFoundDB}
	if err := delNotFoundStore.DeleteVariant(ctx, "id-missing"); !errors.Is(err, ErrVariantNotFound) {
		t.Errorf("expected ErrVariantNotFound, got %v", err)
	}
}

func TestGenerateUUIDv7_RFC9562Properties(t *testing.T) {
	ctx := context.Background()
	generateFromStore := func() string {
		store := NewMemoryStore(nil)
		v, err := store.CreateVariant(ctx, RoomVariant{
			Code:           "var-" + uuid.NewV7().String()[:8],
			Name:           "Test Variant",
			MaxCapacity:    2,
			BasePriceMinor: 500_000,
		})
		if err != nil {
			t.Fatalf("CreateVariant failed: %v", err)
		}
		return v.ID
	}

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "conforms to 36-character canonical UUID format",
			fn: func(t *testing.T) {
				id := generateFromStore()
				if len(id) != 36 {
					t.Fatalf("expected length 36, got %d for %s", len(id), id)
				}
				if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
					t.Fatalf("invalid hyphen placement in UUIDv7: %s", id)
				}
			},
		},
		{
			name: "version 7 flag (RFC 9562) at index 14 is '7'",
			fn: func(t *testing.T) {
				id := generateFromStore()
				if id[14] != '7' {
					t.Fatalf("expected version character '7' at index 14, got %c in %s", id[14], id)
				}
			},
		},
		{
			name: "variant 1 (RFC 4122/9562: 10xx) at index 19 is one of [8, 9, a, b]",
			fn: func(t *testing.T) {
				id := generateFromStore()
				varByte := id[19]
				if varByte != '8' && varByte != '9' && varByte != 'a' && varByte != 'b' {
					t.Fatalf("expected variant character in [8, 9, a, b] at index 19, got %c in %s", varByte, id)
				}
			},
		},
		{
			name: "time-sortable monotonicity across consecutive generations",
			fn: func(t *testing.T) {
				u1 := generateFromStore()
				time.Sleep(2 * time.Millisecond)
				u2 := generateFromStore()
				if u1 >= u2 {
					t.Errorf("expected u1 (%s) < u2 (%s) in time-sortable UUIDv7", u1, u2)
				}
			},
		},
		{
			name: "collision-free in rapid batch generation",
			fn: func(t *testing.T) {
				seen := make(map[string]bool)
				for i := 0; i < 100; i++ {
					id := generateFromStore()
					if seen[id] {
						t.Fatalf("collision detected on iteration %d: %s", i, id)
					}
					seen[id] = true
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

