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
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
)

// batchService mimics the market-data-service /market-data/batch
// endpoint: it answers a set of symbols in one request, and a symbol it
// cannot serve comes back in errors rather than failing the batch.
type batchService struct {
	dates   []time.Time
	failing map[string]bool

	mu      sync.Mutex
	batches [][]string
}

func (s *batchService) nyMidnight(d time.Time) int64 {
	ny, _ := time.LoadLocation("America/New_York")
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, ny).UnixMilli()
}

func (s *batchService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/market-data/batch", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Symbols []string `json:"symbols"`
			From    string   `json:"from"`
			To      string   `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.batches = append(s.batches, body.Symbols)
		s.mu.Unlock()

		from, _ := time.Parse("2006-01-02", body.From)
		to, _ := time.Parse("2006-01-02", body.To)

		results := []map[string]any{}
		errors := []map[string]any{}
		for _, ticker := range body.Symbols {
			if s.failing[ticker] {
				errors = append(errors, map[string]any{
					"ticker": ticker, "status_code": 502, "detail": "upstream error",
				})
				continue
			}
			bars := []map[string]any{}
			for _, d := range s.dates {
				if !d.Before(from) && !d.After(to) {
					bars = append(bars, map[string]any{
						"ticker": ticker, "timestamp": s.nyMidnight(d),
						"open": 100.0, "high": 101.0, "low": 99.0, "close": 100.0,
						"volume": 1.0, "source": "cache",
					})
				}
			}
			results = append(results, map[string]any{
				"ticker": ticker, "from": body.From, "to": body.To,
				"backfilled_bars": 0, "bars": bars,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": results, "errors": errors,
		})
	})
	return mux
}

func (s *batchService) requested() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, len(s.batches))
	copy(out, s.batches)
	return out
}

// A whole universe is asked for in one request, and every symbol comes
// back with its own bars.
func TestMarket_BarsBatchServesEverySymbolInOneRequest(t *testing.T) {
	dates := sessions(2024, time.January, 10)
	svc := &batchService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())

	got, err := m.BarsBatch(context.Background(), []string{"AAPL", "MSFT"}, 3, dates[9])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d symbols, want 2", len(got))
	}
	for _, symbol := range []string{"AAPL", "MSFT"} {
		if len(got[symbol]) != 3 {
			t.Errorf("%s: got %d bars, want 3", symbol, len(got[symbol]))
		}
	}
	if batches := svc.requested(); len(batches) != 1 {
		t.Fatalf("the service was asked %d times, want 1", len(batches))
	}
}

// A symbol the service cannot serve is left out rather than failing the
// whole batch: one name without history does not stop the others.
func TestMarket_BarsBatchLeavesOutASymbolTheServiceCannotServe(t *testing.T) {
	dates := sessions(2024, time.January, 10)
	svc := &batchService{dates: dates, failing: map[string]bool{"MSFT": true}}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())

	got, err := m.BarsBatch(context.Background(), []string{"AAPL", "MSFT"}, 3, dates[9])
	if err != nil {
		t.Fatalf("the batch failed for one bad symbol: %v", err)
	}
	if _, ok := got["MSFT"]; ok {
		t.Error("MSFT should not be in the map")
	}
	if _, ok := got["AAPL"]; !ok {
		t.Error("AAPL should be in the map")
	}
}

// The bars of a batch keep the symbol they belong to, so a strategy
// reading the map cannot mix two names up.
func TestMarket_BarsBatchKeepsTheSymbolOnEachBar(t *testing.T) {
	dates := sessions(2024, time.January, 10)
	svc := &batchService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())

	got, err := m.BarsBatch(context.Background(), []string{"AAPL", "MSFT"}, 2, dates[9])
	if err != nil {
		t.Fatal(err)
	}
	for symbol, bars := range got {
		for _, b := range bars {
			if b.Symbol != symbol {
				t.Errorf("bar of %s is stamped %s", symbol, b.Symbol)
			}
		}
	}
}
