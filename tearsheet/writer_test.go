package tearsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/tearsheet"
)

// A writer that restarts finds its history back.
//
// This is what a live service does when it is restarted: the second
// writer loads the first one's events, so the tear sheet carries the
// whole run and not only the part since the restart.
func TestWriter_RestartLoadsTheHistoryBack(t *testing.T) {
	dir := t.TempDir()
	store := newStore(t)
	ctx := context.Background()

	day1 := time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 5, 21, 0, 0, 0, time.UTC)

	// The first run of the service: one trading day, then it stops.
	first := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "momentum/live", tearsheet.Run{Strategy: "momentum", Mode: "live"})
	first.OnEquity(day1, 10_000)
	first.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day1, Commission: 1})
	if err := first.Flush(); err != nil {
		t.Fatal(err)
	}

	// The service starts again. It knows nothing until it loads.
	second := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "momentum/live", tearsheet.Run{Strategy: "momentum", Mode: "live"})
	if err := second.Load(ctx); err != nil {
		t.Fatal(err)
	}
	second.OnEquity(day2, 10_250)

	rep := second.Report(tearsheet.Meta{Mode: "live"})
	if len(rep.Equity) != 2 {
		t.Fatalf("equity points=%d want 2", len(rep.Equity))
	}
	if rep.Equity[0].Value != 10_000 || rep.Equity[1].Value != 10_250 {
		t.Fatalf("equity=%+v", rep.Equity)
	}
	// The fill the first run recorded is still known, so the trade it
	// opened is still open.
	if len(rep.Trades) != 0 {
		t.Fatalf("trades=%d want 0", len(rep.Trades))
	}
}

// A restored run is matched the same way the run that recorded it was:
// the fill that closed the position is still matched against it.
func TestWriter_RestartMatchesTheTradesTheSameWay(t *testing.T) {
	dir := t.TempDir()
	store := newStore(t)
	ctx := context.Background()

	day1 := time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 5, 21, 0, 0, 0, time.UTC)

	first := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "momentum/live", tearsheet.Run{Strategy: "momentum", Mode: "live"})
	first.OnEquity(day1, 10_000)
	first.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day1, Commission: 1})
	if err := first.Flush(); err != nil {
		t.Fatal(err)
	}

	second := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "momentum/live", tearsheet.Run{Strategy: "momentum", Mode: "live"})
	if err := second.Load(ctx); err != nil {
		t.Fatal(err)
	}
	second.OnEquity(day2, 10_250)
	second.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 110, Time: day2, Commission: 1})

	rep := second.Report(tearsheet.Meta{Mode: "live"})
	if len(rep.Trades) != 1 {
		t.Fatalf("trades=%d want 1", len(rep.Trades))
	}
	trade := rep.Trades[0]
	if trade.Symbol != "AAPL" || trade.Side != "LONG" || trade.Quantity != 10 {
		t.Fatalf("trade=%+v", trade)
	}
	if trade.PnL != 100-2 {
		t.Fatalf("trade pnl=%v want %v", trade.PnL, 100-2)
	}
}

// One strategy's history is never read as another's.
func TestWriter_HistoryIsPerStrategy(t *testing.T) {
	dir := t.TempDir()
	store := newStore(t)
	ctx := context.Background()
	day := time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC)

	momentum := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "momentum/live", tearsheet.Run{Strategy: "momentum", Mode: "live"})
	momentum.OnEquity(day, 10_000)
	if err := momentum.Flush(); err != nil {
		t.Fatal(err)
	}

	value := tearsheet.NewWriter(dir, func(time.Time) string { return "live.html" }, nil).
		WithStore(store, "value/live", tearsheet.Run{Strategy: "value", Mode: "live"})
	if err := value.Load(ctx); err != nil {
		t.Fatal(err)
	}

	rep := value.Report(tearsheet.Meta{Mode: "live"})
	if len(rep.Equity) != 0 {
		t.Fatalf("equity points=%d want 0", len(rep.Equity))
	}
}

// A writer without a store keeps its events to itself.
func TestWriter_WithoutAStore(t *testing.T) {
	w := tearsheet.NewWriter(t.TempDir(), func(time.Time) string { return "live.html" }, nil)
	w.OnEquity(time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), 10_000)
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	rep := w.Report(tearsheet.Meta{Mode: "live"})
	if len(rep.Equity) != 1 {
		t.Fatalf("equity points=%d want 1", len(rep.Equity))
	}
}

// The tear sheet of a run is written where the writer was told.
func TestWriter_WritesTheFile(t *testing.T) {
	dir := t.TempDir()
	w := tearsheet.NewWriter(dir, func(day time.Time) string {
		return "live-" + day.Format("2006-01-02") + ".html"
	}, nil)
	w.OnEquity(time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), 10_000)
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "live-2026-01-02.html")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("tear sheet not written: %v", err)
	}
}
