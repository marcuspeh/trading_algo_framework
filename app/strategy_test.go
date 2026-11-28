package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/tearsheet"
)

// A strategy that names itself gets its tear sheets named after it.
func TestApp_StrategyIDNamesTheTearsheet(t *testing.T) {
	dir := t.TempDir()

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
		}),
		app.WithMarketClient(bars()),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(dir),
		app.WithStrategyID("momentum"),
	)
	if err := a.RegisterJob(job.Job{
		Name: "trade", Time: job.Time(16, 0), Handler: buysOnTheFirstDayAndSellsOnTheSecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	path := filepath.Join(dir, "momentum-backtest-2024-01-02_to-2024-01-03.html")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the tearsheet is not named after the strategy: %v", err)
	}
}

// A store without a strategy id is refused: a persisted run has to
// belong to a strategy, or the store cannot tell whose history it is.
func TestApp_StoreNeedsAStrategyID(t *testing.T) {
	store, err := tearsheet.NewSQLiteStore(filepath.Join(t.TempDir(), "tearsheets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
		}),
		app.WithMarketClient(bars()),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
		app.WithTearsheetStore(store),
	)
	if err := a.RegisterJob(job.Job{
		Name: "trade", Time: job.Time(16, 0), Handler: buysOnTheFirstDayAndSellsOnTheSecond,
	}); err != nil {
		t.Fatal(err)
	}
	err = a.Run()
	if err == nil {
		t.Fatal("expected the run to be refused without a strategy id")
	}
}

// A backtest with a store leaves its history in it.
func TestApp_BacktestKeepsItsHistoryInTheStore(t *testing.T) {
	store, err := tearsheet.NewSQLiteStore(filepath.Join(t.TempDir(), "tearsheets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
		}),
		app.WithMarketClient(bars()),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
		app.WithStrategyID("momentum"),
		app.WithTearsheetStore(store),
	)
	if err := a.RegisterJob(job.Job{
		Name: "trade", Time: job.Time(16, 0), Handler: buysOnTheFirstDayAndSellsOnTheSecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	runs, err := store.Runs(context.Background(), "momentum")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs=%d want 1", len(runs))
	}
	if runs[0].Mode != "backtest" {
		t.Fatalf("run=%+v", runs[0])
	}
}

// Running the same backtest again replaces what was stored for it.
//
// A backtest is self-contained: it does not carry the last run's
// points with it, so the stored run is the one that was just made.
func TestApp_BacktestReplacesItsStoredRun(t *testing.T) {
	store, err := tearsheet.NewSQLiteStore(filepath.Join(t.TempDir(), "tearsheets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := func() {
		a := app.NewApp(
			app.WithCalendarProvider(published.New()),
			app.WithBacktest(app.BacktestConfig{
				Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				StartingCash: 10_000,
			}),
			app.WithMarketClient(bars()),
			app.WithUniverseClient(fakeUniverse{}),
			app.WithConfigClient(fakeConfig{}),
			app.WithTearsheetDir(t.TempDir()),
			app.WithStrategyID("momentum"),
			app.WithTearsheetStore(store),
		)
		if err := a.RegisterJob(job.Job{
			Name: "trade", Time: job.Time(16, 0), Handler: buysOnTheFirstDayAndSellsOnTheSecond,
		}); err != nil {
			t.Fatal(err)
		}
		if err := a.Run(); err != nil {
			t.Fatalf("run: %v", err)
		}
	}

	run()
	_, first, err := store.Load(context.Background(), "momentum/backtest/2024-01-02_2024-01-03")
	if err != nil {
		t.Fatal(err)
	}
	run()
	_, second, err := store.Load(context.Background(), "momentum/backtest/2024-01-02_2024-01-03")
	if err != nil {
		t.Fatal(err)
	}

	// The run was replaced, not added to: the stored events are the
	// ones of the second run alone.
	if len(second.Equity) != len(first.Equity) {
		t.Fatalf("equity points=%d want %d", len(second.Equity), len(first.Equity))
	}
	if len(second.Fills) != len(first.Fills) {
		t.Fatalf("fills=%d want %d", len(second.Fills), len(first.Fills))
	}
	if len(first.Equity) == 0 {
		t.Fatal("the run stored nothing")
	}
}
