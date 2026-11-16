// Package live is the live engine. It uses RealClock + RealScheduler +
// LiveExecutor + LivePortfolio and runs the scheduler's Run loop until
// ctx is cancelled.
package live

import (
	"context"
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// Engine drives a live trading run: reconciles on startup, then enters
// the scheduler's Run loop.
type Engine struct {
	clock      clock.Clock
	scheduler  *scheduler.Real
	executor   execution.Executor
	portfolio  *portfolio.Live
	broker     execution.Client
	session    *clock.Session
	risk       risk.Manager
	logger     job.Logger
	reconciler *execution.Reconciler
}

// Config configures a live Engine.
type Config struct {
	Broker       execution.Client
	Executor     execution.Executor
	Market       market.Client
	Logger       job.Logger
	Risk         risk.Manager
	Clock        clock.Clock
	Session      *clock.Session
	Portfolio    *portfolio.Live
	IsTradingDay func(time.Time) bool

	// Calendar is the exchange calendar. It is required when no
	// Session is supplied: the session's boundaries come from it.
	Calendar calendar.Provider
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
	if cfg.Session == nil {
		if cfg.Calendar == nil {
			return nil, fmt.Errorf("live: Calendar is required")
		}
		cfg.Session = clock.NewSession(cfg.Clock, cfg.Calendar)
	}
	if cfg.Logger == nil {
		cfg.Logger = nopLogger{}
	}
	if cfg.Risk == nil {
		cfg.Risk = risk.NewBasic()
	}
	if cfg.IsTradingDay == nil {
		// A calendar knows which days the exchange traded. When the
		// caller supplied one, the engine honours it: jobs do not fire
		// on a weekend or a holiday.
		cfg.IsTradingDay = cfg.Session.IsTradingDay
	}

	// The executor is the same one a backtest uses, so both modes
	// translate an order the same way.
	executor := cfg.Executor
	if executor == nil {
		executor = execution.NewBroker(cfg.Broker)
	}

	sched := scheduler.NewReal(cfg.Clock, cfg.Logger)
	// Jobs do not fire on a day the exchange is closed.
	sched.SetTradingDay(cfg.IsTradingDay)
	// Close-relative slots resolve against the same session the
	// reconciler and the context use.
	sched.SetSession(cfg.Session)

	return &Engine{
		clock:     cfg.Clock,
		scheduler: sched,
		executor:  executor,
		portfolio: cfg.Portfolio,
		broker:    cfg.Broker,
		session:   cfg.Session,
		risk:      cfg.Risk,
		logger:    cfg.Logger,
		reconciler: execution.NewReconciler(cfg.Broker, cfg.Portfolio, cfg.Logger).
			WithPrices(cfg.Market, cfg.Session),
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

// Run checks the broker is connected, performs the startup
// reconciliation, then enters the scheduler's real-time loop. Blocks
// until ctx is cancelled.
func (e *Engine) Run(parent context.Context, jc job.Context) error {
	if err := e.checkHealth(parent); err != nil {
		return err
	}
	if _, err := e.reconciler.Reconcile(parent); err != nil {
		return fmt.Errorf("live: reconcile: %w", err)
	}
	if !e.scheduler.HasJobs() {
		return fmt.Errorf("live: no jobs registered")
	}

	// The portfolio is refreshed from the broker before every job, so
	// a job that asks what it holds sees what the broker says it
	// holds - not what it held when the process started.
	e.scheduler.SetBeforeJob(func(ctx context.Context, _ scheduler.JobContext, _ job.Job, _ time.Time) error {
		_, err := e.reconciler.Reconcile(ctx)
		return err
	})

	handler := pipeline.Handler(e.risk, e.executor, e.logger)
	return e.scheduler.Run(parent, jc, handler)
}

// checkHealth fails fast when the broker is not connected.
//
// The broker's health endpoint answers even when the exchange behind
// it is down, so the connection flag is what matters: without it every
// order would be rejected one at a time, long after the cause.
func (e *Engine) checkHealth(ctx context.Context) error {
	res, err := e.broker.Health(ctx)
	if err != nil {
		return fmt.Errorf("live: broker health check failed: %w", err)
	}
	if res == nil {
		return fmt.Errorf("live: broker health check returned nothing")
	}
	if !res.IBKRConnected {
		return fmt.Errorf("live: broker is not connected (status %q)", res.Status)
	}
	e.logger.Info("broker connected", "status", res.Status, "client_id", res.ClientID)
	return nil
}

// nopLogger discards the engine's log lines.
type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
