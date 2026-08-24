package app_test

import (
	"context"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/app"
)

type fakeMarket struct{}

func (fakeMarket) Bars(_ context.Context, _ string, _ int, _ time.Time) ([]tf.Bar, error) {
	return nil, nil
}

type fakeConstituents struct{}

func (fakeConstituents) Constituents(_ context.Context, _ string, _ time.Time) ([]string, error) {
	return nil, nil
}

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error)                 { return "", nil }
func (fakeConfig) Watch(string, any, any) (tf.Watcher, error) { return nil, nil }

type nopBarSource struct{}

func (nopBarSource) Bar(string, time.Time) (tf.Bar, bool) { return tf.Bar{}, false }

func TestNewApp_DefaultsToBacktest(t *testing.T) {
	a := app.NewApp()
	a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: func(tf.Context, time.Time) ([]tf.Order, error) {
		return nil, nil
	}})
	if err := a.Run(); err == nil {
		t.Fatal("expected error from missing backtest range")
	}
}

func TestApp_BacktestEndToEnd(t *testing.T) {
	a := app.NewApp(
		app.WithMode(tf.Backtest),
		app.WithBacktestRange(
			time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
		),
		app.WithMarket(fakeMarket{}),
		app.WithConstituents(fakeConstituents{}),
		app.WithConfig(fakeConfig{}),
		app.WithBarSource(nopBarSource{}),
	)
	if err := a.RegisterJob(tf.Job{
		Name: "demo",
		Time: tf.Time(16, 0),
		Handler: func(_ tf.Context, _ time.Time) ([]tf.Order, error) {
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
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

func TestApp_LiveRequiresBroker(t *testing.T) {
	a := app.NewApp(
		app.WithMode(tf.Live),
		app.WithMarket(fakeMarket{}),
		app.WithConstituents(fakeConstituents{}),
		app.WithConfig(fakeConfig{}),
	)
	a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: func(tf.Context, time.Time) ([]tf.Order, error) {
		return nil, nil
	}})
	if err := a.Run(); err == nil {
		t.Fatal("expected error for missing broker in live mode")
	}
}

func TestApp_RegisterJobRejectsBadJob(t *testing.T) {
	a := app.NewApp()
	if err := a.RegisterJob(tf.Job{Name: "", Time: tf.Time(9, 0), Handler: func(tf.Context, time.Time) ([]tf.Order, error) {
		return nil, nil
	}}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestApp_RegisterJobRejectsDuplicates(t *testing.T) {
	a := app.NewApp()
	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }
	if err := a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: dummy}); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: dummy}); err == nil {
		t.Fatal("expected duplicate error")
	}
}
