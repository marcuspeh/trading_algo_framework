package app_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// stubCalendar is a calendar provider that answers from a fixed set of
// closed dates.
type stubCalendar struct {
	closed map[string]bool
	err    error

	mu    sync.Mutex
	asked []string
}

func (c *stubCalendar) TradingDay(d time.Time) (bool, error) {
	c.mu.Lock()
	c.asked = append(c.asked, d.Format("2006-01-02"))
	c.mu.Unlock()
	if c.err != nil {
		return false, c.err
	}
	return !c.closed[d.Format("2006-01-02")], nil
}

func (c *stubCalendar) EarlyClose(d time.Time) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	return false, nil
}

func (c *stubCalendar) SessionOpen(d time.Time) (time.Time, error) {
	if c.err != nil {
		return time.Time{}, c.err
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, calendar.NYSE), nil
}

func (c *stubCalendar) SessionClose(d time.Time) (time.Time, error) {
	if c.err != nil {
		return time.Time{}, c.err
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 16, 0, 0, 0, calendar.NYSE), nil
}

// The provider decides which days the framework trades on.
func TestApp_CalendarProviderDecidesTradingDays(t *testing.T) {
	cal := &stubCalendar{closed: map[string]bool{"2024-01-03": true}}

	var fired []string
	handler := func(_ job.Context, date time.Time) ([]order.Order, error) {
		fired = append(fired, date.Format("2006-01-02"))
		return nil, nil
	}

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
		app.WithCalendarProvider(cal),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: handler}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, d := range fired {
		if d == "2024-01-03" {
			t.Fatalf("job fired on %s, a day the provider closed", d)
		}
	}
	if len(fired) == 0 {
		t.Fatal("job never fired")
	}
}

// The calendar is required: without one the framework has no schedule
// to trade on, so the run does not start.
func TestApp_CalendarProviderIsRequired(t *testing.T) {
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
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: noopJob}); err != nil {
		t.Fatal(err)
	}

	err := a.Run()
	if err == nil {
		t.Fatal("expected the run to fail without a calendar provider")
	}
	if !strings.Contains(err.Error(), "calendar provider is required") {
		t.Fatalf("error = %v, want it to name the missing calendar provider", err)
	}
}

// A provider that cannot answer is fatal: the run fails with the
// provider's error instead of trading on an unconfirmed schedule.
func TestApp_CalendarProviderFailureFailsTheRun(t *testing.T) {
	boom := errors.New("calendar service is down")
	cal := &stubCalendar{err: boom}

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
		app.WithCalendarProvider(cal),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{Name: "demo", Time: job.Time(16, 0), Handler: noopJob}); err != nil {
		t.Fatal(err)
	}

	err := a.Run()
	if err == nil {
		t.Fatal("expected the run to fail when the calendar provider fails")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
}

// A strategy that trades on days the exchange was closed still gets a
// session to resolve its slots against: the standard session stands in
// for the days the provider has no answer for.
func TestApp_TradingDayOverrideTradesOnACalendarClosedDay(t *testing.T) {
	// 2024-01-06 is a Saturday the exchange was closed.
	closed := "2024-01-06"

	var fired []string
	a := app.NewApp(
		app.WithTradingDayFunc(func(d time.Time) bool {
			return d.Format("2006-01-02") == closed
		}),
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "demo", Time: job.AtClose(),
		Handler: func(_ job.Context, d time.Time) ([]order.Order, error) {
			fired = append(fired, d.Format("2006-01-02"))
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(fired) != 1 || fired[0] != closed {
		t.Fatalf("fired on %v want [%s]", fired, closed)
	}
}

// An explicit close wins over the standard session that stands in for
// the days the provider has no answer for.
func TestApp_MarketCloseFuncWinsOverTheStandardSession(t *testing.T) {
	closed := "2024-01-06"

	var fired []time.Time
	a := app.NewApp(
		app.WithTradingDayFunc(func(d time.Time) bool {
			return d.Format("2006-01-02") == closed
		}),
		app.WithMarketCloseFunc(func(d time.Time) time.Time {
			return time.Date(d.Year(), d.Month(), d.Day(), 15, 0, 0, 0, calendar.NYSE)
		}),
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "demo", Time: job.AtClose(),
		Handler: func(_ job.Context, d time.Time) ([]order.Order, error) {
			fired = append(fired, d)
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(fired) != 1 {
		t.Fatalf("fired %d times, want 1: %v", len(fired), fired)
	}
	if fired[0].Hour() != 15 || fired[0].Minute() != 0 {
		t.Errorf("fired at %v want 15:00 from the close override", fired[0])
	}
}
