package clients_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
)

// fakeMDS mimics the market-data-service /constituents contract:
//   - `etf` and `date` query params are both required
//   - a date with no snapshot yields 404 "No constituents snapshot for ..."
//   - an unsupported symbol yields 400
//   - constituents come back as a flat []string
type fakeMDS struct {
	snapshots map[string]map[string][]string // etf -> date -> tickers
	supported map[string]bool
	requests  []string
}

func (f *fakeMDS) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/constituents", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.requests = append(f.requests, q.Get("etf")+"@"+q.Get("date"))
		if q.Get("etf") == "" || q.Get("date") == "" {
			http.Error(w, `{"detail":[{"msg":"Field required"}]}`, http.StatusUnprocessableEntity)
			return
		}
		if !f.supported[q.Get("etf")] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"detail": "Symbol '" + q.Get("etf") + "' is not supported.",
			})
			return
		}
		tickers, ok := f.snapshots[q.Get("etf")][q.Get("date")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"detail": "'No constituents snapshot for " + q.Get("etf") + " on " + q.Get("date") + "'",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"symbol": q.Get("etf"), "date": q.Get("date"),
			"constituents": tickers, "source": "parquet",
		})
	})
	return mux
}

// writeJSON writes a JSON body, matching the service's error shape.
func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func newFakeMDS(t *testing.T, snapshots map[string]map[string][]string, supported ...string) (*fakeMDS, *httptest.Server) {
	t.Helper()
	sup := map[string]bool{}
	for _, s := range supported {
		sup[s] = true
	}
	f := &fakeMDS{snapshots: snapshots, supported: sup}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestConstituents_SDKReturnsTickers(t *testing.T) {
	_, srv := newFakeMDS(t, map[string]map[string][]string{
		"SPY": {"2024-01-08": {"NVDA", "AAPL", "MSFT"}},
	}, "SPY")

	u := clients.NewConstituents(marketdata.NewClient(srv.URL))
	got, err := u.Constituents(context.Background(), "SPY", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := []string{"NVDA", "AAPL", "MSFT"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// The service stores one snapshot per calendar day, so a date without
// one must resolve to the most recent earlier snapshot.
func TestConstituents_FallsBackToMostRecentSnapshot(t *testing.T) {
	f, srv := newFakeMDS(t, map[string]map[string][]string{
		"SPY": {"2024-01-05": {"OLD"}},
	}, "SPY")

	u := clients.NewConstituents(marketdata.NewClient(srv.URL))
	got, err := u.Constituents(context.Background(), "SPY", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0] != "OLD" {
		t.Fatalf("got %v want [OLD]", got)
	}
	t.Logf("requests: %v", f.requests)
}

// An unsupported symbol is a hard error, not something to walk back on.
func TestConstituents_UnsupportedSymbolFailsFast(t *testing.T) {
	f, srv := newFakeMDS(t, map[string]map[string][]string{
		"SPY": {"2024-01-08": {"AAPL"}},
	}, "SPY")

	u := clients.NewConstituents(marketdata.NewClient(srv.URL))
	_, err := u.Constituents(context.Background(), "NOPE", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for unsupported symbol")
	}
	t.Logf("err=%v", err)
	if len(f.requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %v", len(f.requests), f.requests)
	}
}

func TestConstituents_NoSnapshotAnywhereReturnsNotFound(t *testing.T) {
	_, srv := newFakeMDS(t, map[string]map[string][]string{}, "SPY")

	u := clients.NewConstituents(marketdata.NewClient(srv.URL))
	_, err := u.Constituents(context.Background(), "SPY", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestConstituents_ValidatesInput(t *testing.T) {
	u := clients.NewConstituents(marketdata.NewClient("http://localhost:1"))
	if _, err := u.Constituents(context.Background(), "", time.Now()); err == nil {
		t.Fatal("expected error for empty symbol")
	}
	nilClient := clients.NewConstituents(nil)
	if _, err := nilClient.Constituents(context.Background(), "SPY", time.Now()); err == nil {
		t.Fatal("expected error for nil client")
	}
}

// The adapter must speak the SDK's contract, so a raw SDK client works
// with no wrapper.
func TestConstituents_AcceptsRawSDKClient(t *testing.T) {
	var c clients.Universe = marketdata.NewClient("http://localhost:1")
	if c == nil {
		t.Fatal("unreachable")
	}
}
