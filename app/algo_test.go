package app_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

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

	type firing struct {
		name string
		at   time.Time
	}

	run := func(day time.Time) []firing {
		var out []firing

		a := app.NewApp(
			app.WithBacktest(app.BacktestConfig{
				Start:        day,
				End:          day.AddDate(0, 0, 1),
				StartingCash: 10_000,
			}),
			app.WithMarketClient(clients.NewStaticBars(nil)),
			app.WithUniverseClient(fakeUniverse{}),
			app.WithConfigClient(fakeConfig{}),
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
