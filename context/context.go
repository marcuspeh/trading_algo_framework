// Package context provides the framework Context implementation. It is
// the only surface strategy code interacts with. All dependencies
// (Clock, Portfolio, MarketClient, UniverseClient, Config, Logger,
// TradingDayFunc) are injected; there is no mode branching.
package context

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/universe"
)

// Portfolio is the local Portfolio used by Context. Concrete types live
// in the portfolio/ package.
type Portfolio = portfolio.Portfolio

// MarketClient is the framework's market-data dependency. The clients/
// package supplies the real adapter.
type MarketClient = market.Client

// TradingDayFunc is the signature of an exchange-calendar predicate.
type TradingDayFunc = portfolio.TradingDayFunc

// Config is the strategy-facing configuration interface.
type Config = job.Config

// Logger is the structured logger exposed to strategy code.
type Logger = job.Logger

type ctx struct {
	clock      clock.Clock
	session    *clock.Session
	portfolio  Portfolio
	market     MarketClient
	universe   universe.Client
	config     Config
	logger     Logger
	tradingDay TradingDayFunc
	mode       job.Mode
	broker     job.OrderBroker

	barCacheMu sync.Mutex
	barCache   map[string]map[string]barCacheEntry
}

// barCacheEntry holds the bars fetched for one symbol and one session.
//
// The session is recorded explicitly because the newest bar is dated by
// its trading day, which is often earlier than the day the fetch was
// made - a fetch made on a Monday returns Friday's bar - so it cannot
// stand in for "these bars were fetched for this session".
type barCacheEntry struct {
	asOf time.Time
	bars []order.Bar
}

// barWorkers is how many symbols' bars are fetched at once.
//
// The market-data-service takes seconds to answer a request and a
// universe runs to hundreds of names, so fetching them one at a time
// makes a backtest take days.
const barWorkers = 24

// New constructs a Context. All dependencies are required except
// IsTradingDay (defaults to the exchange calendar's trading days) and
// Mode (defaults to
// ModeBacktest).
func New(opts Options) (job.Context, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	return &ctx{
		clock:      opts.Clock,
		session:    opts.GetSession(),
		portfolio:  opts.Portfolio,
		market:     opts.Market,
		universe:   opts.Universe,
		config:     opts.Config,
		logger:     opts.Logger,
		tradingDay: opts.GetIsTradingDay(),
		mode:       opts.GetMode(),
		broker:     opts.Broker,
		barCache:   map[string]map[string]barCacheEntry{},
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

// Bars returns the most recent bars for each of the symbols, ending on
// the session the clock is anchored to.
//
// The anchor is the most recent session whose close has passed, so a
// job that fires before the close reads the previous session and a job
// that fires at or after the close reads the current one. A strategy
// can never see a bar that had not been printed yet.
//
// The symbols are fetched concurrently, a few at a time. A strategy
// that reads the history of a whole universe would otherwise pay for
// it one name at a time, and the market-data-service takes seconds to
// answer a request.
//
// A symbol with less history than was asked for - or with none at all
// - is left out of the map, so an indicator is never silently computed
// over fewer bars than asked for. The error is for a request the
// framework could not make at all: an empty list of symbols, a
// lookback that is not positive, or a blank symbol.
func (c *ctx) Bars(symbols []string, lookback int) (map[string][]order.Bar, error) {
	if len(symbols) == 0 {
		return nil, errInvalid("symbols are required")
	}
	if lookback <= 0 {
		return nil, errInvalid("lookback must be > 0")
	}
	for _, symbol := range symbols {
		if symbol == "" {
			return nil, errInvalid("symbol is required")
		}
	}

	asOf := c.session.Anchor(c.clock.Now())

	out := make(map[string][]order.Bar, len(symbols))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, barWorkers)

	// A symbol asked for twice is fetched once.
	seen := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		if seen[symbol] {
			continue
		}
		seen[symbol] = true

		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			bars, err := c.barsFor(symbol, lookback, asOf)
			if err != nil {
				// The symbol is left out of the map.
				return
			}
			mu.Lock()
			out[symbol] = bars
			mu.Unlock()
		}(symbol)
	}
	wg.Wait()

	return out, nil
}

// barsFor returns the most recent bars for one symbol.
//
// A symbol with a shorter history than was asked for returns
// job.ErrInsufficientBars rather than a short history, so an indicator
// is never silently computed over fewer bars than asked for.
func (c *ctx) barsFor(symbol string, lookback int, asOf time.Time) ([]order.Bar, error) {
	c.barCacheMu.Lock()
	cached, ok := c.barCache[symbol][sessionKey(asOf)]
	c.barCacheMu.Unlock()

	// A cached entry is reusable when it was fetched for the same
	// session and holds at least as many bars as were asked for.
	// Anything else - a different session, or a shorter history than
	// requested - needs a fresh fetch.
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
	if c.barCache[symbol] == nil {
		c.barCache[symbol] = map[string]barCacheEntry{}
	}
	c.barCache[symbol][sessionKey(asOf)] = barCacheEntry{asOf: asOf, bars: bars}
	// The anchor only moves forward, so a session older than this one
	// will never be read again. Dropping it keeps the cache the size of
	// the symbols in play rather than the size of the backtest.
	for key := range c.barCache[symbol] {
		if key < sessionKey(asOf) {
			delete(c.barCache[symbol], key)
		}
	}
	c.barCacheMu.Unlock()

	if len(bars) < lookback {
		return nil, fmt.Errorf("%w: %s has %d bars, %d requested",
			job.ErrInsufficientBars, symbol, len(bars), lookback)
	}
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

// OpenOrders returns the orders resting at the broker.
func (c *ctx) OpenOrders() ([]order.Order, error) {
	if c.broker == nil {
		return nil, errMissing("broker")
	}
	return c.broker.OpenOrders(context.Background())
}

// CancelOrder cancels a resting order by its id.
func (c *ctx) CancelOrder(orderID string) error {
	if c.broker == nil {
		return errMissing("broker")
	}
	return c.broker.Cancel(context.Background(), orderID)
}

// UpdateOrder changes the limit price of a resting order.
func (c *ctx) UpdateOrder(orderID string, limit float64) error {
	if c.broker == nil {
		return errMissing("broker")
	}
	return c.broker.Update(context.Background(), orderID, limit)
}

// The bar cache is keyed by the session the bars were fetched for, so
// it invalidates itself when the session rolls forward. There is no
// reset to call: a job that reads the same session twice is served
// from the cache, and a job that reads a new session is not.
