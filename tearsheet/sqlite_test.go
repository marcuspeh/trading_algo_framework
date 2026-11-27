package tearsheet_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/tearsheet"
)

// A run that is saved is loaded back as it was.
func TestSQLiteStore_SaveAndLoad(t *testing.T) {
	store := newStore(t)

	run := tearsheet.Run{
		Strategy: "momentum",
		Mode:     "live",
		Start:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Starting: 10_000,
		Currency: "USD",
	}
	evts := tearsheet.Events{
		Equity: []tearsheet.Point{
			{Time: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), Value: 10_000, Open: 0},
			{Time: time.Date(2026, 1, 5, 21, 0, 0, 0, time.UTC), Value: 10_250, Open: 2},
		},
		Fills: []order.Fill{
			{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), Commission: 1},
			{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 110, Time: time.Date(2026, 1, 5, 21, 0, 0, 0, time.UTC), Commission: 1},
		},
	}

	if err := store.Save(context.Background(), "momentum/live", run, evts); err != nil {
		t.Fatal(err)
	}

	gotRun, gotEvts, err := store.Load(context.Background(), "momentum/live")
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.Strategy != "momentum" || gotRun.Mode != "live" {
		t.Fatalf("run=%+v", gotRun)
	}
	if gotRun.Starting != 10_000 || gotRun.Currency != "USD" {
		t.Fatalf("run=%+v", gotRun)
	}
	if !gotRun.Start.Equal(run.Start) || !gotRun.End.Equal(run.End) {
		t.Fatalf("run bounds=%v..%v", gotRun.Start, gotRun.End)
	}
	if len(gotEvts.Equity) != 2 {
		t.Fatalf("equity points=%d want 2", len(gotEvts.Equity))
	}
	if gotEvts.Equity[1].Value != 10_250 || gotEvts.Equity[1].Open != 2 {
		t.Fatalf("equity=%+v", gotEvts.Equity[1])
	}
	if len(gotEvts.Fills) != 2 {
		t.Fatalf("fills=%d want 2", len(gotEvts.Fills))
	}
	if gotEvts.Fills[1].Side != order.Sell || gotEvts.Fills[1].Price != 110 {
		t.Fatalf("fill=%+v", gotEvts.Fills[1])
	}
}

// Saving a run twice leaves one copy of it, not two.
func TestSQLiteStore_SaveReplaces(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	run := tearsheet.Run{Strategy: "momentum", Mode: "live"}

	first := tearsheet.Events{Equity: []tearsheet.Point{
		{Time: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), Value: 10_000},
	}}
	if err := store.Save(ctx, "momentum/live", run, first); err != nil {
		t.Fatal(err)
	}

	second := tearsheet.Events{Equity: []tearsheet.Point{
		{Time: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC), Value: 10_000},
		{Time: time.Date(2026, 1, 5, 21, 0, 0, 0, time.UTC), Value: 10_250},
	}}
	if err := store.Save(ctx, "momentum/live", run, second); err != nil {
		t.Fatal(err)
	}

	_, evts, err := store.Load(ctx, "momentum/live")
	if err != nil {
		t.Fatal(err)
	}
	if len(evts.Equity) != 2 {
		t.Fatalf("equity points=%d want 2", len(evts.Equity))
	}
}

// A key that was never saved is empty, not an error.
func TestSQLiteStore_LoadUnknownKey(t *testing.T) {
	store := newStore(t)

	run, evts, err := store.Load(context.Background(), "nothing/here")
	if err != nil {
		t.Fatal(err)
	}
	if run.Strategy != "" || len(evts.Equity) != 0 {
		t.Fatalf("run=%+v evts=%+v", run, evts)
	}
}

// The runs of a strategy are listed, and only its own.
func TestSQLiteStore_Runs(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for _, key := range []string{"momentum/live", "momentum/backtest", "value/live"} {
		if err := store.Save(ctx, key, tearsheet.Run{Strategy: key, Mode: "live"}, tearsheet.Events{}); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := store.Runs(ctx, "momentum/live")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Strategy != "momentum/live" {
		t.Fatalf("runs=%+v", runs)
	}
}

// newStore returns a store backed by a temporary file.
func newStore(t *testing.T) *tearsheet.SQLiteStore {
	t.Helper()
	store, err := tearsheet.NewSQLiteStore(filepath.Join(t.TempDir(), "tearsheets.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
