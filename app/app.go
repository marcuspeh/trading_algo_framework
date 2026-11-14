package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/clock"
	tfcontext "github.com/marcuspeh/trading-framework/context"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
)

// resolve wires up the convenience stacks (backtest/live) and validates
// required dependencies.
func (a *App) resolve() error {
	switch {
	case a.backtestCfg != nil:
		return a.resolveBacktest()
	case a.liveCfg != nil:
		return a.resolveLive()
	default:
		return fmt.Errorf("app: no mode configured (use WithBacktest or WithLive)")
	}
}

func (a *App) resolveBacktest() error {
	cfg := a.backtestCfg
	if cfg.Start.IsZero() || cfg.End.IsZero() {
		return fmt.Errorf("app: backtest: Start and End are required")
	}
	if !cfg.End.After(cfg.Start) {
		return fmt.Errorf("app: backtest: End must be after Start")
	}
	if cfg.BarSource == nil {
		return fmt.Errorf("app: backtest: BarSource is required")
	}
	if a.market == nil {
		return fmt.Errorf("app: backtest: market client is required (WithMarketClient)")
	}
	if a.universe == nil {
		return fmt.Errorf("app: backtest: universe client is required (WithUniverseClient)")
	}
	if a.config == nil {
		return fmt.Errorf("app: backtest: config client is required (WithConfigClient)")
	}

	a.btClock = clock.NewBacktest(clock.StartOfDay(cfg.Start))
	a.clock = a.btClock
	a.portfolio = portfolio.NewBacktest(cfg.StartingCash)
	fill := &execution.DefaultFill{
		Bars:               cfg.BarSource,
		SlippageBps:        cfg.SlippageBps,
		Commission:         cfg.Commission,
		CommissionPerShare: cfg.CommissionPerShare,
	}
	a.executor = execution.NewBacktest(a.portfolio.(*portfolio.Backtest), fill, a.btClock)
	if a.risk == nil {
		a.risk = risk.NewBasic()
	}
	return nil
}

func (a *App) resolveLive() error {
	cfg := a.liveCfg
	if cfg.Broker == nil {
		return fmt.Errorf("app: live: broker client is required")
	}
	if a.market == nil {
		return fmt.Errorf("app: live: market client is required (WithMarketClient)")
	}
	if a.universe == nil {
		return fmt.Errorf("app: live: universe client is required (WithUniverseClient)")
	}
	if a.config == nil {
		return fmt.Errorf("app: live: config client is required (WithConfigClient)")
	}

	a.clock = clock.NewReal()
	a.portfolio = portfolio.NewLive()
	a.executor = execution.NewLive(cfg.Broker)
	if a.risk == nil {
		a.risk = risk.NewBasic()
	}
	a.liveBroker = cfg.Broker
	return nil
}

func (a *App) runBacktest() error {
	ctx, err := tfcontext.New(tfcontext.Options{
		Clock:        a.clock,
		Portfolio:    a.portfolio,
		Market:       a.market,
		Universe:     a.universe,
		Config:       a.config,
		Logger:       a.logger,
		IsTradingDay: a.tradingDay,
		Mode:         job.ModeBacktest,
		Broker:       a.executor,
	})
	if err != nil {
		return fmt.Errorf("app: backtest context: %w", err)
	}

	engine, err := backtest.New(backtest.Config{
		Start:       a.backtestCfg.Start,
		End:         a.backtestCfg.End,
		Market:      a.market,
		Logger:      a.logger,
		Portfolio:   a.portfolio.(*portfolio.Backtest),
		Executor:    a.executor,
		Risk:        a.risk,
		Clock:       a.btClock,
		IsTradingDay: a.tradingDay,
	})
	if err != nil {
		return fmt.Errorf("app: backtest engine: %w", err)
	}
	a.btEngine = engine

	for _, j := range a.jobs {
		if err := engine.RegisterJob(j); err != nil {
			return fmt.Errorf("app: register job %q: %w", j.Name, err)
		}
	}

	a.positionClockForWarmup()
	if err := a.runWarmup(ctx); err != nil {
		return fmt.Errorf("app: warmup: %w", err)
	}

	a.logger.Info("backtest start",
		"start", a.backtestCfg.Start.Format("2006-01-02"),
		"end", a.backtestCfg.End.Format("2006-01-02"),
	)
	return engine.Run(context.Background(), ctx)
}

// positionClockForWarmup moves the backtest clock to the first slot of
// the first trading day.
//
// The bar cache is keyed by the session the bars were fetched for, so
// the warmup has to read the same session the first job will. At
// midnight the anchor would still be the previous session - the
// current one has not closed yet - and the warmed entry would be
// discarded.
func (a *App) positionClockForWarmup() {
	if a.btClock == nil || len(a.jobs) == 0 {
		return
	}

	day := clock.NYSETime(a.backtestCfg.Start)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, clock.NYSE)
	for i := 0; i < 14 && !a.tradingDay(day); i++ {
		day = day.AddDate(0, 0, 1)
	}

	earliest := time.Time{}
	for _, j := range a.jobs {
		at := j.Time.Resolve(day)
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
	}
	if !earliest.IsZero() {
		a.btClock.Set(earliest)
	}
}

// runWarmup prefetches the history each warmup entry asks for and primes
// the Context's bar cache with it, so the first Job of the first day does
// not pay for a cold fetch.
//
// A symbol with a shorter history than was asked for is a warning, not
// a failure: the backtest is still worth running, and the strategy will
// see the same shortfall when it asks for the history itself.
func (a *App) runWarmup(ctx job.Context) error {
	a.mu.Lock()
	entries := make([]warmupEntry, len(a.warmup))
	copy(entries, a.warmup)
	a.mu.Unlock()

	for _, w := range entries {
		bars, err := ctx.Bars(w.Symbol, w.Lookback)
		if err != nil {
			if errors.Is(err, job.ErrInsufficientBars) {
				a.logger.Warn("warmup has a short history",
					"symbol", w.Symbol,
					"lookback", w.Lookback,
					"err", err.Error(),
				)
				continue
			}
			return fmt.Errorf("warmup %s: %w", w.Symbol, err)
		}
		a.logger.Info("warmup complete",
			"symbol", w.Symbol,
			"lookback", w.Lookback,
			"bars", len(bars),
		)
	}
	return nil
}

func (a *App) runLive() error {
	ctx, err := tfcontext.New(tfcontext.Options{
		Clock:        a.clock,
		Portfolio:    a.portfolio,
		Market:       a.market,
		Universe:     a.universe,
		Config:       a.config,
		Logger:       a.logger,
		IsTradingDay: a.tradingDay,
		Mode:         job.ModeLive,
		Broker:       a.executor,
	})
	if err != nil {
		return fmt.Errorf("app: live context: %w", err)
	}

	engine, err := live.New(live.Config{
		Broker:       a.liveBroker,
		Logger:       a.logger,
		Risk:         a.risk,
		Portfolio:    a.portfolio.(*portfolio.Live),
		Clock:        a.clock,
		IsTradingDay: a.tradingDay,
	})
	if err != nil {
		return fmt.Errorf("app: live engine: %w", err)
	}
	a.liveEngine = engine

	for _, j := range a.jobs {
		if err := engine.RegisterJob(j); err != nil {
			return fmt.Errorf("app: register job %q: %w", j.Name, err)
		}
	}

	a.logger.Info("live start")
	return engine.Run(context.Background(), ctx)
}
