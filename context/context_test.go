package context_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	tfcontext "github.com/marcuspeh/trading-framework/context"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

type fakeMarket struct {
	asOf time.Time
	bars []order.Bar
}

func (f *fakeMarket) Bars(_ context.Context, _ string, _ int, asOf time.Time) ([]order.Bar, error) {
	f.asOf = asOf
	return f.bars, nil
}

type fakeUniverse struct {
	asOf time.Time
}

func (f *fakeUniverse) Constituents(_ context.Context, etf string, asOf time.Time) ([]string, error) {
	f.asOf = asOf
	return []string{"AAPL", "MSFT"}, nil
}

type fakePortfolio struct{}

func (fakePortfolio) Cash() float64                  { return 1000 }
func (fakePortfolio) Position(string) order.Position { return order.Position{} }
func (fakePortfolio) Positions() []order.Position    { return nil }
func (fakePortfolio) Value() float64                 { return 1000 }

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error) { return "", nil }

func newCtx(t *testing.T, mkt *fakeMarket, uni *fakeUniverse, today time.Time) (interface {
	Today() time.Time
	Now() time.Time
	Bars(string, int) ([]order.Bar, error)
	Constituents(string) ([]string, error)
	IsTradingDay(time.Time) bool
}, error) {
	t.Helper()
	clk := &backtestClock{t: today}
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:     published.New(),
		Clock:        clk,
		Portfolio:    fakePortfolio{},
		Market:       mkt,
		Universe:     uni,
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		IsTradingDay: published.TradingDay,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

type backtestClock struct{ t time.Time }

func (b *backtestClock) Now() time.Time { return b.t }
func (b *backtestClock) Today() time.Time {
	return time.Date(b.t.Year(), b.t.Month(), b.t.Day(), 0, 0, 0, 0, b.t.Location())
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

func TestContext_TodayAndNowFromClock(t *testing.T) {
	today := time.Date(2024, 1, 2, 14, 30, 0, 0, time.UTC)
	c, err := newCtx(t, &fakeMarket{}, &fakeUniverse{}, today)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Today(); !got.Equal(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Today()=%v", got)
	}
	if got := c.Now(); !got.Equal(today) {
		t.Fatalf("Now()=%v", got)
	}
}

// Bars resolve their date through the session anchor, not the clock's
// date. A job that fires before the close reads the previous session,
// because the current one has not happened yet.
func TestContext_BarsUsesTheSessionAnchor(t *testing.T) {
	// 14:30 ET on 2024-01-02 - before the close, so the anchor is the
	// previous session. 2024-01-01 was New Year's Day, so the previous
	// session is Friday 2023-12-29.
	now := time.Date(2024, 1, 2, 14, 30, 0, 0, clock.NYSE)
	mkt := &fakeMarket{}
	c, err := newCtx(t, mkt, &fakeUniverse{}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Bars("AAPL", 5)
	want := time.Date(2023, 12, 29, 0, 0, 0, 0, clock.NYSE)
	if !mkt.asOf.Equal(want) {
		t.Fatalf("Bars asOf=%v want %v", mkt.asOf, want)
	}
}

// At or after the close, the anchor is the current session.
func TestContext_BarsAfterTheCloseUsesTheCurrentSession(t *testing.T) {
	now := time.Date(2024, 1, 2, 16, 0, 0, 0, clock.NYSE)
	mkt := &fakeMarket{}
	c, err := newCtx(t, mkt, &fakeUniverse{}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Bars("AAPL", 5)
	want := time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE)
	if !mkt.asOf.Equal(want) {
		t.Fatalf("Bars asOf=%v want %v", mkt.asOf, want)
	}
}

func TestContext_ConstituentsUsesClockDate(t *testing.T) {
	today := time.Date(2024, 1, 2, 14, 30, 0, 0, time.UTC)
	uni := &fakeUniverse{}
	c, err := newCtx(t, &fakeMarket{}, uni, today)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Constituents("SPY")
	want := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if !uni.asOf.Equal(want) {
		t.Fatalf("Constituents asOf=%v want %v", uni.asOf, want)
	}
}

func TestContext_MissingDepsRejected(t *testing.T) {
	_, err := tfcontext.New(tfcontext.Options{})
	if err == nil {
		t.Fatal("expected error for missing deps")
	}
}

// countingMarket counts how many times the market client is called.
type countingMarket struct {
	calls int
	bars  []order.Bar
}

func (m *countingMarket) Bars(_ context.Context, _ string, _ int, _ time.Time) ([]order.Bar, error) {
	m.calls++
	return m.bars, nil
}

// The Context caches bars per symbol per day. Repeated calls on the same
// day must be served from the cache rather than re-fetching.
//
// The cache used to be keyed on the newest bar's date, which is the
// bar's trading day - usually earlier than the day the fetch was made -
// so it never matched and every call went to the market client.
func TestContext_BarsCachedWithinDay(t *testing.T) {
	today := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	mkt := &countingMarket{bars: []order.Bar{
		{Timestamp: time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC), Close: 100},
	}}
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:  published.New(),
		Clock:     &backtestClock{t: today},
		Portfolio: fakePortfolio{},
		Market:    mkt,
		Universe:  &fakeUniverse{},
		Config:    fakeConfig{},
		Logger:    nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if _, err := c.Bars("AAPL", 1); err != nil {
			t.Fatal(err)
		}
	}
	if mkt.calls != 1 {
		t.Fatalf("market client called %d times, want 1", mkt.calls)
	}
}

// A cache entry fetched for one day must not be reused for another.
func TestContext_BarsNotCachedAcrossDays(t *testing.T) {
	mkt := &countingMarket{bars: []order.Bar{
		{Timestamp: time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC), Close: 100},
	}}
	clk := &backtestClock{t: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)}
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:  published.New(),
		Clock:     clk,
		Portfolio: fakePortfolio{},
		Market:    mkt,
		Universe:  &fakeUniverse{},
		Config:    fakeConfig{},
		Logger:    nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Bars("AAPL", 1); err != nil {
		t.Fatal(err)
	}
	clk.t = time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	if _, err := c.Bars("AAPL", 1); err != nil {
		t.Fatal(err)
	}
	if mkt.calls != 2 {
		t.Fatalf("market client called %d times, want 2", mkt.calls)
	}
}

// TestContext_DefaultTradingDayFuncDoesNotPanic verifies that omitting
// IsTradingDay falls back to the weekday calendar instead of panicking
// on a nil function call.
func TestContext_DefaultTradingDayFuncDoesNotPanic(t *testing.T) {
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:  published.New(),
		Clock:     &backtestClock{t: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		Portfolio: fakePortfolio{},
		Market:    &fakeMarket{},
		Universe:  &fakeUniverse{},
		Config:    fakeConfig{},
		Logger:    nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2024-01-02 is a Tuesday, 2024-01-06 a Saturday.
	if !c.IsTradingDay(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Error("expected Tuesday to be a trading day")
	}
	if c.IsTradingDay(time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC)) {
		t.Error("expected Saturday to not be a trading day")
	}
}

// TestContext_ModeDefaultsToBacktest verifies the mode default.
func TestContext_ModeDefaultsToBacktest(t *testing.T) {
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:  published.New(),
		Clock:     &backtestClock{t: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		Portfolio: fakePortfolio{},
		Market:    &fakeMarket{},
		Universe:  &fakeUniverse{},
		Config:    fakeConfig{},
		Logger:    nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Mode(); got != job.ModeBacktest {
		t.Fatalf("Mode()=%q want %q", got, job.ModeBacktest)
	}
}

// TestContext_ExplicitModeIsUsed verifies an explicit mode wins.
func TestContext_ExplicitModeIsUsed(t *testing.T) {
	c, err := tfcontext.New(tfcontext.Options{
		Calendar:  published.New(),
		Clock:     &backtestClock{t: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		Portfolio: fakePortfolio{},
		Market:    &fakeMarket{},
		Universe:  &fakeUniverse{},
		Config:    fakeConfig{},
		Logger:    nopLogger{},
		Mode:      job.ModeLive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Mode(); got != job.ModeLive {
		t.Fatalf("Mode()=%q want %q", got, job.ModeLive)
	}
}
