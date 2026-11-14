// Package app is the framework's top-level wiring layer. Strategy code
// constructs an App via NewApp + functional options, registers Jobs, and
// calls Run.
package app

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
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

	clock      clock.Clock
	btClock    *clock.BacktestClock
	executor   execution.Executor
	portfolio  portfolio.Portfolio
	risk       RiskManager
	market     MarketClient
	universe   universe.Client
	config     job.Config
	logger     job.Logger
	tradingDay portfolio.TradingDayFunc

	// Convenience configs. If set, Run() wires the matching stack.
	backtestCfg *BacktestConfig
	liveCfg     *LiveConfig

	// Engines, populated after resolve(). Exposed for inspection in tests.
	btEngine   *backtestEngine
	liveEngine *liveEngine

	// liveBroker is the broker used by the live stack.
	liveBroker execution.Client

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

func WithRiskManager(r RiskManager) Option                 { return func(a *App) { a.risk = r } }
func WithMarketClient(m MarketClient) Option               { return func(a *App) { a.market = m } }
func WithUniverseClient(u universe.Client) Option          { return func(a *App) { a.universe = u } }
func WithConfigClient(c job.Config) Option                 { return func(a *App) { a.config = c } }
func WithLogger(l job.Logger) Option                       { return func(a *App) { a.logger = l } }
func WithTradingDayFunc(f portfolio.TradingDayFunc) Option { return func(a *App) { a.tradingDay = f } }

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
}

// WithLive configures the App for live mode and wires up the live stack.
func WithLive(cfg LiveConfig) Option {
	return func(a *App) { a.liveCfg = &cfg }
}

// NewApp constructs an App with the supplied options.
func NewApp(opts ...Option) *App {
	a := &App{
		logger:     logging.Nop(),
		tradingDay: portfolio.NYSEWeekdays,
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
	if err := a.resolve(); err != nil {
		return err
	}
	switch {
	case a.backtestCfg != nil:
		return a.runBacktest()
	case a.liveCfg != nil:
		return a.runLive()
	default:
		return errors.New("app: configure WithBacktest or WithLive")
	}
}
