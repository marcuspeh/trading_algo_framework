package clients_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/clients"
)

// countingService serves daily bars and records the ranges it is asked
// for.
type countingService struct {
	mu     sync.Mutex
	dates  []time.Time
	ranges []string
}

func (s *countingService) nyMidnight(d time.Time) int64 {
	ny, _ := time.LoadLocation("America/New_York")
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, ny).UnixMilli()
}

func (s *countingService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/market-data/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, _ := time.Parse("2006-01-02", q.Get("from"))
		to, _ := time.Parse("2006-01-02", q.Get("to"))
		ticker := r.URL.Path[len("/market-data/"):]

		s.mu.Lock()
		s.ranges = append(s.ranges, q.Get("from")+".."+q.Get("to"))
		s.mu.Unlock()

		bars := []map[string]any{}
		if ticker == "AAPL" {
			for _, d := range s.dates {
				if !d.Before(from) && !d.After(to) {
					bars = append(bars, map[string]any{
						"ticker": "AAPL", "timestamp": s.nyMidnight(d),
						"open": 100, "high": 110, "low": 90, "close": 105, "volume": 1000,
					})
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ticker": ticker, "from": q.Get("from"), "to": q.Get("to"), "bars": bars,
		})
	})
	return mux
}

func (s *countingService) requested() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.ranges))
	copy(out, s.ranges)
	return out
}

// sessions returns the trading days in the given month.
func sessions(year int, month time.Month, count int) []time.Time {
	out := make([]time.Time, 0, count)
	d := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	for len(out) < count {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, d)
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

// A request for N bars asks the service for a range that covers N
// trading days, computed with the trading calendar rather than a
// day-count estimate.
func TestMarket_WindowCoversTheRequestedSessions(t *testing.T) {
	dates := sessions(2024, time.January, 10)
	svc := &countingService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL))
	asOf := dates[9]
	bars, err := m.Bars(context.Background(), "AAPL", 3, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 3 {
		t.Fatalf("got %d bars want 3", len(bars))
	}
	// The bars are the most recent three.
	for i, want := range dates[7:] {
		if !bars[i].Timestamp.Equal(want.In(bars[0].Timestamp.Location())) &&
			bars[i].Timestamp.Format("2006-01-02") != want.Format("2006-01-02") {
			t.Errorf("bar %d is %s want %s", i,
				bars[i].Timestamp.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

// The adapter caches what it has fetched, so a backtest walking the
// calendar one day at a time costs one bar per day rather than a whole
// window per day.
func TestMarket_CachesBetweenCalls(t *testing.T) {
	dates := sessions(2024, time.January, 20)
	svc := &countingService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL))

	// The first call fetches the window.
	if _, err := m.Bars(context.Background(), "AAPL", 5, dates[9]); err != nil {
		t.Fatal(err)
	}
	first := len(svc.requested())

	// The second call, for the same session, is served from the cache.
	if _, err := m.Bars(context.Background(), "AAPL", 5, dates[9]); err != nil {
		t.Fatal(err)
	}
	if got := len(svc.requested()); got != first {
		t.Errorf("the cache was not used: %d requests, want %d", got, first)
	}

	// A later session extends the cache rather than starting over.
	if _, err := m.Bars(context.Background(), "AAPL", 5, dates[10]); err != nil {
		t.Fatal(err)
	}
	if got := len(svc.requested()); got != first+1 {
		t.Errorf("expected one more request, got %d in total", got)
	}
}

// The adapter can supply prices to a fill model, so the strategy and
// the fill model read the same series.
func TestMarket_SuppliesBarPrices(t *testing.T) {
	dates := sessions(2024, time.January, 5)
	svc := &countingService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL))

	bar, ok := m.Bar("AAPL", dates[2])
	if !ok {
		t.Fatal("expected a bar")
	}
	if bar.Close != 105 {
		t.Errorf("close=%f want 105", bar.Close)
	}

	if _, ok := m.Bar("MSFT", dates[2]); ok {
		t.Error("expected no bar for an unknown symbol")
	}
}
