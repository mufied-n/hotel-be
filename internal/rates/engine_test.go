package rates

import (
	"context"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestQuote_WeekdayAndWeekend(t *testing.T) {
	// 2026-10-09 = Jumat, 2026-10-10 = Sabtu; 2026-10-11 = Minggu (hari kerja), 12 = Senin
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.25)

	quotes, err := eng.Quote(context.Background(), "std", date("2026-10-09"), date("2026-10-13"))
	if err != nil {
		t.Fatalf("Quote error: %v", err)
	}
	want := []int64{125_000, 125_000, 100_000, 100_000} // Fri, Sat, Sun, Mon
	if len(quotes) != len(want) {
		t.Fatalf("len quotes = %d, want %d", len(quotes), len(want))
	}
	for i, q := range quotes {
		if q.RateMinor != want[i] {
			t.Errorf("quote[%d] (%s) = %d, want %d", i, q.Date.Format("2006-01-02"), q.RateMinor, want[i])
		}
	}
}

func TestQuote_HalfOpen(t *testing.T) {
	eng := NewEngine(map[string]int64{"std": 100_000}, 1.0)
	quotes, _ := eng.Quote(context.Background(), "std", date("2026-10-10"), date("2026-10-11"))
	if len(quotes) != 1 {
		t.Fatalf("1 malam → %d quote, want 1", len(quotes))
	}
}

func TestQuote_UnknownRoomType(t *testing.T) {
	eng := NewEngine(map[string]int64{}, 1.0)
	if _, err := eng.Quote(context.Background(), "xxx", date("2026-10-10"), date("2026-10-11")); err == nil {
		t.Fatal("unknown room type harus error")
	}
}
