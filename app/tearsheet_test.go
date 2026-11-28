package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// bars are two days of AAPL: 100 and then 110.
func bars() *clients.StaticBars {
	return clients.NewStaticBars(map[string][]order.Bar{
		"AAPL": {
			{Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Open: 99, High: 101, Low: 98, Close: 100, Volume: 1000},
			{Timestamp: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Open: 105, High: 112, Low: 104, Close: 110, Volume: 1000},
		},
	})
}

// buysOnTheFirstDayAndSellsOnTheSecond is a strategy that takes a
// position and lets it go.
func buysOnTheFirstDayAndSellsOnTheSecond(_ job.Context, date time.Time) ([]order.Order, error) {
	if date.Day() == 2 {
		return []order.Order{{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market}}, nil
	}
	return []order.Order{{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Type: order.Market}}, nil
}

// TestApp_BacktestWritesATearsheet checks that a backtest leaves a tear
// sheet behind, with the run's numbers on it.
func TestApp_BacktestWritesATearsheet(t *testing.T) {
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
	)
	if err := a.RegisterJob(job.Job{
		Name: "trade", Time: job.Time(16, 0), Handler: buysOnTheFirstDayAndSellsOnTheSecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	path := filepath.Join(dir, "backtest-2024-01-02_to-2024-01-03.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the tearsheet: %v", err)
	}

	page := string(body)
	for _, want := range []string{"Total return", "AAPL", "Portfolio value", "Trades"} {
		if !strings.Contains(page, want) {
			t.Errorf("expected the tearsheet to contain %q", want)
		}
	}
	// The trade made 100 on a 10000 account.
	if !strings.Contains(page, "+1.00%") {
		t.Error("expected the tearsheet to carry the run's return")
	}
}
