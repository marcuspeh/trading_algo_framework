// Package ctx provides the concrete framework Context implementation.
// Strategy code only sees the trading.Context interface (root package);
// all market data, constituents, broker, and config dependencies live
// behind this internal package.
package ctx

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/go-tools/util"
	trading "github.com/marcuspeh/trading-framework"
)

type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]trading.Bar, error)
}

type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

type Portfolio interface {
	Cash() float64
	Position(symbol string) trading.Position
	Positions() []trading.Position
	Value() float64
}

type Clock interface {
	Now() time.Time
	Date() time.Time
}

type ctx struct {
	mode         trading.Mode
	clock        Clock
	portfolio    Portfolio
	market       MarketDataClient
	constituents ConstituentsClient
	config       trading.Config
	logger       trading.Logger
	tradingDay   func(date time.Time) bool

	barCacheMu sync.Mutex
	barCache   map[string][]trading.Bar
}

type Options struct {
	Mode         trading.Mode
	Clock        Clock
	Portfolio    Portfolio
	Market       MarketDataClient
	Constituents ConstituentsClient
	Config       trading.Config
	Logger       trading.Logger

	// IsTradingDay is optional. When nil, the context uses the default
	// weekday-only heuristic.
	IsTradingDay func(date time.Time) bool
}

func New(opts Options) (trading.Context, error) {
	if opts.Clock == nil {
		return nil, errMissing("clock")
	}
	if opts.Portfolio == nil {
		return nil, errMissing("portfolio")
	}
	if opts.Market == nil {
		return nil, errMissing("market")
	}
	if opts.Constituents == nil {
		return nil, errMissing("constituents")
	}
	if opts.Config == nil {
		return nil, errMissing("config")
	}
	if opts.Logger == nil {
		return nil, errMissing("logger")
	}

	isTradingDay := opts.IsTradingDay
	if isTradingDay == nil {
		isTradingDay = defaultIsTradingDay
	}

	return &ctx{
		mode:         opts.Mode,
		clock:        opts.Clock,
		portfolio:    opts.Portfolio,
		market:       opts.Market,
		constituents: opts.Constituents,
		config:       opts.Config,
		logger:       opts.Logger,
		tradingDay:   isTradingDay,
		barCache:     map[string][]trading.Bar{},
	}, nil
}

func (c *ctx) Date() time.Time { return c.clock.Date() }
func (c *ctx) Now() time.Time  { return c.clock.Now() }

func (c *ctx) Bars(symbol string, lookback int) ([]trading.Bar, error) {
	if lookback <= 0 {
		return nil, errInvalid("lookback must be > 0")
	}
	if symbol == "" {
		return nil, errInvalid("symbol is required")
	}

	asOf := c.clock.Date()

	c.barCacheMu.Lock()
	cached, ok := c.barCache[symbol]
	c.barCacheMu.Unlock()

	if ok && len(cached) >= lookback {
		// The cache is invalidated by the engine when the date rolls
		// forward, so a cache hit only counts when the cache slice's
		// last bar is on the same date as the current asOf.
		last := cached[len(cached)-1].Time
		if sameDate(last, asOf) {
			return cloneBars(cached[len(cached)-lookback:]), nil
		}
	}

	bars, err := c.market.Bars(context.Background(), symbol, lookback, asOf)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(bars, func(i, j int) bool {
		return bars[i].Time.Before(bars[j].Time)
	})

	c.barCacheMu.Lock()
	c.barCache[symbol] = bars
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
	syms, err := c.constituents.Constituents(context.Background(), etf, c.clock.Date())
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

func (c *ctx) Position(symbol string) trading.Position {
	return c.portfolio.Position(symbol)
}

func (c *ctx) Positions() []trading.Position {
	ps := c.portfolio.Positions()
	out := make([]trading.Position, len(ps))
	copy(out, ps)
	return out
}

func (c *ctx) Cash() float64           { return c.portfolio.Cash() }
func (c *ctx) PortfolioValue() float64 { return c.portfolio.Value() }
func (c *ctx) Mode() trading.Mode      { return c.mode }
func (c *ctx) Config() trading.Config  { return c.config }
func (c *ctx) Logger() trading.Logger  { return c.logger }

// ResetBarsCache clears the per-Context bar cache. The engine calls this
// at the start of each backtest date so the next Bars() call refetches.
func ResetBarsCache(c trading.Context) {
	if rc, ok := c.(interface{ resetBars() }); ok {
		rc.resetBars()
	}
}

func (c *ctx) resetBars() {
	c.barCacheMu.Lock()
	defer c.barCacheMu.Unlock()
	c.barCache = map[string][]trading.Bar{}
}

func cloneBars(in []trading.Bar) []trading.Bar {
	return util.Copy(in)
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func defaultIsTradingDay(date time.Time) bool {
	d := date.Weekday()
	return d != time.Saturday && d != time.Sunday
}

type missingDepError struct{ name string }

func (e missingDepError) Error() string { return "context: missing dependency: " + e.name }

func errMissing(name string) error { return missingDepError{name: name} }

type invalidArgError struct{ msg string }

func (e invalidArgError) Error() string { return "context: " + e.msg }

func errInvalid(msg string) error { return invalidArgError{msg: msg} }
