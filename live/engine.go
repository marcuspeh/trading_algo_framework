package live

import (
	"context"
	"fmt"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/internal/ctx"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// MarketDataClient is the live market-data dependency. It serves bars
// from the live market-data client (typically IBKR OHLC).
type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error)
}

// ConstituentsClient returns ETF constituents for the current date.
type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

// Engine drives a live trading run. It runs startup reconciliation,
// then enters the scheduler's RunLive loop.
type Engine struct {
	broker       BrokerClient
	market       MarketDataClient
	constituents ConstituentsClient
	config       tf.Config
	logger       tf.Logger

	clock      Clock
	clockCtx   ctxClock
	mirror     *portfolio.MirrorPortfolio
	executor   *Executor
	scheduler  *scheduler.Scheduler
	processor  scheduler.JobProcessor
	risk       tf.RiskManager
	reconciler *Reconciler

	isTradingDay func(time.Time) bool
}

// Config configures a live Engine.
type Config struct {
	Broker       BrokerClient
	Market       MarketDataClient
	Constituents ConstituentsClient
	Config       tf.Config
	Logger       tf.Logger
	IsTradingDay func(time.Time) bool
	Risk         tf.RiskManager
}

// New constructs a live Engine.
func New(cfg Config) (*Engine, error) {
	if cfg.Broker == nil {
		return nil, fmt.Errorf("live: Broker is required")
	}
	if cfg.Market == nil {
		return nil, fmt.Errorf("live: Market is required")
	}
	if cfg.Constituents == nil {
		return nil, fmt.Errorf("live: Constituents is required")
	}
	if cfg.Config == nil {
		return nil, fmt.Errorf("live: Config is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("live: Logger is required")
	}
	if cfg.IsTradingDay == nil {
		cfg.IsTradingDay = defaultTradingDay
	}

	clock := Clock(RealClock{})
	mirror := portfolio.NewMirror()
	executor := NewExecutor(cfg.Broker)
	rec := NewReconciler(cfg.Broker, mirror, cfg.Logger)

	return &Engine{
		broker:       cfg.Broker,
		market:       cfg.Market,
		constituents: cfg.Constituents,
		config:       cfg.Config,
		logger:       cfg.Logger,
		clock:        clock,
		clockCtx:     ctxClock{inner: clock},
		mirror:       mirror,
		executor:     executor,
		reconciler:   rec,
		risk:         cfg.Risk,
		isTradingDay: cfg.IsTradingDay,
	}, nil
}

// RegisterJob registers a job on the engine's scheduler.
func (e *Engine) RegisterJob(job tf.Job) error {
	if e.scheduler == nil {
		e.scheduler = scheduler.New(e.logger)
	}
	return e.scheduler.RegisterJob(job)
}

// Executor returns the live Executor.
func (e *Engine) Executor() *Executor { return e.executor }

// Reconciler returns the Reconciler used at startup.
func (e *Engine) Reconciler() *Reconciler { return e.reconciler }

// Mirror returns the MirrorPortfolio.
func (e *Engine) Mirror() *portfolio.MirrorPortfolio { return e.mirror }

// Scheduler returns the engine's scheduler.
func (e *Engine) Scheduler() *scheduler.Scheduler { return e.scheduler }

// Reconcile runs the startup reconciliation against the broker.
func (e *Engine) Reconcile(ctx context.Context) (ReconcileResult, error) {
	res, err := e.reconciler.Reconcile(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{
		Positions:  res.Positions,
		OpenOrders: res.OpenOrders,
		HasNAV:     res.HasNAV,
		NAV:        res.NAV,
	}, nil
}

// ReconcileResult is the engine-level view of a reconciliation.
type ReconcileResult struct {
	Positions  int
	OpenOrders int
	HasNAV     bool
	NAV        float64
}

// Run performs startup reconciliation then enters RunLive. It blocks
// until ctx is cancelled or the live loop returns.
func (e *Engine) Run(parent context.Context) error {
	if e.scheduler == nil {
		return fmt.Errorf("live: no jobs registered")
	}

	e.logger.Info("live engine: startup reconciliation")
	if _, err := e.Reconcile(parent); err != nil {
		return fmt.Errorf("live: reconcile: %w", err)
	}

	tfCtx, err := ctx.New(ctx.Options{
		Mode:         tf.Live,
		Clock:        e.clockCtx,
		Portfolio:    e.mirror,
		Market:       e.market,
		Constituents: e.constituents,
		Config:       e.config,
		Logger:       e.logger,
		IsTradingDay: e.isTradingDay,
	})
	if err != nil {
		return fmt.Errorf("live: context: %w", err)
	}

	if e.processor == nil {
		e.processor = pipeline.New(e.risk, e.executor, e.logger)
	}

	e.logger.Info("live engine: entering scheduler")
	return e.scheduler.RunLive(parent, e.clock, e.processor, tfCtx)
}

func defaultTradingDay(d time.Time) bool {
	w := d.Weekday()
	return w != time.Saturday && w != time.Sunday
}

// ctxClock adapts the live Clock (which only provides Now) to the
// internal ctx.Clock interface that also requires Date().
type ctxClock struct{ inner Clock }

func (c ctxClock) Now() time.Time { return c.inner.Now() }
func (c ctxClock) Date() time.Time {
	now := c.inner.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
