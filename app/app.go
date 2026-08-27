package app

import (
	"context"
	"fmt"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/clock"
	tfcontext "github.com/marcuspeh/trading-framework/context"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
)

// resolve wires up the convenience stacks (backtest/live) and validates
// required dependencies for hand-wired configurations.
func (a *App) resolve() error {
	if a.backtestCfg != nil {
		return a.resolveBacktest()
	}
	if a.liveCfg != nil {
		return a.resolveLive()
	}
	if a.scheduler == nil {
		return fmt.Errorf("app: no scheduler configured (use WithScheduler or WithBacktest/WithLive)")
	}
	if a.executor == nil {
		return fmt.Errorf("app: no executor configured (use WithExecutor or WithBacktest/WithLive)")
	}
	if a.portfolio == nil {
		return fmt.Errorf("app: no portfolio configured (use WithPortfolio or WithBacktest/WithLive)")
	}
	if a.market == nil {
		return fmt.Errorf("app: no market client configured (WithMarketClient)")
	}
	if a.universe == nil {
		return fmt.Errorf("app: no universe client configured (WithUniverseClient)")
	}
	if a.config == nil {
		return fmt.Errorf("app: no config client configured (WithConfigClient)")
	}
	if a.clock == nil {
		return fmt.Errorf("app: no clock configured (WithClock or WithBacktest/WithLive)")
	}
	if a.risk == nil {
		a.risk = risk.NewBasic()
	}
	if len(a.jobs) == 0 {
		return fmt.Errorf("app: no jobs registered")
	}
	return nil
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

	a.clock = clock.NewBacktest(clock.StartOfDay(cfg.Start))
	a.portfolio = portfolio.NewBacktest(cfg.StartingCash)
	a.executor = execution.NewBacktest(a.portfolio.(*portfolio.Backtest),
		execution.NewDefaultFill(cfg.BarSource), a.clock)
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
	})
	if err != nil {
		return fmt.Errorf("app: backtest context: %w", err)
	}

	engine, err := backtest.New(backtest.Config{
		Start:     a.backtestCfg.Start,
		End:       a.backtestCfg.End,
		Market:    a.market,
		Logger:    a.logger,
		Portfolio: a.portfolio.(*portfolio.Backtest),
		Executor:  a.executor,
		Risk:      a.risk,
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

	a.logger.Info("backtest start",
		"start", a.backtestCfg.Start.Format("2006-01-02"),
		"end", a.backtestCfg.End.Format("2006-01-02"),
	)
	return engine.Run(context.Background(), ctx)
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
	})
	if err != nil {
		return fmt.Errorf("app: live context: %w", err)
	}

	engine, err := live.New(live.Config{
		Broker:    a.liveBroker,
		Logger:    a.logger,
		Risk:      a.risk,
		Portfolio: a.portfolio.(*portfolio.Live),
		Clock:     a.clock,
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
