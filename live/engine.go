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
	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/scheduler"
	"github.com/marcuspeh/trading-framework/tearsheet"
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
	fills      *fills.Registry
	observer   tearsheet.Observer

	// fillAddr is the address the broker's fill reports arrive on, and
	// fillServer is the listener that takes them. Both are empty when
	// the run does not listen for fills.
	fillAddr   string
	fillServer *fills.Server
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

	// FillWebhook is the address the broker's fill reports arrive on,
	// e.g. "127.0.0.1:8000". It is empty when the run does not listen
	// for them.
	//
	// The broker's webhook URL has to point at it: the proxy posts to
	// {WEBHOOK_URL}/fills.
	FillWebhook string

	// Fills names the orders the executor placed, so a fill report can
	// be logged against the order that filled.
	Fills *fills.Registry

	// Observer watches the account's value. It is optional: an engine
	// without one keeps the value to itself.
	Observer tearsheet.Observer
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
		fills:     cfg.Fills,
		observer:  cfg.Observer,
		fillAddr:  cfg.FillWebhook,
		reconciler: execution.NewReconciler(cfg.Broker, cfg.Portfolio, cfg.Logger).
			WithPrices(cfg.Market, cfg.Session),
	}, nil
}

func (e *Engine) RegisterJob(j job.Job) error { return e.scheduler.Register(j) }
func (e *Engine) Clock() clock.Clock          { return e.clock }
func (e *Engine) Portfolio() *portfolio.Live  { return e.portfolio }

// FillAddr returns the address the fill listener is on.
//
// It is empty until the run starts the listener, and it is the address
// the broker's webhook URL has to point at.
func (e *Engine) FillAddr() string {
	if e.fillServer == nil {
		return ""
	}
	return e.fillServer.Addr()
}
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
	if err := e.startFillWebhook(); err != nil {
		return err
	}
	defer e.stopFillWebhook()
	if _, err := e.reconciler.Reconcile(parent); err != nil {
		return fmt.Errorf("live: reconcile: %w", err)
	}
	if !e.scheduler.HasJobs() {
		return fmt.Errorf("live: no jobs registered")
	}

	// The account's value when the run started: the first point of the
	// equity curve.
	e.observeEquity(e.clock.Now())

	// The portfolio is refreshed from the broker before every job, so
	// a job that asks what it holds sees what the broker says it
	// holds - not what it held when the process started.
	e.scheduler.SetBeforeJob(func(ctx context.Context, _ scheduler.JobContext, _ job.Job, _ time.Time) error {
		if _, err := e.reconciler.Reconcile(ctx); err != nil {
			return err
		}
		// The value is taken after the reconciliation, so it is the
		// broker's own number rather than the framework's guess.
		e.observeEquity(e.clock.Now())
		return nil
	})

	handler := pipeline.Handler(e.risk, e.executor, e.logger)
	err := e.scheduler.Run(parent, jc, handler)

	// The run is over: the account is worth what the broker says it
	// is worth now, and that is the last point of the curve.
	//
	// The reconciliation runs on a fresh context: the run's own is
	// usually done by now, and a cancelled context would fail the
	// call that closes the account's books.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, rerr := e.reconciler.Reconcile(ctx); rerr != nil {
		e.logger.Warn("final reconcile failed", "err", rerr.Error())
	} else {
		e.observeEquity(e.clock.Now())
	}
	return err
}

// observeEquity reports the account's value to the observer.
func (e *Engine) observeEquity(at time.Time) {
	if e.observer == nil {
		return
	}
	e.observer.OnEquity(at, e.portfolio.Value())
}

// startFillWebhook begins listening for the broker's fill reports.
//
// A listener that cannot bind fails the run: a run that cannot hear
// its fills would log nothing about them and look healthy.
func (e *Engine) startFillWebhook() error {
	if e.fills == nil || e.fillAddr == "" {
		return nil
	}
	srv := fills.NewServer(e.fillAddr, e.fills, e.logger)
	if err := srv.Start(); err != nil {
		return err
	}
	e.fillServer = srv
	e.logger.Info("listening for fill webhooks", "addr", e.fillAddr)
	return nil
}

// stopFillWebhook shuts the fill listener down.
func (e *Engine) stopFillWebhook() {
	if e.fillServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.fillServer.Stop(ctx)
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
