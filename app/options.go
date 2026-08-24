// Package app is the framework's top-level wiring layer.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/logging"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// MarketDataClient is the small interface the App needs from the
// market-data adapter.
type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error)
}

// ConstituentsClient is the constituents interface used by the App.
type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

// BrokerClient is the broker interface used by the live engine.
type BrokerClient = live.BrokerClient

// BarProvider supplies backtest fills.
type BarProvider interface {
	Bar(symbol string, date time.Time) (tf.Bar, bool)
}

// Clock is the time source for live mode.
type Clock interface {
	Now() time.Time
}

// RealClock is the default live Clock.
type RealClock struct{}

// Now returns time.Now.
func (RealClock) Now() time.Time { return time.Now() }

// App is the top-level entry point used by strategy code.
type App struct {
	mu sync.Mutex

	mode tf.Mode

	logger     tf.Logger
	risk       tf.RiskManager
	tradingDay tf.TradingDayFunc

	market       MarketDataClient
	constituents ConstituentsClient
	broker       BrokerClient

	barSource BarProvider

	config    tf.Config
	liveClock Clock

	start time.Time
	end   time.Time
	cash  float64

	scheduler *scheduler.Scheduler

	warmup []warmupEntry
}

type warmupEntry struct {
	Symbol   string
	Lookback int
}

// Option configures the App.
type Option func(*App)

func WithMode(m tf.Mode) Option {
	return func(a *App) { a.mode = m }
}

func WithLogger(l tf.Logger) Option {
	return func(a *App) { a.logger = l }
}

func WithRiskManager(r tf.RiskManager) Option {
	return func(a *App) { a.risk = r }
}

func WithTradingDayFunc(f tf.TradingDayFunc) Option {
	return func(a *App) { a.tradingDay = f }
}

func WithMarket(m MarketDataClient) Option {
	return func(a *App) { a.market = m }
}

func WithConstituents(c ConstituentsClient) Option {
	return func(a *App) { a.constituents = c }
}

func WithBroker(b BrokerClient) Option {
	return func(a *App) { a.broker = b }
}

func WithConfig(c tf.Config) Option {
	return func(a *App) { a.config = c }
}

func WithLiveClock(c Clock) Option {
	return func(a *App) { a.liveClock = c }
}

func WithBacktestRange(start, end time.Time) Option {
	return func(a *App) { a.start = start; a.end = end }
}

func WithStartingCash(cash float64) Option {
	return func(a *App) { a.cash = cash }
}

func WithBarSource(b BarProvider) Option {
	return func(a *App) { a.barSource = b }
}

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

func WithStdoutLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stdout) }
}

func WithStderrLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stderr) }
}

// NewApp constructs an App with the supplied options.
func NewApp(opts ...Option) *App {
	a := &App{
		mode:       tf.Backtest,
		logger:     logging.Nop(),
		tradingDay: tf.NYSEWeekdays,
		liveClock:  RealClock{},
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.logger == nil {
		a.logger = logging.Nop()
	}
	a.scheduler = scheduler.New(a.logger)
	return a
}

// RegisterJob adds a Job to the scheduler.
func (a *App) RegisterJob(job tf.Job) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scheduler.RegisterJob(job)
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

// Run executes the configured mode.
func (a *App) Run() error {
	if err := a.validate(); err != nil {
		return err
	}

	a.mu.Lock()
	warmup := append([]warmupEntry(nil), a.warmup...)
	a.mu.Unlock()

	if len(warmup) > 0 {
		if err := a.runWarmup(warmup); err != nil {
			return fmt.Errorf("app: warmup: %w", err)
		}
	}

	a.logger.Info("app start", "mode", a.mode.String())

	switch a.mode {
	case tf.Live:
		return a.runLive()
	default:
		return a.runBacktest()
	}
}

func (a *App) validate() error {
	switch a.mode {
	case tf.Live:
		if a.broker == nil {
			return errors.New("app: live mode requires a broker (WithBroker)")
		}
	case tf.Backtest:
		if a.start.IsZero() || a.end.IsZero() {
			return errors.New("app: backtest mode requires WithBacktestRange")
		}
		if !a.end.After(a.start) {
			return errors.New("app: backtest End must be after Start")
		}
	}
	if a.market == nil {
		return errors.New("app: market data client is required (WithMarket)")
	}
	if a.constituents == nil {
		return errors.New("app: constituents client is required (WithConstituents)")
	}
	if a.config == nil {
		return errors.New("app: config is required (WithConfig)")
	}
	if !a.scheduler.HasJobs() {
		return errors.New("app: no jobs registered")
	}
	return nil
}

func (a *App) runWarmup(entries []warmupEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	asOf := time.Now()
	if a.mode == tf.Backtest {
		asOf = a.start
	}
	for _, e := range entries {
		a.logger.Info("warmup", "symbol", e.Symbol, "lookback", e.Lookback, "as_of", asOf.Format("2006-01-02"))
		bars, err := a.market.Bars(ctx, e.Symbol, e.Lookback, asOf)
		if err != nil {
			return fmt.Errorf("warmup %s: %w", e.Symbol, err)
		}
		a.logger.Info("warmup ok", "symbol", e.Symbol, "bars", len(bars))
	}
	return nil
}

func (a *App) runBacktest() error {
	fill := backtest.NewDefaultFill(a.barSource)

	engine, err := backtest.New(backtest.Config{
		Start:        a.start,
		End:          a.end,
		StartingCash: a.cash,
		Market:       a.market,
		Constituents: a.constituents,
		Config:       a.config,
		Logger:       a.logger,
		IsTradingDay: a.tradingDay,
		Fill:         fill,
		Risk:         a.risk,
	})
	if err != nil {
		return fmt.Errorf("app: backtest: %w", err)
	}

	a.mu.Lock()
	for _, j := range a.scheduler.Jobs() {
		if err := engine.RegisterJob(j); err != nil {
			a.mu.Unlock()
			return fmt.Errorf("app: register job %q: %w", j.Name, err)
		}
	}
	a.mu.Unlock()

	a.logger.Info("backtest run", "start", a.start.Format("2006-01-02"), "end", a.end.Format("2006-01-02"))
	return engine.Run(context.Background())
}

func (a *App) runLive() error {
	engine, err := newLiveEngine(a)
	if err != nil {
		return fmt.Errorf("app: live: %w", err)
	}

	a.mu.Lock()
	for _, j := range a.scheduler.Jobs() {
		if err := engine.RegisterJob(j); err != nil {
			a.mu.Unlock()
			return fmt.Errorf("app: register job %q: %w", j.Name, err)
		}
	}
	a.mu.Unlock()

	a.logger.Info("live run")
	return engine.Run(context.Background())
}
