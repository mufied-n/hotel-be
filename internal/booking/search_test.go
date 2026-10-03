package booking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
)

type mockSearchCatalog struct {
	variants []catalog.RoomVariant
	err      error
}

func (m *mockSearchCatalog) GetVariant(_ context.Context, _ string) (catalog.RoomVariant, error) {
	return catalog.RoomVariant{}, nil
}

func (m *mockSearchCatalog) ListVariants(_ context.Context) ([]catalog.RoomVariant, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.variants, nil
}

type mockSearchInv struct {
	avail []inventory.Availability
	err   error
}

func (m *mockSearchInv) GetByDate(_ context.Context, _ string, _, _ time.Time) ([]inventory.Availability, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.avail, nil
}

type mockSearchRates struct {
	quotes []rates.Quote
	err    error
}

func (m *mockSearchRates) Quote(_ context.Context, _ string, _, _ time.Time) ([]rates.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quotes, nil
}

func TestSearchAvailability_TableDriven(t *testing.T) {
	ctx := context.Background()
	checkIn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	checkOut := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	baseVariant := catalog.RoomVariant{
		ID:          "v-std",
		Code:        "std",
		Name:        "Standard Room",
		MaxCapacity: 2,
		MaxAdults:   2,
		MaxChildren: 1,
	}

	tests := []struct {
		name          string
		cat           *mockSearchCatalog
		inv           *mockSearchInv
		r             *mockSearchRates
		query         SearchQuery
		wantAvailable bool
		wantReason    string
		wantErr       error
	}{
		{
			name: "catalog error returns ErrCatalogUnavailable",
			cat:  &mockSearchCatalog{err: errors.New("db down")},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    1,
				Adults:   2,
			},
			wantErr: ErrCatalogUnavailable,
		},
		{
			name: "overcapacity flagged as EXCEEDS_CAPACITY",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    1,
				Adults:   3, // exceeds max_adults=2
			},
			wantAvailable: false,
			wantReason:    "EXCEEDS_CAPACITY",
		},
		{
			name: "missing inventory flagged as MISSING_INVENTORY",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			inv:  &mockSearchInv{avail: []inventory.Availability{{Date: checkIn, AvailableRooms: 5}}}, // only 1 night
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2, // expects 2 nights
				Rooms:    1,
				Adults:   2,
			},
			wantAvailable: false,
			wantReason:    "MISSING_INVENTORY",
		},
		{
			name: "rates unavailable flagged as RATE_UNAVAILABLE",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			inv: &mockSearchInv{avail: []inventory.Availability{
				{Date: checkIn, AvailableRooms: 5},
				{Date: checkIn.AddDate(0, 0, 1), AvailableRooms: 5},
			}},
			r: &mockSearchRates{err: errors.New("no rates")},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    1,
				Adults:   2,
			},
			wantAvailable: false,
			wantReason:    "RATE_UNAVAILABLE",
		},
		{
			name: "sold out flagged as SOLD_OUT",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			inv: &mockSearchInv{avail: []inventory.Availability{
				{Date: checkIn, AvailableRooms: 0},
				{Date: checkIn.AddDate(0, 0, 1), AvailableRooms: 0},
			}},
			r: &mockSearchRates{quotes: []rates.Quote{
				{RateMinor: 500_000},
				{RateMinor: 500_000},
			}},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    1,
				Adults:   2,
			},
			wantAvailable: false,
			wantReason:    "SOLD_OUT",
		},
		{
			name: "insufficient rooms flagged as INSUFFICIENT_ROOMS",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			inv: &mockSearchInv{avail: []inventory.Availability{
				{Date: checkIn, AvailableRooms: 1},
				{Date: checkIn.AddDate(0, 0, 1), AvailableRooms: 1},
			}},
			r: &mockSearchRates{quotes: []rates.Quote{
				{RateMinor: 500_000},
				{RateMinor: 500_000},
			}},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    2, // requests 2 rooms, only 1 available
				Adults:   2,
			},
			wantAvailable: false,
			wantReason:    "INSUFFICIENT_ROOMS",
		},
		{
			name: "valid and sufficient rooms returns available",
			cat:  &mockSearchCatalog{variants: []catalog.RoomVariant{baseVariant}},
			inv: &mockSearchInv{avail: []inventory.Availability{
				{Date: checkIn, AvailableRooms: 5},
				{Date: checkIn.AddDate(0, 0, 1), AvailableRooms: 5},
			}},
			r: &mockSearchRates{quotes: []rates.Quote{
				{RateMinor: 500_000},
				{RateMinor: 500_000},
			}},
			query: SearchQuery{
				CheckIn:  checkIn,
				CheckOut: checkOut,
				Nights:   2,
				Rooms:    1,
				Adults:   2,
			},
			wantAvailable: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := SearchAvailability(ctx, tc.cat, tc.inv, tc.r, tc.query)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(res) == 0 {
				t.Fatal("expected at least 1 result")
			}
			if res[0].Available != tc.wantAvailable {
				t.Errorf("res[0].Available = %v, want %v", res[0].Available, tc.wantAvailable)
			}
			if res[0].UnavailableReason != tc.wantReason {
				t.Errorf("res[0].UnavailableReason = %q, want %q", res[0].UnavailableReason, tc.wantReason)
			}
		})
	}
}
