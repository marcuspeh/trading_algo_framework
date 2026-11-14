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
		// fires, no mark-to-market happens, and resting orders are not
		// re-evaluated, because there is no bar for the day.
		if !e.isTradingDay(cur) {
			continue
		}

		// Give resting orders from previous days a chance to trigger
		// against today's bar before the day's jobs run.
		e.advanceRestingOrders(cur)

		for _, at := range e.slotsOn(cur) {
			e.clock.Set(at)
			e.markToMarket(cur)
			if err := e.scheduler.FireAt(parent, jc, e.handler, at); err != nil {
				return err
			}
		}
	}
	return nil
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

// advanceRestingOrders retries unfilled orders against the given day's
// bar, so limit orders are good-till-cancelled rather than expiring on
// the day they were submitted.
func (e *Engine) advanceRestingOrders(date time.Time) {
	bt, ok := e.executor.(*execution.Backtest)
	if !ok {
		return
	}
	bt.AdvanceDay(date)
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
