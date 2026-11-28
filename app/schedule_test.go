package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// The reference strategy (trading_algo/algo01-corner2RSI) schedules four
// jobs: cancel stale orders before the open, liquidate removed
// constituents during the session, preload an hour before the close and
// run at the close. On an early-close day the last two move with the
// session.
//
// This test proves the framework can express that schedule, and that the
// close-relative slots follow the exchange's early closes.
func TestApp_ScheduleFollowsTheSession(t *testing.T) {
	// 2026-11-27 is the Friday after Thanksgiving: the exchange closes
	// at 13:00 ET instead of 16:00.
	earlyClose := time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE)
	// 2026-12-01 is an ordinary Tuesday.
	normalDay := time.Date(2026, 12, 1, 0, 0, 0, 0, calendar.NYSE)

	type slot struct {
		name string
		when time.Time
	}

	run := func(day time.Time) []slot {
		var mu sync.Mutex
		var fired []slot

		mk := func(name string, s job.Slot) {
			a := app.NewApp(
				app.WithCalendarProvider(published.New()),
				app.WithBacktest(app.BacktestConfig{
					Start:        day,
					End:          day.AddDate(0, 0, 1),
					StartingCash: 10_000,
					BarSource:    fakeBars{},
				}),
				app.WithMarketClient(fakeMarket{}),
				app.WithUniverseClient(fakeUniverse{}),
				app.WithConfigClient(fakeConfig{}),
				app.WithTearsheetDir(t.TempDir()),
			)
			if err := a.RegisterJob(job.Job{
				Name: name, Time: s,
				Handler: func(_ job.Context, at time.Time) ([]order.Order, error) {
					mu.Lock()
					fired = append(fired, slot{name: name, when: at})
					mu.Unlock()
					return nil, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			if err := a.Run(); err != nil {
				t.Fatal(err)
			}
		}

		mk("cancel-stale", job.Time(7, 30))
		mk("liquidate", job.Time(10, 30))
		mk("preload", job.BeforeClose(time.Hour))
		mk("run", job.AtClose())
		return fired
	}

	find := func(slots []slot, name string) time.Time {
		for _, s := range slots {
			if s.name == name {
				return s.when
			}
		}
		return time.Time{}
	}

	// An ordinary day: the close-relative jobs follow the 16:00 close.
	normal := run(normalDay)
	if got := find(normal, "run"); got.Hour() != 16 || got.Minute() != 0 {
		t.Errorf("run at %v want 16:00", got)
	}
	if got := find(normal, "preload"); got.Hour() != 15 || got.Minute() != 0 {
		t.Errorf("preload at %v want 15:00", got)
	}
	// The fixed slots do not move.
	if got := find(normal, "cancel-stale"); got.Hour() != 7 || got.Minute() != 30 {
		t.Errorf("cancel-stale at %v want 07:30", got)
	}
	if got := find(normal, "liquidate"); got.Hour() != 10 || got.Minute() != 30 {
		t.Errorf("liquidate at %v want 10:30", got)
	}

	// An early-close day: the close-relative jobs move to 13:00 and
	// 12:00, while the fixed slots stay put.
	early := run(earlyClose)
	if got := find(early, "run"); got.Hour() != 13 || got.Minute() != 0 {
		t.Errorf("run at %v want 13:00 on an early-close day", got)
	}
	if got := find(early, "preload"); got.Hour() != 12 || got.Minute() != 0 {
		t.Errorf("preload at %v want 12:00 on an early-close day", got)
	}
	if got := find(early, "cancel-stale"); got.Hour() != 7 || got.Minute() != 30 {
		t.Errorf("cancel-stale at %v want 07:30", got)
	}
	if got := find(early, "liquidate"); got.Hour() != 10 || got.Minute() != 30 {
		t.Errorf("liquidate at %v want 10:30", got)
	}
}

// A job must not fire on a day the exchange is closed.
func TestApp_JobsDoNotFireOnHolidays(t *testing.T) {
	// 2026-12-25 is Christmas Day.
	holiday := time.Date(2026, 12, 25, 0, 0, 0, 0, calendar.NYSE)

	var fired int
	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        holiday,
			End:          holiday.AddDate(0, 0, 1),
			StartingCash: 10_000,
			BarSource:    fakeBars{},
		}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "run", Time: job.AtClose(),
		Handler: func(job.Context, time.Time) ([]order.Order, error) {
			fired++
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if fired != 0 {
		t.Fatalf("job fired %d times on a market holiday", fired)
	}
}

var _ = context.Background
