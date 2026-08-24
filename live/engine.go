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

type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error)
}

type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

// Engine drives a live trading run: reconciles on startup, then enters
// the scheduler's RunLive loop.
type Engine struct {
	broker       BrokerClient
	market       MarketDataClient
	constituents ConstituentsClient
	config       tf.Config
	logger       tf.Logger

	clock      Clock
	mirror     *portfolio.MirrorPortfolio
	executor   *Executor
	scheduler  *scheduler.Scheduler
	processor  scheduler.JobProcessor
	risk       tf.RiskManager
	reconciler *Reconciler

	isTradingDay func(time.Time) bool
}

type Config struct {
	Broker       BrokerClient
	Market       MarketDataClient
	Constituents ConstituentsClient
	Config       tf.Config
	Logger       tf.Logger
	IsTradingDay func(time.Time) bool
	Risk         tf.RiskManager
	Clock        Clock
}

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
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}

	mirror := portfolio.NewMirror()
	return &Engine{
		broker:       cfg.Broker,
		market:       cfg.Market,
		constituents: cfg.Constituents,
		config:       cfg.Config,
		logger:       cfg.Logger,
		clock:        cfg.Clock,
		mirror:       mirror,
		executor:     NewExecutor(cfg.Broker),
		reconciler:   NewReconciler(cfg.Broker, mirror, cfg.Logger),
		risk:         cfg.Risk,
		isTradingDay: cfg.IsTradingDay,
	}, nil
}

func (e *Engine) RegisterJob(job tf.Job) error {
	if e.scheduler == nil {
		e.scheduler = scheduler.New(e.logger)
	}
	return e.scheduler.RegisterJob(job)
}

func (e *Engine) Executor() *Executor                { return e.executor }
func (e *Engine) Reconciler() *Reconciler            { return e.reconciler }
func (e *Engine) Mirror() *portfolio.MirrorPortfolio { return e.mirror }
func (e *Engine) Scheduler() *scheduler.Scheduler    { return e.scheduler }

func (e *Engine) Reconcile(ctx context.Context) (ReconcileResult, error) {
	res, err := e.reconciler.Reconcile(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult(res), nil
}

type ReconcileResult struct {
	Positions  int
	OpenOrders int
	HasNAV     bool
	NAV        float64
}

// Run performs startup reconciliation then enters RunLive. It blocks
// until ctx is cancelled.
func (e *Engine) Run(parent context.Context) error {
	if e.scheduler == nil {
		return fmt.Errorf("live: no jobs registered")
	}

	if _, err := e.Reconcile(parent); err != nil {
		return fmt.Errorf("live: reconcile: %w", err)
	}

	tfCtx, err := ctx.New(ctx.Options{
		Mode:         tf.Live,
		Clock:        clockWithDate{e.clock},
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

	return e.scheduler.RunLive(parent, e.clock, e.processor, tfCtx)
}

func defaultTradingDay(d time.Time) bool {
	w := d.Weekday()
	return w != time.Saturday && w != time.Sunday
}

// clockWithDate adapts a live Clock (Now only) to the internal ctx.Clock
// interface (Now + Date).
type clockWithDate struct{ inner Clock }

func (c clockWithDate) Now() time.Time { return c.inner.Now() }
func (c clockWithDate) Date() time.Time {
	now := c.inner.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
