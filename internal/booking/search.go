package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/hotel-booking/internal/catalog"
	"github.com/example/hotel-booking/internal/inventory"
	"github.com/example/hotel-booking/internal/rates"
)

// SearchQuery mendefinisikan parameter input untuk pencarian kamar (BE-G02, FR-14).
type SearchQuery struct {
	CheckIn  time.Time
	CheckOut time.Time
	Nights   int
	Rooms    int
	Adults   int
	Children int
}

// SearchResultItem merepresentasikan opsi kamar hasil pencarian lintas varian (BE-G02).
type SearchResultItem struct {
	RoomVariant       catalog.RoomVariant `json:"room_variant"`
	Available         bool                `json:"available"`
	AvailableRooms    int                 `json:"available_rooms"`
	UnavailableReason string              `json:"unavailable_reason,omitempty"`
	TotalPriceMinor   int64               `json:"total_price_minor"`
	Currency          string              `json:"currency"`
	Quotes            []rates.Quote       `json:"quotes"`
}

func sumQuotes(quotes []rates.Quote) int64 {
	var total int64
	for _, q := range quotes {
		total += q.RateMinor
	}
	return total
}

// SearchAvailability mengeksekusi use case pencarian ketersediaan varian kamar (BE-G02, FR-14).
func SearchAvailability(ctx context.Context, cat CatalogReader, inv inventory.AvailabilityStore, r rates.RateProvider, q SearchQuery) ([]SearchResultItem, error) {
	if cat == nil {
		return nil, ErrCatalogUnavailable
	}
	variants, err := cat.ListVariants(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCatalogUnavailable, err)
	}

	var results []SearchResultItem

	for _, v := range variants {
		item := SearchResultItem{
			RoomVariant: v,
			Currency:    "IDR",
		}

		// Kapasitas okupansi (BE-G03, BE-R07):
		// - Tamu per kamar tidak boleh melebihi max_capacity
		// - Dewasa per kamar tidak boleh melebihi max_adults
		// - Anak per kamar tidak boleh melebihi max_children
		totalGuests := q.Adults + q.Children
		if totalGuests > v.MaxCapacity*q.Rooms || q.Adults > v.MaxAdults*q.Rooms || q.Children > v.MaxChildren*q.Rooms {
			item.Available = false
			item.UnavailableReason = "EXCEEDS_CAPACITY"
			results = append(results, item)
			continue
		}

		if inv == nil {
			item.Available = false
			item.UnavailableReason = "MISSING_INVENTORY"
			results = append(results, item)
			continue
		}

		// Periksa ketersediaan multi-malam kontinu (BE-G02)
		avail, err := inv.GetByDate(ctx, v.ID, q.CheckIn, q.CheckOut)
		if errors.Is(err, inventory.ErrNotFound) {
			item.Available = false
			item.UnavailableReason = "MISSING_INVENTORY"
			results = append(results, item)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInventoryUnavailable, err)
		}
		if len(avail) < q.Nights {
			item.Available = false
			item.UnavailableReason = "MISSING_INVENTORY"
			results = append(results, item)
			continue
		}

		if r == nil {
			item.Available = false
			item.UnavailableReason = "RATE_UNAVAILABLE"
			results = append(results, item)
			continue
		}

		// Hitung kuotasi tarif (BE-R09: pricing guard)
		quotes, err := r.Quote(ctx, v.ID, q.CheckIn, q.CheckOut)
		if err != nil || len(quotes) == 0 {
			item.Available = false
			item.UnavailableReason = "RATE_UNAVAILABLE"
			results = append(results, item)
			continue
		}
		item.Quotes = quotes
		item.TotalPriceMinor = sumQuotes(quotes) * int64(q.Rooms)
		if item.TotalPriceMinor <= 0 {
			item.Available = false
			item.UnavailableReason = "RATE_UNAVAILABLE"
			results = append(results, item)
			continue
		}

		// Cari sisa kamar minimum sepanjang rentang menginap
		minAvail := avail[0].AvailableRooms
		for _, a := range avail {
			if a.AvailableRooms < minAvail {
				minAvail = a.AvailableRooms
			}
		}

		item.AvailableRooms = minAvail
		if minAvail >= q.Rooms {
			item.Available = true
		} else if minAvail == 0 {
			item.Available = false
			item.UnavailableReason = "SOLD_OUT"
		} else {
			item.Available = false
			item.UnavailableReason = "INSUFFICIENT_ROOMS"
		}

		results = append(results, item)
	}

	return results, nil
}

// SearchAvailability adalah method use case pada Service (FR-14).
func (s *Service) SearchAvailability(ctx context.Context, q SearchQuery) ([]SearchResultItem, error) {
	return SearchAvailability(ctx, s.catalogStore, s.inv, s.rates, q)
}
