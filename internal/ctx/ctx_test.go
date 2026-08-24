package ctx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	ctximpl "github.com/marcuspeh/trading-framework/internal/ctx"
)

// fakeMarket is a minimal MarketDataClient implementation.
type fakeMarket struct {
	bars map[string][]tf.Bar
}

func (f *fakeMarket) Bars(_ context.Context, symbol string, lookback int, _ time.Time) ([]tf.Bar, error) {
	list, ok := f.bars[symbol]
	if !ok {
		return nil, tf.ErrNotFound
	}
	if lookback > len(list) {
		lookback = len(list)
	}
	out := make([]tf.Bar, lookback)
	copy(out, list[len(list)-lookback:])
	return out, nil
}

type fakeConstituents struct {
	symbols map[string][]string
}

func (f *fakeConstituents) Constituents(_ context.Context, etf string, _ time.Time) ([]string, error) {
	s, ok := f.symbols[etf]
	if !ok {
		return nil, tf.ErrNotFound
	}
	out := make([]string, len(s))
	copy(out, s)
	return out, nil
}

type fakePortfolio struct {
	cash float64
	pos  map[string]tf.Position
}

func (f *fakePortfolio) Cash() float64 { return f.cash }

func (f *fakePortfolio) Position(s string) tf.Position {
	p, ok := f.pos[s]
	if !ok {
		return tf.Position{Symbol: s}
	}
	return p
}

func (f *fakePortfolio) Positions() []tf.Position {
	out := make([]tf.Position, 0, len(f.pos))
	for _, p := range f.pos {
		out = append(out, p)
	}
	return out
}

func (f *fakePortfolio) Value() float64 {
	t := f.cash
	for _, p := range f.pos {
		t += p.Quantity * p.AvgPrice
	}
	return t
}

type fakeClock struct {
	now  time.Time
	date time.Time
}

func (f *fakeClock) Now() time.Time  { return f.now }
func (f *fakeClock) Date() time.Time { return f.date }

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error) { return "", nil }
func (fakeConfig) Watch(string, any, any) (tf.Watcher, error) {
	return nil, errors.New("not implemented")
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

func newCtx(t *testing.T, market *fakeMarket, constituents *fakeConstituents, port *fakePortfolio, clk *fakeClock) tf.Context {
	t.Helper()
	c, err := ctximpl.New(ctximpl.Options{
		Mode:         tf.Backtest,
		Clock:        clk,
		Portfolio:    port,
		Market:       market,
		Constituents: constituents,
		Config:       fakeConfig{},
		Logger:       nopLogger{},
	})
	if err != nil {
		t.Fatalf("ctx.New: %v", err)
	}
	return c
}

func TestContext_RejectsMissingDeps(t *testing.T) {
	_, err := ctximpl.New(ctximpl.Options{
		Mode:   tf.Backtest,
		Logger: nopLogger{},
	})
	if err == nil {
		t.Fatal("expected missing-dep error")
	}
}

func TestContext_BarsUsesAsOfAndCaches(t *testing.T) {
	day := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	market := &fakeMarket{bars: map[string][]tf.Bar{
		"AAPL": {
			{Time: day.AddDate(0, 0, -2), Close: 100},
			{Time: day.AddDate(0, 0, -1), Close: 101},
			{Time: day, Close: 102},
		},
	}}
	constituents := &fakeConstituents{symbols: map[string][]string{}}
	port := &fakePortfolio{cash: 1000, pos: map[string]tf.Position{}}
	clk := &fakeClock{now: day, date: day}

	c := newCtx(t, market, constituents, port, clk)
	got, err := c.Bars("AAPL", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[len(got)-1].Close != 102 {
		t.Fatalf("unexpected bars: %+v", got)
	}

	// Second call should hit the cache.
	got2, err := c.Bars("AAPL", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 2 || got2[len(got2)-1].Close != 102 {
		t.Fatalf("cached bars mismatch: %+v", got2)
	}

	// Reset cache + change date so the next call refetches.
	clk.date = day.AddDate(0, 0, 1)
	clk.now = day.AddDate(0, 0, 1)
	ctximpl.ResetBarsCache(c)
	if _, err := c.Bars("AAPL", 1); err != nil {
		t.Fatal(err)
	}
}

func TestContext_BarsValidates(t *testing.T) {
	c := newCtx(t, &fakeMarket{}, &fakeConstituents{}, &fakePortfolio{cash: 0, pos: map[string]tf.Position{}}, &fakeClock{})
	if _, err := c.Bars("AAPL", 0); err == nil {
		t.Fatal("expected lookback error")
	}
	if _, err := c.Bars("", 1); err == nil {
		t.Fatal("expected symbol error")
	}
}

func TestContext_ConstituentsValidates(t *testing.T) {
	c := newCtx(t, &fakeMarket{}, &fakeConstituents{symbols: map[string][]string{"SPY": {"AAPL"}}}, &fakePortfolio{cash: 0, pos: map[string]tf.Position{}}, &fakeClock{date: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)})
	if _, err := c.Constituents(""); err == nil {
		t.Fatal("expected etf error")
	}
	if _, err := c.Constituents("QQQ"); err == nil {
		t.Fatal("expected not-found error")
	}
	syms, err := c.Constituents("SPY")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0] != "AAPL" {
		t.Fatalf("unexpected: %v", syms)
	}
}

func TestContext_IsTradingDayOverride(t *testing.T) {
	never := func(time.Time) bool { return false }
	c, err := ctximpl.New(ctximpl.Options{
		Mode:         tf.Backtest,
		Clock:        &fakeClock{date: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)},
		Portfolio:    &fakePortfolio{cash: 0, pos: map[string]tf.Position{}},
		Market:       &fakeMarket{},
		Constituents: &fakeConstituents{},
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		IsTradingDay: never,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.IsTradingDay(time.Now()) {
		t.Fatal("override should make all dates non-trading days")
	}
}

func TestContext_PortfolioAndModeAndLogger(t *testing.T) {
	port := &fakePortfolio{cash: 250, pos: map[string]tf.Position{
		"AAPL": {Symbol: "AAPL", Quantity: 5, AvgPrice: 10},
	}}
	c := newCtx(t, &fakeMarket{}, &fakeConstituents{}, port, &fakeClock{date: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)})
	if c.Cash() != 250 {
		t.Fatalf("cash: %f", c.Cash())
	}
	if c.PortfolioValue() != 300 {
		t.Fatalf("value: %f", c.PortfolioValue())
	}
	if c.Position("AAPL").Quantity != 5 {
		t.Fatal("position mismatch")
	}
	if c.Mode() != tf.Backtest {
		t.Fatal("mode mismatch")
	}
	if c.Logger() == nil {
		t.Fatal("logger nil")
	}
	if c.Config() == nil {
		t.Fatal("config nil")
	}
}

func TestContext_DateAndNow(t *testing.T) {
	day := time.Date(2024, 1, 5, 12, 30, 0, 0, time.UTC)
	c := newCtx(t, &fakeMarket{}, &fakeConstituents{}, &fakePortfolio{cash: 0, pos: map[string]tf.Position{}}, &fakeClock{date: day, now: day})
	if !c.Date().Equal(day) {
		t.Fatalf("date mismatch: %v", c.Date())
	}
	if !c.Now().Equal(day) {
		t.Fatalf("now mismatch: %v", c.Now())
	}
}
