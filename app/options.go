// Package app is the framework's top-level wiring layer. Strategy code
// constructs an App via NewApp + functional options, registers Jobs, and
// calls Run.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/broker/mock"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/logging"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/universe"
)

// MarketClient is the framework's market-data interface.
type MarketClient = market.Client

// RiskManager validates orders returned by Jobs before they reach the
// Executor. Strategies may swap it for custom logic.
type RiskManager = risk.Manager

// App is the top-level entry point used by strategy code.
type App struct {
	mu sync.Mutex

	clock     clock.Clock
	btClock   *clock.BacktestClock
	session   *clock.Session
	executor  execution.Executor
	portfolio portfolio.Portfolio
	risk      RiskManager
	market    MarketClient
	universe  universe.Client
	config    job.Config
	logger    job.Logger
	calendar  calendar.Provider

	// tradingDay is the effective trading-day predicate, derived from
	// the calendar in resolve(). tradingDayOverride and
	// marketCloseOverride are the strategy's replacements for the
	// calendar's answers.
	tradingDay          portfolio.TradingDayFunc
	tradingDayOverride  portfolio.TradingDayFunc
	marketCloseOverride func(time.Time) time.Time

	// Convenience configs. If set, Run() wires the matching stack.
	backtestCfg *BacktestConfig
	liveCfg     *LiveConfig

	// Engines, populated after resolve(). Exposed for inspection in tests.
	btEngine   *backtestEngine
	liveEngine *liveEngine

	// liveBroker is the broker used by the live stack.
	liveBroker execution.Client

	// btBroker is the simulated broker used by the backtest stack.
	btBroker *mock.Mock

	// fills remembers the orders the live executor places, so the
	// fills the broker reports later can be named.
	fills *fills.Registry

	jobs   []job.Job
	warmup []warmupEntry
}

type backtestEngine = backtest.Engine
type liveEngine = live.Engine

type warmupEntry struct {
	Symbol   string
	Lookback int
}

// Option configures the App.
type Option func(*App)

func WithRiskManager(r RiskManager) Option { return func(a *App) { a.risk = r } }

// WithMarketCloseFunc replaces the exchange calendar's session close.
//
// It takes the close of the session on a date and returns the time of
// it. The default is the calendar's own answer: 16:00 ET, or 13:00 ET
// on an early-close day.
//
// The escape hatch is for a house view of the schedule that differs
// from the published one - an exchange that closes at a different
// hour, or a data vendor whose early closes are not the exchange's.
// The function is called for every date the framework resolves a
// session for, so it should be cheap and must be pure.
//
// Passing nil restores the calendar's answer.
func WithMarketCloseFunc(fn func(date time.Time) time.Time) Option {
	return func(a *App) { a.marketCloseOverride = fn }
}

// WithCalendarProvider sets the exchange calendar the framework trades
// on.
//
// The provider answers whether the exchange traded on a date and when
// the session opened and closed. It is required: a provider backed by
// the market-data-service - calendar.NewService - keeps the framework
// on the exchange's published schedule.
//
// A provider that cannot answer is fatal: the framework raises a
// *calendar.ProviderError and Run returns it as an error. Trading on a
// schedule it could not confirm is worse than not trading.
//
// WithTradingDayFunc and WithMarketCloseFunc win over the provider, so
// a strategy that overrides one of them keeps its override.
func WithCalendarProvider(p calendar.Provider) Option {
	return func(a *App) { a.calendar = p }
}
func WithMarketClient(m MarketClient) Option      { return func(a *App) { a.market = m } }
func WithUniverseClient(u universe.Client) Option { return func(a *App) { a.universe = u } }
func WithConfigClient(c job.Config) Option        { return func(a *App) { a.config = c } }
func WithLogger(l job.Logger) Option              { return func(a *App) { a.logger = l } }

// WithTradingDayFunc replaces the exchange calendar's trading days.
//
// It takes a date and reports whether the strategy trades on it. The
// default is the calendar's own answer.
//
// The escape hatch is for a strategy that trades on a different
// exchange's days, or on a schedule of its own. On the days it names
// that the exchange was closed, the standard session - 09:30 to 16:00
// ET - stands in, so a close-relative slot still resolves.
//
// The function is called for every date the framework resolves a
// session for, so it should be cheap and must be pure.
//
// Passing nil restores the calendar's answer.
func WithTradingDayFunc(f portfolio.TradingDayFunc) Option {
	return func(a *App) { a.tradingDayOverride = f }
}

// WithStdoutLogger writes log lines to stdout.
func WithStdoutLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stdout) }
}

// WithStderrLogger writes log lines to stderr.
func WithStderrLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stderr) }
}

// WithLogFile creates a TXT-file logger.
func WithLogFile(path string) Option {
	return func(a *App) {
		l, err := logging.NewFileLogger(path)
		if err != nil {
			a.logger = logging.Nop()
			return
		}
		a.logger = l
	}
}

// BacktestConfig is a convenience: when set, the App wires up the
// backtest stack (BacktestClock, BacktestScheduler, BacktestExecutor,
// BacktestPortfolio).
type BacktestConfig struct {
	Start        time.Time
	End          time.Time
	StartingCash float64
	BarSource    execution.BarProvider

	// SlippageBps is the adverse price movement applied to every
	// simulated fill, in basis points (1 bp = 0.01%). Zero means
	// frictionless fills.
	SlippageBps float64

	// Commission is charged per fill, in account currency.
	Commission float64

	// CommissionPerShare is charged per unit of quantity.
	CommissionPerShare float64
}

// WithBacktest configures the App for backtest mode and wires up the
// backtest stack.
func WithBacktest(cfg BacktestConfig) Option {
	return func(a *App) { a.backtestCfg = &cfg }
}

// LiveConfig is a convenience: when set, the App wires up the live stack
// (RealClock, RealScheduler, LiveExecutor, LivePortfolio, Reconciler).
type LiveConfig struct {
	Broker execution.Client

	// FillWebhook is the address the broker's fill reports arrive on,
	// e.g. "127.0.0.1:8000". It is empty when the run does not listen
	// for them.
	//
	// The broker's webhook URL has to point at it: the proxy posts to
	// {WEBHOOK_URL}/fills, and the proxy's default webhook URL is
	// http://127.0.0.1:8000/webhook.
	FillWebhook string
}

// WithLive configures the App for live mode and wires up the live stack.
func WithLive(cfg LiveConfig) Option {
	return func(a *App) { a.liveCfg = &cfg }
}

// NewApp constructs an App with the supplied options.
func NewApp(opts ...Option) *App {
	a := &App{
		logger: logging.Nop(),
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.logger == nil {
		a.logger = logging.Nop()
	}
	return a
}

// RegisterJob queues a Job. Jobs are forwarded to the engine's scheduler
// when Run() builds the stack.
func (a *App) RegisterJob(j job.Job) error {
	if err := j.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, existing := range a.jobs {
		if existing.Name == j.Name {
			return fmt.Errorf("app: duplicate job name %q", j.Name)
		}
	}
	a.jobs = append(a.jobs, j)
	return nil
}

// AddWarmup records a (symbol, lookback) pair to be fetched before jobs run.
func (a *App) AddWarmup(symbol string, lookback int) error {
	if symbol == "" {
		return errors.New("app: warmup symbol is required")
	}
	if lookback <= 0 {
		return errors.New("app: warmup lookback must be > 0")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.warmup = append(a.warmup, warmupEntry{Symbol: symbol, Lookback: lookback})
	return nil
}

// Run executes the configured mode (backtest or live).
func (a *App) Run() error {
	return a.RunContext(context.Background())
}

// RunContext runs the app until the context is cancelled.
//
// A live run blocks until its context is done, so a caller that wants
// to stop a trading session - or a test that wants to drive one - needs
// a context it can cancel.
//
// A calendar provider that cannot answer is fatal: the run fails with
// the provider's error rather than trading on an unconfirmed schedule.
func (a *App) RunContext(ctx context.Context) (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		perr, ok := r.(*calendar.ProviderError)
		if !ok {
			panic(r)
		}
		err = fmt.Errorf("app: %w", perr)
	}()

	if err := a.resolve(); err != nil {
		return err
	}
	switch {
	case a.backtestCfg != nil:
		return a.runBacktest(ctx)
	case a.liveCfg != nil:
		return a.runLive(ctx)
	default:
		return errors.New("app: configure WithBacktest or WithLive")
	}
}
