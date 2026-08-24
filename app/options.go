// Package app is the framework's top-level wiring layer.
//
// The App accepts Options (mode, clients, executor, risk, logger,
// clock, jobs, warmup) and provides:
//   - RegisterJob
//   - AddWarmup
//   - Run
//
// Internally the App builds the right engine (backtest or live) and
// hands off execution. Strategy code only interacts with the App's
// public surface.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/logging"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// MarketDataClient is the small interface the App needs from the
// market-data adapter. Internal/3rd-party adapters supply this.
type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error)
}

// ConstituentsClient is the constituents interface used by the App.
type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

// BrokerClient is the broker interface used by the live engine. Live
// mode is the only place this is required.
type BrokerClient interface {
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error)
	CancelOrder(ctx context.Context, orderID int) error
	GetPositions(ctx context.Context) ([]PositionResponse, error)
	GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error)
	GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error)
}

// MarketOrderRequest mirrors broker client shape.
type MarketOrderRequest struct {
	Symbol        string
	Action        string
	TotalQuantity float64
	OrderRef      string
	OutsideRth    bool
}

// LimitOrderRequest mirrors broker client shape.
type LimitOrderRequest struct {
	MarketOrderRequest
	LmtPrice float64
}

// OrderResponse mirrors broker client shape.
type OrderResponse struct {
	PermID  int
	OrderID int
	Status  string
}

// PositionResponse mirrors broker client shape.
type PositionResponse struct {
	ConID       int
	Symbol      string
	Quantity    float64
	AverageCost float64
}

// OpenOrderResponse mirrors broker client shape.
type OpenOrderResponse struct {
	PermID        int
	OrderID       int
	Symbol        string
	Action        string
	TotalQuantity float64
	LmtPrice      *float64
	AuxPrice      *float64
	Status        string
	Remaining     float64
}

// AccountNAVResponse mirrors broker client shape.
type AccountNAVResponse struct {
	Account  string
	NAV      float64
	Currency string
}

// BarProvider supplies backtest fills.
type BarProvider interface {
	Bar(symbol string, date time.Time) (tf.Bar, bool)
}

// Clock is the time source for live mode. Backtest mode uses an
// internal deterministic clock regardless of this option.
type Clock interface {
	Now() time.Time
}

// RealClock is the default live Clock.
type RealClock struct{}

// Now returns time.Now().
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

// warmupEntry records a symbol/lookback pair registered via AddWarmup.
type warmupEntry struct {
	Symbol   string
	Lookback int
}

// Option configures the App.
type Option func(*App)

// WithMode selects Backtest or Live.
func WithMode(m tf.Mode) Option {
	return func(a *App) { a.mode = m }
}

// WithLogger sets the framework logger.
func WithLogger(l tf.Logger) Option {
	return func(a *App) { a.logger = l }
}

// WithRiskManager overrides the default risk manager.
func WithRiskManager(r tf.RiskManager) Option {
	return func(a *App) { a.risk = r }
}

// WithTradingDayFunc overrides the default trading-day predicate.
func WithTradingDayFunc(f tf.TradingDayFunc) Option {
	return func(a *App) { a.tradingDay = f }
}

// WithMarket sets the market-data client (required for both modes).
func WithMarket(m MarketDataClient) Option {
	return func(a *App) { a.market = m }
}

// WithConstituents sets the constituents client (required for both modes).
func WithConstituents(c ConstituentsClient) Option {
	return func(a *App) { a.constituents = c }
}

// WithBroker sets the broker client (required for live mode).
func WithBroker(b BrokerClient) Option {
	return func(a *App) { a.broker = b }
}

// WithConfig sets the configuration adapter (required for both modes).
func WithConfig(c tf.Config) Option {
	return func(a *App) { a.config = c }
}

// WithLiveClock sets a custom Clock for live mode (default RealClock).
func WithLiveClock(c Clock) Option {
	return func(a *App) { a.liveClock = c }
}

// WithBacktestRange sets the simulated date range for backtest mode.
func WithBacktestRange(start, end time.Time) Option {
	return func(a *App) { a.start = start; a.end = end }
}

// WithStartingCash sets the initial cash for backtest mode.
func WithStartingCash(cash float64) Option {
	return func(a *App) { a.cash = cash }
}

// WithBarSource sets the BarProvider used by the default backtest
// fill model.
func WithBarSource(b BarProvider) Option {
	return func(a *App) { a.barSource = b }
}

// WithLogFile creates a TXT-file logger writing to the supplied path.
// Mutually convenient with WithLogger.
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

// WithStdoutLogger writes log lines to stdout. Useful for development.
func WithStdoutLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stdout) }
}

// WithStderrLogger writes log lines to stderr.
func WithStderrLogger() Option {
	return func(a *App) { a.logger = logging.NewTextLogger(os.Stderr) }
}

// NewApp constructs an App with the supplied options.
func NewApp(opts ...Option) *App {
	a := &App{
		mode:       tf.Backtest,
		logger:     logging.Nop(),
		risk:       nil,
		tradingDay: tf.NYSEWeekdays,
		liveClock:  RealClock{},
		scheduler:  scheduler.New(nil), // logger injected via Resolve below
	}
	for _, opt := range opts {
		opt(a)
	}
	a.scheduler = scheduler.New(a.logger)
	return a
}

// RegisterJob adds a Job to the scheduler. Returns the underlying
// validation/duplicate errors unchanged.
func (a *App) RegisterJob(job tf.Job) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scheduler.RegisterJob(job)
}

// AddWarmup records a (symbol, lookback) pair to be fetched before
// jobs run. The bar data is fetched at Run() time.
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

// Run executes the configured mode. In backtest mode it builds a
// backtest.Engine and runs to completion. In live mode it builds a
// live.Engine, reconciles, then enters the live loop.
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
	if a.logger == nil {
		a.logger = logging.Nop()
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

	// Forward jobs to the engine's scheduler.
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

// Discard is a Logger that drops everything. Used in tests.
type discard struct{}

func (discard) Info(string, ...any)  {}
func (discard) Warn(string, ...any)  {}
func (discard) Error(string, ...any) {}

// DiscardLogger returns a Logger that throws output away.
func DiscardLogger() tf.Logger { return discard{} }

// Compile-time check.
var _ io.Writer = (*os.File)(nil)
