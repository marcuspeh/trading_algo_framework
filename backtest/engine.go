// Package backtest is the backtest engine. It drives the BacktestClock
// and the BacktestScheduler in deterministic chronological order. There
// is no real-time waiting.
package backtest

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/marcuspeh/go-tools/util"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// Engine drives a backtest run.
type Engine struct {
	clock        *clock.BacktestClock
	portfolio    *portfolio.Backtest
	scheduler    *scheduler.Backtest
	market       market.Client
	executor     execution.Executor
	risk         risk.Manager
	logger       job.Logger
	handler      scheduler.Handler
	isTradingDay portfolio.TradingDayFunc
	session      *clock.Session
	broker       broker.SessionAdvancer

	// lastSession is the most recent session the resting orders have
	// been evaluated against.
	lastSession time.Time

	start time.Time
	end   time.Time
}

// Config configures a backtest Engine.
type Config struct {
	Start     time.Time
	End       time.Time
	Market    market.Client
	Config    job.Config
	Logger    job.Logger
	Portfolio *portfolio.Backtest
	Executor  execution.Executor
	Risk      risk.Manager
	// Clock is the simulated clock the engine advances. It must be the
	// same instance the strategy Context and Executor were built with,
	// otherwise the Context would observe a frozen date. When nil, the
	// engine creates its own BacktestClock.
	Clock *clock.BacktestClock

	// IsTradingDay decides which dates are trading days. Jobs do not
	// fire on a non-trading day. When nil, every day is a trading day.
	IsTradingDay portfolio.TradingDayFunc

	// Session resolves the session a timestamp belongs to. It must be
	// the same Session the Context was built with.
	Session *clock.Session

	// Broker is the simulated broker. When it implements
	// broker.SessionAdvancer, the engine tells it when a session has
	// closed so it can evaluate the orders resting at it.
	Broker broker.SessionAdvancer
}

func New(cfg Config) (*Engine, error) {
	if cfg.Start.IsZero() || cfg.End.IsZero() {
		return nil, fmt.Errorf("backtest: Start and End are required")
	}
	if !cfg.End.After(cfg.Start) {
		return nil, fmt.Errorf("backtest: End must be after Start")
	}
	if cfg.Market == nil {
		return nil, fmt.Errorf("backtest: Market is required")
	}
	if cfg.Portfolio == nil {
		return nil, fmt.Errorf("backtest: Portfolio is required")
	}
	if cfg.Executor == nil {
		return nil, fmt.Errorf("backtest: Executor is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("backtest: Logger is required")
	}
	if cfg.Risk == nil {
		cfg.Risk = risk.NewBasic()
	}

	clk := cfg.Clock
	if clk == nil {
		clk = clock.NewBacktest(cfg.Start)
	}
	isTradingDay := cfg.IsTradingDay
	if isTradingDay == nil {
		isTradingDay = func(time.Time) bool { return true }
	}
	e := &Engine{
		clock:        clk,
		portfolio:    cfg.Portfolio,
		scheduler:    scheduler.NewBacktest(cfg.Logger),
		market:       cfg.Market,
		executor:     cfg.Executor,
		risk:         cfg.Risk,
		logger:       cfg.Logger,
		start:        util.GetDay(cfg.Start),
		end:          util.GetDay(cfg.End),
		isTradingDay: isTradingDay,
		session:      cfg.Session,
		broker:       cfg.Broker,
	}
	e.handler = pipeline.Handler(cfg.Risk, cfg.Executor, cfg.Logger)
	return e, nil
}

// RegisterJob registers a Job on the backtest scheduler.
func (e *Engine) RegisterJob(j job.Job) error { return e.scheduler.Register(j) }

// Clock returns the injected Clock.
func (e *Engine) Clock() *clock.BacktestClock { return e.clock }

// Portfolio returns the simulated portfolio.
func (e *Engine) Portfolio() *portfolio.Backtest { return e.portfolio }

// Scheduler returns the backtest scheduler.
func (e *Engine) Scheduler() *scheduler.Backtest { return e.scheduler }

// Run executes the backtest. For each day in [Start, End], it advances
// the clock and tells the scheduler to fire every Job whose slot
// matches the current simulated time.
//
// A job's slot is resolved against the day being simulated, so a
// close-relative slot fires at 13:00 on an early-close day and 16:00
// on an ordinary one.
func (e *Engine) Run(parent context.Context, jc job.Context) error {
	if !e.scheduler.HasJobs() {
		return fmt.Errorf("backtest: no jobs registered")
	}

	for cur := e.start; !cur.After(e.end); cur = cur.AddDate(0, 0, 1) {
		if err := parent.Err(); err != nil {
			return nil
		}

		// The exchange is closed on weekends and holidays: no job
		// fires and no mark-to-market happens, because there is no bar
		// for the day.
		if !e.isTradingDay(cur) {
			continue
		}

		for _, at := range e.slotsOn(cur) {
			e.clock.Set(at)

			// A session's bar is final at its close, so any session
			// that has closed is evaluated before the job runs. A
			// job at the close therefore sees the fills that session
			// produced.
			anchor := e.anchor(at)
			e.advanceSessions(anchor)

			e.markToMarket(anchor)
			if err := e.scheduler.FireAt(parent, jc, e.handler, at); err != nil {
				return err
			}
		}
	}
	return nil
}

// anchor returns the session a timestamp belongs to.
func (e *Engine) anchor(at time.Time) time.Time {
	if e.session != nil {
		return e.session.Anchor(at)
	}
	return at
}

// slotsOn returns the distinct slot times for the given day, in
// chronological order.
//
// Slots are resolved per day, so the same job can fire at different
// times on an ordinary day and an early-close day.
func (e *Engine) slotsOn(day time.Time) []time.Time {
	jobs := e.scheduler.Jobs()
	seen := map[int64]struct{}{}
	out := []time.Time{}

	for _, j := range jobs {
		at := j.Time.Resolve(day)
		key := at.UnixNano()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, at)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})
	return out
}

// advanceRestingOrders evaluates the orders resting at the broker
// against a session's bar.
//
// A session's bar is final at its close, so the engine evaluates it
// as soon as the simulated time passes the close. A limit order that
// did not trigger keeps resting, which is what makes it good-till-
// cancelled.
func (e *Engine) advanceRestingOrders(session time.Time) {
	if e.broker == nil {
		return
	}
	e.broker.AdvanceSession(session)
}

// advanceSessions evaluates every session that has closed but has not
// been evaluated yet.
//
// A session is evaluated once, in order, so an order is never
// evaluated twice against the same bar.
func (e *Engine) advanceSessions(to time.Time) {
	if e.broker == nil || e.session == nil {
		return
	}
	if e.lastSession.IsZero() {
		e.lastSession = e.session.Anchor(e.start)
	}
	for s := e.nextSession(e.lastSession); !s.After(to); s = e.nextSession(s) {
		e.advanceRestingOrders(s)
		e.lastSession = s
	}
}

// nextSession returns the trading day after the given one.
func (e *Engine) nextSession(after time.Time) time.Time {
	candidate := after.AddDate(0, 0, 1)
	for i := 0; i < 14; i++ {
		if e.isTradingDay(candidate) {
			return candidate
		}
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}

func (e *Engine) markToMarket(date time.Time) {
	for _, pos := range e.portfolio.Positions() {
		bars, err := e.market.Bars(context.Background(), pos.Symbol, 1, date)
		if err != nil {
			// Logged rather than swallowed: a stale mark silently
			// understates the portfolio value for the rest of the day.
			e.logger.Warn("mark-to-market failed",
				"symbol", pos.Symbol,
				"date", date.Format("2006-01-02"),
				"err", err.Error(),
			)
			continue
		}
		if len(bars) == 0 {
			e.logger.Warn("mark-to-market found no bar",
				"symbol", pos.Symbol,
				"date", date.Format("2006-01-02"),
			)
			continue
		}
		e.portfolio.MarkPrice(pos.Symbol, bars[len(bars)-1].Close)
	}
}
