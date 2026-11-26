package app_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// firing records a job that ran and when.
type firing struct {
	name string
	at   time.Time
}

// The reference strategy (trading_algo/algo01-corner2RSI) runs four
// jobs, two of them tied to the close of the session:
//
//	07:30  cancel stale entry orders   (before the open)
//	10:30  liquidate removed constituents (during the session)
//	15:00  preload the day's state      (an hour before the close)
//	16:00  run the strategy             (at the close)
//
// On an early-close day the last two move to 12:00 and 13:00, which is
// what the algo does by hand with an early-close table of its own.
//
// This test expresses that schedule as framework jobs and checks the
// times they fire, on an ordinary day and on the Friday after
// Thanksgiving.
func TestApp_ExpressesTheReferenceSchedule(t *testing.T) {
	// 2026-11-27 is the Friday after Thanksgiving: the exchange closes
	// at 13:00 ET.
	earlyClose := time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE)
	// 2026-12-01 is an ordinary Tuesday.
	ordinary := time.Date(2026, 12, 1, 0, 0, 0, 0, calendar.NYSE)

	run := func(day time.Time) []firing {
		var out []firing

		a := app.NewApp(
			app.WithCalendarProvider(published.New()),
			app.WithBacktest(app.BacktestConfig{
				Start:        day,
				End:          day.AddDate(0, 0, 1),
				StartingCash: 10_000,
			}),
			app.WithMarketClient(clients.NewStaticBars(nil)),
			app.WithUniverseClient(fakeUniverse{}),
			app.WithConfigClient(fakeConfig{}),
			app.WithTearsheetDir(t.TempDir()),
		)

		add := func(name string, slot job.Slot) {
			if err := a.RegisterJob(job.Job{
				Name: name, Time: slot,
				Handler: func(_ job.Context, at time.Time) ([]order.Order, error) {
					out = append(out, firing{name: name, at: at})
					return nil, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
		}
		add("cancel-stale", job.Time(7, 30))
		add("liquidate", job.Time(10, 30))
		add("preload", job.BeforeClose(time.Hour))
		add("run", job.AtClose())

		if err := a.Run(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	at := func(fs []firing, name string) time.Time {
		for _, f := range fs {
			if f.name == name {
				return f.at
			}
		}
		return time.Time{}
	}

	ordinaryFirings := run(ordinary)
	for name, want := range map[string][2]int{
		"cancel-stale": {7, 30},
		"liquidate":    {10, 30},
		"preload":      {15, 0},
		"run":          {16, 0},
	} {
		got := at(ordinaryFirings, name)
		if got.Hour() != want[0] || got.Minute() != want[1] {
			t.Errorf("%s fired at %v want %02d:%02d ET", name, got, want[0], want[1])
		}
	}

	earlyFirings := run(earlyClose)
	for name, want := range map[string][2]int{
		"cancel-stale": {7, 30},
		"liquidate":    {10, 30},
		"preload":      {12, 0},
		"run":          {13, 0},
	} {
		got := at(earlyFirings, name)
		if got.Hour() != want[0] || got.Minute() != want[1] {
			t.Errorf("%s fired at %v want %02d:%02d ET on an early-close day", name, got, want[0], want[1])
		}
	}
}

// A custom session close replaces the exchange calendar's, for a house
// view of the schedule that differs from the published one.
//
// The escape hatch exists because the early closes are computed by
// rule: a venue that closes at a different hour, or a data vendor
// whose early closes are not the exchange's, cannot be expressed any
// other way.
func TestApp_CustomMarketClose(t *testing.T) {
	day := time.Date(2026, 12, 1, 0, 0, 0, 0, calendar.NYSE)

	// A close at 12:00 every day.
	customClose := func(date time.Time) time.Time {
		y, m, d := date.Date()
		return time.Date(y, m, d, 12, 0, 0, 0, calendar.NYSE)
	}

	var fired []firing
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        day,
			End:          day.AddDate(0, 0, 1),
			StartingCash: 10_000,
		}),
		app.WithMarketCloseFunc(customClose),
		app.WithMarketClient(clients.NewStaticBars(nil)),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
	)

	add := func(name string, slot job.Slot) {
		if err := a.RegisterJob(job.Job{
			Name: name, Time: slot,
			Handler: func(_ job.Context, at time.Time) ([]order.Order, error) {
				fired = append(fired, firing{name: name, at: at})
				return nil, nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("preload", job.BeforeClose(time.Hour))
	add("run", job.AtClose())

	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	at := func(name string) time.Time {
		for _, f := range fired {
			if f.name == name {
				return f.at
			}
		}
		return time.Time{}
	}
	if got := at("run"); got.Hour() != 12 || got.Minute() != 0 {
		t.Errorf("the close job fired at %v want 12:00 under the custom close", got)
	}
	if got := at("preload"); got.Hour() != 11 || got.Minute() != 0 {
		t.Errorf("the preload job fired at %v want 11:00 under the custom close", got)
	}
}
