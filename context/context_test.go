package context_test

import (
	"context"
	"testing"
	"time"

	tfcontext "github.com/marcuspeh/trading-framework/context"
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

func (fakeConfig) Get(string) (string, error)                  { return "", nil }
func (fakeConfig) Watch(string, any, any) (job.Watcher, error) { return nil, nil }

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
		Clock:        clk,
		Portfolio:    fakePortfolio{},
		Market:       mkt,
		Universe:     uni,
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		IsTradingDay: func(d time.Time) bool { return d.Weekday() != time.Saturday && d.Weekday() != time.Sunday },
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

func TestContext_BarsUsesClockDate(t *testing.T) {
	today := time.Date(2024, 1, 2, 14, 30, 0, 0, time.UTC)
	mkt := &fakeMarket{}
	c, err := newCtx(t, mkt, &fakeUniverse{}, today)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Bars("AAPL", 5)
	want := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
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
