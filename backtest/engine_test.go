package backtest_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// Regression test for the double-clock bug: the backtest engine used to
// create its own BacktestClock, so the Context the strategy received
// stayed frozen on the start date and every order filled against the
// first day's bar.
func TestBacktest_ContextSeesCurrentSimulatedDate(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

	var seen []time.Time
	handler := func(ctx job.Context, date time.Time) ([]order.Order, error) {
		seen = append(seen, ctx.Today())
		return nil, nil
	}

	a := app.NewApp(
		app.WithBacktest(app.BacktestConfig{
			Start:        start,
			End:          end,
			StartingCash: 10_000,

		}),
		app.WithMarketClient(clients.NewStaticBars(map[string][]order.Bar{})),
		app.WithUniverseClient(clients.NewStaticConstituents(nil)),
		app.WithConfigClient(&noopConfig{}),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: handler}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	want := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE),
		time.Date(2024, 1, 3, 0, 0, 0, 0, clock.NYSE),
		time.Date(2024, 1, 4, 0, 0, 0, 0, clock.NYSE),
		time.Date(2024, 1, 5, 0, 0, 0, 0, clock.NYSE),
	}
	if len(seen) != len(want) {
		t.Fatalf("handler ran %d times, want %d", len(seen), len(want))
	}
	for i := range want {
		if !seen[i].Equal(want[i]) {
			t.Fatalf("day %d: ctx.Today()=%v want %v", i, seen[i], want[i])
		}
	}
}

// Regression test: orders must fill against the bar of the day they
// were placed, not the bar of the backtest's start date.
func TestBacktest_OrderFillsAgainstCurrentDayBar(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

	bars := map[string][]order.Bar{
		"AAPL": {
			{Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Open: 100, High: 100, Low: 100, Close: 100},
			{Timestamp: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Open: 200, High: 200, Low: 200, Close: 200},
			{Timestamp: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC), Open: 300, High: 300, Low: 300, Close: 300},
		},
	}

	// Cash observed at the start of each day tells us what the previous
	// day's order actually filled at.
	var cashByDay []float64
	handler := func(ctx job.Context, date time.Time) ([]order.Order, error) {
		cashByDay = append(cashByDay, ctx.Cash())
		if date.Day() == 3 {
			return []order.Order{{
				Symbol: "AAPL", Side: order.Buy, Quantity: 1, Type: order.Market,
			}}, nil
		}
		return nil, nil
	}

	a := app.NewApp(
		app.WithBacktest(app.BacktestConfig{
			Start:        start,
			End:          end,
			StartingCash: 10_000,

		}),
		app.WithMarketClient(clients.NewStaticBars(bars)),
		app.WithUniverseClient(clients.NewStaticConstituents(nil)),
		app.WithConfigClient(&noopConfig{}),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: handler}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	if len(cashByDay) != 3 {
		t.Fatalf("handler ran %d times, want 3", len(cashByDay))
	}
	if cashByDay[0] != 10_000 {
		t.Fatalf("day 1 cash=%f want 10000", cashByDay[0])
	}
	// The order placed on Jan 3 must fill at 200 (the Jan 3 close), so
	// cash on Jan 4 is 10000 - 200 = 9800. Before the fix it filled at
	// 100, the Jan 2 close.
	if cashByDay[2] != 9_800 {
		t.Fatalf("day 3 cash=%f want 9800 (fill at Jan 3 close of 200)", cashByDay[2])
	}
}

type noopConfig struct{}

func (noopConfig) Get(string) (string, error)                  { return "", job.ErrNotFound }
