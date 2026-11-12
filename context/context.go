// Package context provides the framework Context implementation. It is
// the only surface strategy code interacts with. All dependencies
// (Clock, Portfolio, MarketClient, UniverseClient, Config, Logger,
// TradingDayFunc) are injected; there is no mode branching.
package context

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/universe"
)

// Portfolio is the local Portfolio used by Context. Concrete types live
// in the portfolio/ package.
type Portfolio = portfolio.Portfolio

// MarketClient is the framework's market-data dependency. The clients/
// package supplies the real adapter.
type MarketClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error)
}

// TradingDayFunc is the signature of an exchange-calendar predicate.
type TradingDayFunc = portfolio.TradingDayFunc

// Config is the strategy-facing configuration interface.
type Config = job.Config

// Logger is the structured logger exposed to strategy code.
type Logger = job.Logger

type ctx struct {
	clock      clock.Clock
	portfolio  Portfolio
	market     MarketClient
	universe   universe.Client
	config     Config
	logger     Logger
	tradingDay TradingDayFunc
	mode       job.Mode

	barCacheMu sync.Mutex
	barCache   map[string]barCacheEntry
}

// barCacheEntry holds the bars fetched for one symbol, along with the
// date they were fetched for.
//
// The date is recorded explicitly because the newest bar is dated by
// its trading day, which is often earlier than the day the fetch was
// made - a fetch made on a Monday returns Friday's bar - so it cannot
// stand in for "these bars were fetched for this day".
type barCacheEntry struct {
	asOf time.Time
	bars []order.Bar
}

// New constructs a Context. All dependencies are required except
// IsTradingDay (defaults to NYSEWeekdays) and Mode (defaults to
// ModeBacktest).
func New(opts Options) (job.Context, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	return &ctx{
		clock:      opts.Clock,
		portfolio:  opts.Portfolio,
		market:     opts.Market,
		universe:   opts.Universe,
		config:     opts.Config,
		logger:     opts.Logger,
		tradingDay: opts.GetIsTradingDay(),
		mode:       opts.GetMode(),
		barCache:   map[string]barCacheEntry{},
	}, nil
}

func (c *ctx) Today() time.Time {
	return c.clock.Today()
}

func (c *ctx) Now() time.Time {
	return c.clock.Now()
}

func (c *ctx) Mode() job.Mode {
	return c.mode
}

func (c *ctx) Bars(symbol string, lookback int) ([]order.Bar, error) {
	if lookback <= 0 {
		return nil, errInvalid("lookback must be > 0")
	}
	if symbol == "" {
		return nil, errInvalid("symbol is required")
	}

	asOf := c.clock.Today()

	c.barCacheMu.Lock()
	cached, ok := c.barCache[symbol]
	c.barCacheMu.Unlock()

	// A cached entry is reusable when it was fetched for the same day
	// and holds at least as many bars as were asked for. Anything else
	// - a different day, or a shorter history than requested - needs a
	// fresh fetch.
	if ok && sameDate(cached.asOf, asOf) && len(cached.bars) >= lookback {
		return cloneBars(cached.bars[len(cached.bars)-lookback:]), nil
	}

	bars, err := c.market.Bars(context.Background(), symbol, lookback, asOf)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(bars, func(i, j int) bool {
		return bars[i].Timestamp.Before(bars[j].Timestamp)
	})

	c.barCacheMu.Lock()
	c.barCache[symbol] = barCacheEntry{asOf: asOf, bars: bars}
	c.barCacheMu.Unlock()

	if len(bars) > lookback {
		bars = bars[len(bars)-lookback:]
	}
	return cloneBars(bars), nil
}

func (c *ctx) Constituents(etf string) ([]string, error) {
	if etf == "" {
		return nil, errInvalid("etf is required")
	}
	syms, err := c.universe.Constituents(context.Background(), etf, c.clock.Today())
	if err != nil {
		return nil, err
	}
	out := make([]string, len(syms))
	copy(out, syms)
	return out, nil
}

func (c *ctx) IsTradingDay(date time.Time) bool {
	return c.tradingDay(date)
}

func (c *ctx) Position(symbol string) order.Position {
	return c.portfolio.Position(symbol)
}

func (c *ctx) Positions() []order.Position {
	ps := c.portfolio.Positions()
	out := make([]order.Position, len(ps))
	copy(out, ps)
	return out
}

func (c *ctx) Cash() float64 {
	return c.portfolio.Cash()
}

func (c *ctx) PortfolioValue() float64 {
	return c.portfolio.Value()
}

func (c *ctx) Config() Config {
	return c.config
}

func (c *ctx) Logger() Logger {
	return c.logger
}

func (c *ctx) reset() {
	c.barCacheMu.Lock()
	defer c.barCacheMu.Unlock()
	c.barCache = map[string]barCacheEntry{}
}

// Reset clears the per-Context bar cache. The engine calls this when the
// date rolls forward so the next Bars() call refetches.
func Reset(c job.Context) {
	if rc, ok := c.(interface{ reset() }); ok {
		rc.reset()
	}
}
