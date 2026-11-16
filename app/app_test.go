package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

type fakeMarket struct{}

func (fakeMarket) Bars(_ context.Context, _ string, _ int, _ time.Time) ([]order.Bar, error) {
	return nil, nil
}

type fakeUniverse struct{}

func (fakeUniverse) Constituents(_ context.Context, _ string, _ time.Time) ([]string, error) {
	return nil, nil
}

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error) { return "", nil }

type fakeBars struct{}

func (fakeBars) Bar(string, time.Time) (order.Bar, bool) { return order.Bar{}, false }

func noopJob(_ job.Context, _ time.Time) ([]order.Order, error) { return nil, nil }

func TestNewApp_NoConfigErrors(t *testing.T) {
	a := app.NewApp(app.WithCalendarProvider(published.New()))
	a.RegisterJob(job.Job{Name: "x", Time: job.Time(9, 0), Handler: noopJob})
	if err := a.Run(); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestApp_BacktestEndToEnd(t *testing.T) {
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.RegisterJob(job.Job{
		Name: "demo", Time: job.Time(16, 0), Handler: noopJob,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestApp_LiveRequiresBroker(t *testing.T) {
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithLive(app.LiveConfig{}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	a.RegisterJob(job.Job{Name: "x", Time: job.Time(9, 0), Handler: noopJob})
	if err := a.Run(); err == nil {
		t.Fatal("expected error for missing broker")
	}
}

func TestApp_RegisterJobRejectsBadJob(t *testing.T) {
	a := app.NewApp(app.WithCalendarProvider(published.New()))
	if err := a.RegisterJob(job.Job{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestApp_RegisterJobRejectsDuplicateName(t *testing.T) {
	a := app.NewApp(app.WithCalendarProvider(published.New()))
	if err := a.RegisterJob(job.Job{Name: "x", Time: job.Time(9, 0), Handler: noopJob}); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterJob(job.Job{Name: "x", Time: job.Time(16, 0), Handler: noopJob}); err == nil {
		t.Fatal("expected error for duplicate job name")
	}
}

func TestApp_AddWarmupValidates(t *testing.T) {
	a := app.NewApp(app.WithCalendarProvider(published.New()))
	if err := a.AddWarmup("", 10); err == nil {
		t.Fatal("expected error for empty symbol")
	}
	if err := a.AddWarmup("AAPL", 0); err == nil {
		t.Fatal("expected error for non-positive lookback")
	}
}

// TestApp_BacktestContextReportsBacktestMode verifies the Mode() method
// that the README documents.
func TestApp_BacktestContextReportsBacktestMode(t *testing.T) {
	var got job.Mode
	handler := func(ctx job.Context, _ time.Time) ([]order.Order, error) {
		got = ctx.Mode()
		return nil, nil
	}

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: handler}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if got != job.ModeBacktest {
		t.Fatalf("Mode()=%q want %q", got, job.ModeBacktest)
	}
}

// TestApp_WarmupFetchesHistory verifies AddWarmup actually pulls data
// rather than silently recording it.
func TestApp_WarmupFetchesHistory(t *testing.T) {
	mkt := &countingMarket{}
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(mkt),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.AddWarmup("AAPL", 20); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: noopJob}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if mkt.calls == 0 {
		t.Fatal("expected warmup to fetch bars")
	}
}

type countingMarket struct {
	calls int
}

func (c *countingMarket) Bars(_ context.Context, _ string, _ int, _ time.Time) ([]order.Bar, error) {
	c.calls++
	return []order.Bar{
		{Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Close: 100},
	}, nil
}
