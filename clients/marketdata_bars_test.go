package clients_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
)

// barsService mimics the market-data-service /market-data endpoint:
// it returns bars for the trading dates inside [from, to], stamped at
// NYSE midnight like the real service.
type barsService struct {
	dates   []time.Time
	seen    []string
	barOnly bool
}

func (s *barsService) nyMidnight(d time.Time) int64 {
	ny, _ := time.LoadLocation("America/New_York")
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, ny).UnixMilli()
}

func (s *barsService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/market-data/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.seen = append(s.seen, q.Get("from")+".."+q.Get("to"))
		from, _ := time.Parse("2006-01-02", q.Get("from"))
		to, _ := time.Parse("2006-01-02", q.Get("to"))
		bars := []map[string]any{}
		for _, d := range s.dates {
			if !d.Before(from) && !d.After(to) {
				bars = append(bars, map[string]any{
					"ticker": "AAPL", "timestamp": s.nyMidnight(d),
					"open": 100.0, "high": 101.0, "low": 99.0, "close": 100.0,
					"volume": 1.0, "source": "cache",
				})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ticker": "AAPL", "from": q.Get("from"), "to": q.Get("to"),
			"backfilled_bars": 0, "bars": bars,
		})
	})
	return mux
}

func tradingDaysIn(from, to time.Time) []time.Time {
	out := []time.Time{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, d)
		}
	}
	return out
}

// The request window has to cover `lookback` trading days once
// weekends and holidays are discounted. It used to be `lookback*2`
// calendar days, which came up short.
func TestBars_LookbackReturnsFullCount(t *testing.T) {
	dates := tradingDaysIn(
		time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
	)
	svc := &barsService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())
	asOf := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	for _, lb := range []int{1, 3, 5, 8, 20} {
		bars, err := m.Bars(context.Background(), "AAPL", lb, asOf)
		if err != nil {
			t.Fatalf("lookback=%d: err=%v", lb, err)
		}
		if len(bars) != lb {
			t.Errorf("lookback=%d: got %d bars, want %d", lb, len(bars), lb)
		}
	}
}

// A one-bar lookback on a Monday must work: the request window has to
// reach back past the weekend.
func TestBars_LookbackOneOnMonday(t *testing.T) {
	dates := tradingDaysIn(
		time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
	)
	svc := &barsService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())
	for _, d := range []string{"2024-01-08", "2024-01-15", "2024-01-22"} {
		asOf, _ := time.Parse("2006-01-02", d)
		bars, err := m.Bars(context.Background(), "AAPL", 1, asOf)
		if err != nil {
			t.Errorf("%s: err=%v", d, err)
			continue
		}
		if len(bars) != 1 {
			t.Errorf("%s: got %d bars, want 1", d, len(bars))
		}
	}
}

// Bar timestamps must carry the trading day as their calendar date, in
// NYSE terms, so they compare cleanly against the clock's date.
func TestBars_TimestampAnchoredOnNYSE(t *testing.T) {
	dates := []time.Time{time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)}
	svc := &barsService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())
	asOf := time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC)
	bars, err := m.Bars(context.Background(), "AAPL", 1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("got %d bars", len(bars))
	}
	ny, _ := time.LoadLocation("America/New_York")
	if got := bars[0].Timestamp.In(ny).Format("2006-01-02"); got != "2024-01-08" {
		t.Errorf("bar NY date=%s want 2024-01-08", got)
	}
	if got := bars[0].Timestamp.Location().String(); got != "America/New_York" {
		t.Errorf("bar location=%s want America/New_York", got)
	}
}
