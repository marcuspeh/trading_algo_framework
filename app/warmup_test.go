package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// recordingMkt records the session each call was made for.
type recordingMkt struct {
	calls []time.Time
}

func (m *recordingMkt) Bars(_ context.Context, _ string, lookback int, asOf time.Time) ([]order.Bar, error) {
	m.calls = append(m.calls, asOf)
	bars := make([]order.Bar, lookback)
	for i := range bars {
		bars[i] = order.Bar{Symbol: "AAPL", Timestamp: asOf, Close: 100}
	}
	return bars, nil
}

// The warmup primes the Context's bar cache, and the cache is keyed by
// the session the bars were fetched for. The first job of the first day
// reads the same session, so it must be served from the cache rather
// than refetched.
//
// The engine used to clear the cache at the top of every day, so the
// warmed entry was thrown away before the first job ran.
func TestApp_WarmupIsReusedByTheFirstJob(t *testing.T) {
	m := &recordingMkt{}
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, calendar.NYSE),
			End:          time.Date(2024, 1, 3, 0, 0, 0, 0, calendar.NYSE),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(m),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.AddWarmup("AAPL", 5); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterJob(job.Job{
		Name: "j", Time: job.Time(16, 0),
		Handler: func(ctx job.Context, _ time.Time) ([]order.Order, error) {
			_, _ = ctx.Bars("AAPL", 5)
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	// Two sessions are simulated, so two fetches are expected: one for
	// the warmup and the first job, which share a session, and one for
	// the second day.
	firstDay := time.Date(2024, 1, 2, 0, 0, 0, 0, calendar.NYSE)
	forced := 0
	for _, c := range m.calls {
		if c.Equal(firstDay) {
			forced++
		}
	}
	if forced != 1 {
		t.Errorf("the first session was fetched %d times, want 1 (the warmup should be reused): %v",
			forced, m.calls)
	}
}

// A day can hold a job before the close and one at it. They read
// different sessions - the previous one and the current one - and
// neither may evict the other's cached bars.
func TestApp_WarmupSurvivesJobsOnBothSidesOfTheClose(t *testing.T) {
	m := &recordingMkt{}
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 3, 0, 0, 0, 0, calendar.NYSE),
			End:          time.Date(2024, 1, 4, 0, 0, 0, 0, calendar.NYSE),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(m),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.AddWarmup("AAPL", 5); err != nil {
		t.Fatal(err)
	}

	read := func(name string) job.Handler {
		return func(ctx job.Context, _ time.Time) ([]order.Order, error) {
			_, _ = ctx.Bars("AAPL", 5)
			return nil, nil
		}
	}
	for _, j := range []job.Job{
		{Name: "preopen", Time: job.Time(9, 0), Handler: read("preopen")},
		{Name: "atclose", Time: job.AtClose(), Handler: read("atclose")},
	} {
		if err := a.RegisterJob(j); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	// The first day is read by two jobs on two sessions, and the warmup
	// covers the one the first job reads. So the first session is
	// fetched once, not three times.
	seen := map[string]int{}
	for _, call := range m.calls {
		seen[call.Format("2006-01-02")]++
	}
	for session, n := range seen {
		if n > 1 {
			t.Errorf("session %s was fetched %d times, want 1: %v", session, n, m.calls)
		}
	}
}
