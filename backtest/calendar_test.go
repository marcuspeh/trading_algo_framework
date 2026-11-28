package backtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
)

// emptyMarket is a market client with no bars.
type emptyMarket struct{}

// Bars returns no bars.
func (emptyMarket) Bars(context.Context, string, int, time.Time) ([]order.Bar, error) {
	return nil, nil
}

// BarsBatch returns no bars.
func (emptyMarket) BarsBatch(_ context.Context, symbols []string, _ int, _ time.Time) (map[string][]order.Bar, error) {
	out := make(map[string][]order.Bar, len(symbols))
	for _, symbol := range symbols {
		out[symbol] = nil
	}
	return out, nil
}

// nopLogger discards every log line.
type nopLogger struct{}

// Info discards the line.
func (nopLogger) Info(string, ...any) {}

// Error discards the line.
func (nopLogger) Error(string, ...any) {}

// Warn discards the line.
func (nopLogger) Warn(string, ...any) {}

// Debug discards the line.
func (nopLogger) Debug(string, ...any) {}

// newCalendarOnlyEngine builds an engine with only a Calendar, and the
// minimum else the engine needs.
func newCalendarOnlyEngine(t *testing.T, start, end time.Time) *backtest.Engine {
	t.Helper()
	e, err := backtest.New(backtest.Config{
		Start:     start,
		End:       end,
		Calendar:  published.New(),
		Market:    emptyMarket{},
		Portfolio: portfolio.NewBacktest(10_000),
		Executor:  execution.NewBroker(nil),
		Risk:      risk.NewBasic(),
		Logger:    nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// An engine built with only a Calendar knows the exchange's trading
// days, so a job does not fire on a weekend or a holiday.
//
// The range runs from New Year's Day to the Saturday after it, so the
// only trading days in it are the 2nd, 3rd, 4th and 5th.
func TestEngine_CalendarOnlyDoesNotFireOnNonTradingDays(t *testing.T) {
	var fired []string

	e := newCalendarOnlyEngine(t,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
	)
	if err := e.RegisterJob(job.Job{
		Name: "daily",
		Time: job.Time(10, 0),
		Handler: func(_ job.Context, d time.Time) ([]order.Order, error) {
			fired = append(fired, d.Format("2006-01-02"))
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	want := []string{"2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05"}
	if len(fired) != len(want) {
		t.Fatalf("fired on %v want %v", fired, want)
	}
	for i, d := range want {
		if fired[i] != d {
			t.Errorf("fired[%d] = %s want %s", i, fired[i], d)
		}
	}
}

// A close-relative slot resolves against the session's boundaries, so
// it fires at the close of each trading day and nowhere else.
func TestEngine_CalendarOnlyResolvesCloseRelativeSlots(t *testing.T) {
	var fired []time.Time

	e := newCalendarOnlyEngine(t,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
	)
	if err := e.RegisterJob(job.Job{
		Name: "at-close",
		Time: job.AtClose(),
		Handler: func(_ job.Context, d time.Time) ([]order.Order, error) {
			fired = append(fired, d)
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	// Four trading days, each at its close.
	if len(fired) != 4 {
		t.Fatalf("fired %d times, want 4: %v", len(fired), fired)
	}
	for _, at := range fired {
		if at.Hour() != 16 || at.Minute() != 0 {
			t.Errorf("fired at %v want 16:00", at)
		}
	}
}

// An early-close day closes at 13:00, so a close-relative slot fires
// then rather than at 16:00.
func TestEngine_CalendarOnlyResolvesAnEarlyClose(t *testing.T) {
	var fired []time.Time

	// 2024-11-29 is the Friday after Thanksgiving.
	day := time.Date(2024, 11, 29, 0, 0, 0, 0, time.UTC)
	e := newCalendarOnlyEngine(t, day, day.AddDate(0, 0, 1))
	if err := e.RegisterJob(job.Job{
		Name: "at-close",
		Time: job.AtClose(),
		Handler: func(_ job.Context, d time.Time) ([]order.Order, error) {
			fired = append(fired, d)
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if len(fired) != 1 {
		t.Fatalf("fired %d times, want 1: %v", len(fired), fired)
	}
	if fired[0].Hour() != 13 || fired[0].Minute() != 0 {
		t.Errorf("fired at %v want 13:00 on an early-close day", fired[0])
	}
}
