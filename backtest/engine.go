package backtest

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

// MarketDataClient is the backtest market-data dependency. It serves
// bars from the engine's pre-loaded dataset for the current simulated
// date.
type MarketDataClient interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error)
}

// ConstituentsClient returns constituents for a given simulated date.
type ConstituentsClient interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}

// Engine drives a backtest: walks a date range, advances the simulated
// clock, and lets the registered scheduler fire jobs at each slot.
type Engine struct {
	clock        *Clock
	portfolio    *portfolio.MemPortfolio
	exec         *Executor
	market       MarketDataClient
	constituents ConstituentsClient
	config       tf.Config
	logger       tf.Logger
	scheduler    *scheduler.Scheduler
	processor    scheduler.JobProcessor
	isTradingDay func(time.Time) bool

	start time.Time
	end   time.Time
}

// Config configures a backtest Engine.
type Config struct {
	Start        time.Time
	End          time.Time
	StartingCash float64
	Market       MarketDataClient
	Constituents ConstituentsClient
	Config       tf.Config
	Logger       tf.Logger
	IsTradingDay func(time.Time) bool
	Fill         FillModel
	BarSource    BarProvider
	Risk         tf.RiskManager
}

// New constructs a backtest Engine. It does not start running until
// Run is called.
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
	if cfg.Constituents == nil {
		return nil, fmt.Errorf("backtest: Constituents is required")
	}
	if cfg.Config == nil {
		return nil, fmt.Errorf("backtest: Config is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("backtest: Logger is required")
	}
	if cfg.IsTradingDay == nil {
		cfg.IsTradingDay = defaultTradingDay
	}
	if cfg.Fill == nil {
		if cfg.BarSource == nil {
			return nil, fmt.Errorf("backtest: Fill or BarSource required")
		}
		cfg.Fill = NewDefaultFill(cfg.BarSource)
	}

	start := startOfDay(cfg.Start)
	end := startOfDay(cfg.End)

	clock := NewClock(start)
	port := portfolio.NewMem(cfg.StartingCash)
	exec := NewExecutor(port, cfg.Fill, clock)

	return &Engine{
		clock:        clock,
		portfolio:    port,
		exec:         exec,
		market:       cfg.Market,
		constituents: cfg.Constituents,
		config:       cfg.Config,
		logger:       cfg.Logger,
		isTradingDay: cfg.IsTradingDay,
		start:        start,
		end:          end,
	}, nil
}

// RegisterJob registers a job on the engine's scheduler.
func (e *Engine) RegisterJob(job tf.Job) error {
	if e.scheduler == nil {
		e.scheduler = scheduler.New(e.logger)
	}
	return e.scheduler.RegisterJob(job)
}

// Executor returns the backtest Executor. Useful for tests that need
// to inspect fills or open orders.
func (e *Engine) Executor() *Executor { return e.exec }

// Portfolio returns the MemPortfolio used by the backtest.
func (e *Engine) Portfolio() *portfolio.MemPortfolio { return e.portfolio }

// Clock returns the deterministic clock.
func (e *Engine) Clock() *Clock { return e.clock }

// Scheduler returns the engine's scheduler. Useful for tests.
func (e *Engine) Scheduler() *scheduler.Scheduler { return e.scheduler }

// Run executes the backtest. For each calendar day in [Start, End],
// it advances the clock and runs the scheduler's RunBacktest method.
func (e *Engine) Run(parent context.Context) error {
	if e.scheduler == nil {
		return fmt.Errorf("backtest: no jobs registered")
	}

	tfCtx, err := ctx.New(ctx.Options{
		Mode:         tf.Backtest,
		Clock:        e.clock,
		Portfolio:    e.portfolio,
		Market:       e.market,
		Constituents: e.constituents,
		Config:       e.config,
		Logger:       e.logger,
		IsTradingDay: e.isTradingDay,
	})
	if err != nil {
		return fmt.Errorf("backtest: context: %w", err)
	}

	if e.processor == nil {
		e.processor = pipeline.New(e.riskOrDefault(), e.exec, e.logger)
	}

	cur := e.start
	for {
		if err := parent.Err(); err != nil {
			return nil
		}
		if cur.After(e.end) {
			return nil
		}

		e.clock.SetDate(cur)
		e.clock.SetNow(time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location()))

		// Reset bar cache at each new simulated date so strategy calls
		// to ctx.Bars see the new asOf.
		ctx.ResetBarsCache(tfCtx)

		// Mark-to-market held positions with today's close.
		e.markToMarket(cur)

		// Run the day's slots starting at 00:00 local time.
		if err := e.scheduler.RunBacktest(parent, e.processor, tfCtx, cur, e.clock.Now()); err != nil {
			return err
		}

		cur = cur.AddDate(0, 0, 1)
	}
}

func (e *Engine) riskOrDefault() tf.RiskManager {
	// The engine never sees a custom RiskManager from Config (that
	// field is wired by the App, not the engine). The default
	// basic risk manager is always used at this layer; strategies
	// can extend it via the App.
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

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func defaultTradingDay(d time.Time) bool {
	w := d.Weekday()
	return w != time.Saturday && w != time.Sunday
}
