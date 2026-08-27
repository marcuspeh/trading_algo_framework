// Package live is the live engine. It uses RealClock + RealScheduler +
// LiveExecutor + LivePortfolio and runs the scheduler's Run loop until
// ctx is cancelled.
package live

import (
	"context"
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/scheduler"
	"github.com/marcuspeh/trading-framework/universe"
)

// Engine drives a live trading run: reconciles on startup, then enters
// the scheduler's Run loop.
type Engine struct {
	clock      clock.Clock
	scheduler  *scheduler.Real
	executor   execution.Executor
	portfolio  *portfolio.Live
	broker     execution.BrokerClient
	risk       risk.Manager
	logger     job.Logger
	reconciler *execution.Reconciler
}

// Config configures a live Engine.
type Config struct {
	Broker       execution.BrokerClient
	Market       market.Client
	Universe     universe.Client
	Config       job.Config
	Logger       job.Logger
	Risk         risk.Manager
	Clock        clock.Clock
	Portfolio    *portfolio.Live
	IsTradingDay func(time.Time) bool
}

func New(cfg Config) (*Engine, error) {
	if cfg.Broker == nil {
		return nil, fmt.Errorf("live: Broker is required")
	}
	if cfg.Portfolio == nil {
		cfg.Portfolio = portfolio.NewLive()
	}
	if cfg.Clock == nil {
		cfg.Clock = clock.NewReal()
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("live: Logger is required")
	}
	if cfg.Risk == nil {
		cfg.Risk = risk.NewBasic()
	}

	return &Engine{
		clock:      cfg.Clock,
		scheduler:  scheduler.NewReal(cfg.Clock, cfg.Logger),
		executor:   execution.NewLive(cfg.Broker),
		portfolio:  cfg.Portfolio,
		broker:     cfg.Broker,
		risk:       cfg.Risk,
		logger:     cfg.Logger,
		reconciler: execution.NewReconciler(cfg.Broker, cfg.Portfolio, cfg.Logger),
	}, nil
}

func (e *Engine) RegisterJob(j job.Job) error { return e.scheduler.Register(j) }
func (e *Engine) Clock() clock.Clock          { return e.clock }
func (e *Engine) Portfolio() *portfolio.Live  { return e.portfolio }
func (e *Engine) Reconciler() *execution.Reconciler {
	return e.reconciler
}
func (e *Engine) Reconcile(ctx context.Context) (execution.Result, error) {
	return e.reconciler.Reconcile(ctx)
}

// Run performs startup reconciliation then enters the scheduler's
// real-time loop. Blocks until ctx is cancelled.
func (e *Engine) Run(parent context.Context, jc job.Context) error {
	if _, err := e.reconciler.Reconcile(parent); err != nil {
		return fmt.Errorf("live: reconcile: %w", err)
	}
	if !e.scheduler.HasJobs() {
		return fmt.Errorf("live: no jobs registered")
	}
	handler := pipeline.Handler(e.risk, e.executor, e.logger)
	return e.scheduler.Run(parent, jc, handler)
}
