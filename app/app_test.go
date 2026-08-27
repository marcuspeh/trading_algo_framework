package app_test

import (
	"context"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/app"
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

func (fakeConfig) Get(string) (string, error)                  { return "", nil }
func (fakeConfig) Watch(string, any, any) (job.Watcher, error) { return nil, nil }

type fakeBars struct{}

func (fakeBars) Bar(string, time.Time) (order.Bar, bool) { return order.Bar{}, false }

func noopJob(_ job.Context, _ time.Time) ([]order.Order, error) { return nil, nil }

func TestNewApp_NoConfigErrors(t *testing.T) {
	a := app.NewApp()
	a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: noopJob})
	if err := a.Run(); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestApp_BacktestEndToEnd(t *testing.T) {
	a := app.NewApp(
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
	if err := a.RegisterJob(tf.Job{
		Name: "demo", Time: tf.Time(16, 0), Handler: noopJob,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestApp_LiveRequiresBroker(t *testing.T) {
	a := app.NewApp(
		app.WithLive(app.LiveConfig{}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: noopJob})
	if err := a.Run(); err == nil {
		t.Fatal("expected error for missing broker")
	}
}

func TestApp_RegisterJobRejectsBadJob(t *testing.T) {
	a := app.NewApp()
	if err := a.RegisterJob(tf.Job{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestApp_AddWarmupValidates(t *testing.T) {
	a := app.NewApp()
	if err := a.AddWarmup("", 10); err == nil {
		t.Fatal("expected error for empty symbol")
	}
	if err := a.AddWarmup("AAPL", 0); err == nil {
		t.Fatal("expected error for non-positive lookback")
	}
}
