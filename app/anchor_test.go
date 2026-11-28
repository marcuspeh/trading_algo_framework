package app_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// anchorBars returns a bar for each session in the range, with the
// session's date in the close so a test can tell which one was read.
func anchorBars() map[string][]order.Bar {
	out := map[string][]order.Bar{}
	day := time.Date(2024, 1, 2, 0, 0, 0, 0, calendar.NYSE)
	for i := 0; i < 10; i++ {
		if !published.TradingDay(day) {
			day = day.AddDate(0, 0, 1)
			continue
		}
		out["AAPL"] = append(out["AAPL"], order.Bar{
			Symbol:    "AAPL",
			Timestamp: day,
			Open:      100, High: 110, Low: 90,
			Close:  float64(100 + i),
			Volume: 1_000_000,
		})
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// A job that fires before the close reads the previous session, and a
// job that fires at or after the close reads the current one.
//
// This is the rule the reference strategy relies on: it preloads
// yesterday's history before the open and merges today's bar at the
// close.
func TestApp_JobSeesTheSessionItShould(t *testing.T) {
	bars := anchorBars()

	// The close of each bar identifies the session it belongs to.
	sessionOf := func(bars []order.Bar) string {
		return bars[len(bars)-1].Timestamp.Format("2006-01-02")
	}

	seen := map[string]string{}
	record := func(name string) job.Handler {
		return func(ctx job.Context, date time.Time) ([]order.Order, error) {
			if _, ok := seen[name]; ok {
				// Only the first day is of interest here.
				return nil, nil
			}
			bars, err := ctx.Bars([]string{"AAPL"}, 1)
			if err != nil {
				return nil, err
			}
			seen[name] = sessionOf(bars["AAPL"])
			return nil, nil
		}
	}

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 3, 0, 0, 0, 0, calendar.NYSE),
			End:          time.Date(2024, 1, 5, 0, 0, 0, 0, calendar.NYSE),
			StartingCash: 10_000,
		}),
		app.WithMarketClient(clients.NewStaticBars(bars)),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "preopen", Time: job.Time(7, 30), Handler: record("preopen"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterJob(job.Job{
		Name: "atclose", Time: job.AtClose(), Handler: record("atclose"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	// On 2024-01-03 the pre-open job reads the previous session
	// (2024-01-02) and the close job reads the current one
	// (2024-01-03).
	if seen["preopen"] != "2024-01-02" {
		t.Errorf("the pre-open job read %s want 2024-01-02", seen["preopen"])
	}
	if seen["atclose"] != "2024-01-03" {
		t.Errorf("the close job read %s want 2024-01-03", seen["atclose"])
	}
}
