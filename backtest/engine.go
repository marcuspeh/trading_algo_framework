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
	tfcontext "github.com/marcuspeh/trading-framework/context"
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
	clock     *clock.BacktestClock
	portfolio *portfolio.Backtest
	scheduler *scheduler.Backtest
	market    market.Client
	executor  execution.Executor
	risk      risk.Manager
	logger    job.Logger
	handler   scheduler.Handler

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

	clk := clock.NewBacktest(cfg.Start)
	e := &Engine{
		clock:     clk,
		portfolio: cfg.Portfolio,
		scheduler: scheduler.NewBacktest(cfg.Logger),
		market:    cfg.Market,
		executor:  cfg.Executor,
		risk:      cfg.Risk,
		logger:    cfg.Logger,
		start:     util.GetDay(cfg.Start),
		end:       util.GetDay(cfg.End),
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
// the clock and tells the scheduler to fire every Job whose TimeOfDay
// matches the current simulated time.
func (e *Engine) Run(parent context.Context, jc job.Context) error {
	if !e.scheduler.HasJobs() {
		return fmt.Errorf("backtest: no jobs registered")
	}

	for cur := e.start; !cur.After(e.end); cur = cur.AddDate(0, 0, 1) {
		if err := parent.Err(); err != nil {
			return nil
		}
		tfcontext.Reset(jc)

		slots := uniqueSlots(e.scheduler.Jobs())
		for _, slot := range slots {
			at := time.Date(cur.Year(), cur.Month(), cur.Day(),
				slot.Hour, slot.Minute, 0, 0, cur.Location())
			e.clock.Set(at)
			e.markToMarket(cur)
			if err := e.scheduler.FireAt(parent, jc, e.handler, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) markToMarket(date time.Time) {
	for _, pos := range e.portfolio.Positions() {
		bars, err := e.market.Bars(context.Background(), pos.Symbol, 1, date)
		if err != nil || len(bars) == 0 {
			continue
		}
		e.portfolio.MarkPrice(pos.Symbol, bars[len(bars)-1].Close)
	}
}

func uniqueSlots(jobs []job.Job) []job.TimeOfDay {
	seen := map[job.TimeOfDay]struct{}{}
	out := []job.TimeOfDay{}
	for _, j := range jobs {
		if _, ok := seen[j.Time]; ok {
			continue
		}
		seen[j.Time] = struct{}{}
		out = append(out, j.Time)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hour != out[j].Hour {
			return out[i].Hour < out[j].Hour
		}
		return out[i].Minute < out[j].Minute
	})
	return out
}
