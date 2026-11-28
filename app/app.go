package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/backtest"
	"github.com/marcuspeh/trading-framework/broker/mock"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	tfcontext "github.com/marcuspeh/trading-framework/context"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/portfolio"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/tearsheet"
)

// resolve wires up the convenience stacks (backtest/live) and validates
// required dependencies.
func (a *App) resolve() error {
	switch {
	case a.backtestCfg != nil:
	case a.liveCfg != nil:
	default:
		return fmt.Errorf("app: no mode configured (use WithBacktest or WithLive)")
	}

	if err := a.applyCalendar(); err != nil {
		return err
	}

	// A persisted run has to belong to a strategy: the store holds the
	// history of one strategy's runs, and without an id there is no
	// telling whose history it is.
	if a.tearsheetStore != nil && a.strategyID == "" {
		return errors.New("app: a tear sheet store needs a strategy id (use WithStrategyID)")
	}

	if a.backtestCfg != nil {
		return a.resolveBacktest()
	}
	return a.resolveLive()
}

// applyCalendar settles which exchange calendar the framework trades on.
//
// The calendar provider is the exchange's schedule. A strategy's
// WithTradingDayFunc and WithMarketCloseFunc win over it: they replace
// the provider's answers rather than adding to them.
func (a *App) applyCalendar() error {
	if a.calendar == nil {
		return errors.New("app: a calendar provider is required (WithCalendarProvider)")
	}
	// A strategy that trades on days the exchange was closed still needs
	// a session to resolve its slots against. The standard session stands
	// in for the days the provider has no answer for.
	if a.tradingDayOverride != nil {
		a.calendar = calendar.WithSessionFallback(a.calendar)
	}
	if a.marketCloseOverride != nil {
		a.calendar = calendar.WithCloseFunc(a.calendar, a.marketCloseOverride)
	}
	if a.tradingDayOverride != nil {
		a.calendar = calendar.WithTradingDayFunc(a.calendar, a.tradingDayOverride)
	}
	a.tradingDay = calendar.NewTradingDayFunc(a.calendar)
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
	if a.market == nil {
		return fmt.Errorf("app: backtest: market client is required (WithMarketClient)")
	}
	if a.universe == nil {
		return fmt.Errorf("app: backtest: universe client is required (WithUniverseClient)")
	}
	if a.config == nil {
		return fmt.Errorf("app: backtest: config client is required (WithConfigClient)")
	}

	// The fill model reads prices from the same market client the
	// strategy reads them from, so the backtest can never simulate
	// against prices the strategy never saw.
	bars := cfg.BarSource
	if bars == nil {
		provider, ok := a.market.(market.BarProvider)
		if !ok {
			return fmt.Errorf(
				"app: backtest: the market client must implement market.BarProvider " +
					"to be used as the bar source, or supply BacktestConfig.BarSource")
		}
		bars = provider
	}

	a.btClock = clock.NewBacktest(clock.StartOfDay(cfg.Start))
	a.clock = a.btClock
	a.portfolio = portfolio.NewBacktest(cfg.StartingCash)

	// One session, shared by the context, the engine and the mock
	// broker, so they cannot disagree about which session a timestamp
	// belongs to.
	a.session = clock.NewSession(a.btClock, a.calendar)

	// The tear sheet collects the run's performance. A backtest is one
	// run, so it is one file.
	a.tearsheet = tearsheet.NewWriter(a.tearsheetDir, func(time.Time) string {
		return fmt.Sprintf("%sbacktest-%s_to-%s.html", a.filePrefix(),
			cfg.Start.Format("2006-01-02"), cfg.End.Format("2006-01-02"))
	}, a.logger)
	a.tearsheet.Meta(tearsheet.Meta{
		Mode:     "backtest",
		Start:    cfg.Start,
		End:      cfg.End,
		Starting: cfg.StartingCash,
		Currency: a.tearsheetCurrency,
	})
	a.tearsheet.WithStore(a.tearsheetStore, a.runKey("backtest", cfg.Start, cfg.End), tearsheet.Run{
		Strategy: a.strategyID,
		Mode:     "backtest",
		Start:    cfg.Start,
		End:      cfg.End,
		Starting: cfg.StartingCash,
		Currency: a.tearsheetCurrency,
	})

	mockBroker := mock.New(mock.Config{
		Portfolio:          a.portfolio.(*portfolio.Backtest),
		Bars:               bars,
		Session:            a.session,
		SlippageBps:        cfg.SlippageBps,
		Commission:         cfg.Commission,
		CommissionPerShare: cfg.CommissionPerShare,
		Logger:             a.logger,
		Fills:              a.tearsheet,
	})
	a.btBroker = mockBroker
	a.executor = execution.NewBroker(mockBroker)
	if a.risk == nil {
		a.risk = risk.NewBasic()
	}
	return nil
}

// filePrefix names the strategy in the tear sheet's file name.
//
// It is empty when the strategy has no id, so a strategy that does not
// name itself keeps the plain names.
func (a *App) filePrefix() string {
	if a.strategyID == "" {
		return ""
	}
	return a.strategyID + "-"
}

// runKey says which run of which strategy a tear sheet belongs to.
//
// A backtest is a run over a window, so the window is part of the key
// and the same window can be run again without disturbing the last
// one. A live run is one run of the strategy, so its key is the same
// every time the service starts: that is what lets it find its history
// back.
func (a *App) runKey(mode string, start, end time.Time) string {
	key := a.strategyID + "/" + mode
	if mode == "backtest" {
		key += "/" + start.Format("2006-01-02") + "_" + end.Format("2006-01-02")
	}
	return key
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
	a.session = clock.NewSession(a.clock, a.calendar)
	a.portfolio = portfolio.NewLive()

	// The tear sheet collects the run's performance. A live run lasts
	// longer than a day, so it writes one file per trading day.
	a.tearsheet = tearsheet.NewWriter(a.tearsheetDir, func(day time.Time) string {
		return fmt.Sprintf("%slive-%s.html", a.filePrefix(), day.Format("2006-01-02"))
	}, a.logger)
	a.tearsheet.Meta(tearsheet.Meta{
		Mode:     "live",
		Currency: a.tearsheetCurrency,
	})
	// A live run is one run of the strategy, however many times the
	// service restarts: the key says so, and the store keeps the
	// history across the restarts.
	a.tearsheet.WithStore(a.tearsheetStore, a.runKey("live", time.Time{}, time.Time{}), tearsheet.Run{
		Strategy: a.strategyID,
		Mode:     "live",
		Currency: a.tearsheetCurrency,
	})

	// The history of a live run is the strategy's, and the strategy's
	// own: the service loads it back when it starts again. A backtest
	// does not, because a run over a window is self-contained - and
	// because a run of the same window again would otherwise carry
	// the last one's points with it.
	if err := a.tearsheet.Load(context.Background()); err != nil {
		return fmt.Errorf("app: tearsheet history: %w", err)
	}

	// The registry remembers the orders the executor places, so the
	// fills the broker reports later can be named.
	a.fills = fills.NewRegistry().WithSink(a.tearsheet)
	a.executor = execution.NewBroker(cfg.Broker).WithFills(a.fills)
	if a.risk == nil {
		a.risk = risk.NewBasic()
	}
	a.liveBroker = cfg.Broker
	return nil
}

func (a *App) runBacktest(parent context.Context) error {
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
		Session:      a.session,
	})
	if err != nil {
		return fmt.Errorf("app: backtest context: %w", err)
	}

	engine, err := backtest.New(backtest.Config{
		Start:        a.backtestCfg.Start,
		End:          a.backtestCfg.End,
		Market:       a.market,
		Logger:       a.logger,
		Portfolio:    a.portfolio.(*portfolio.Backtest),
		Executor:     a.executor,
		Risk:         a.risk,
		Clock:        a.btClock,
		IsTradingDay: a.tradingDay,
		Session:      a.session,
		Calendar:     a.calendar,
		Broker:       a.btBroker,
		Observer:     a.tearsheet,
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
	return engine.Run(parent, ctx)
}

// positionClockForWarmup moves the backtest clock to the earliest slot
// of the first trading day.
//
// The bar cache is keyed by the session the bars were fetched for, so
// the warmup has to read the same session the first job will. At
// midnight the anchor would still be the previous session - the
// current one has not closed yet - and the warmed entry would be
// discarded.
//
// The earliest slot is used because the warmup exists for the first
// job: it primes the session that job reads. Later jobs read other
// sessions and fetch them for themselves.
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
		at := j.Time.Resolve(day, a.session)
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

	// The entries are fetched in batches of one lookback, so the
	// framework can read them concurrently.
	byLookback := map[int][]string{}
	for _, w := range entries {
		byLookback[w.Lookback] = append(byLookback[w.Lookback], w.Symbol)
	}

	for lookback, symbols := range byLookback {
		bars, err := ctx.Bars(symbols, lookback)
		if err != nil {
			return fmt.Errorf("warmup: %w", err)
		}
		for _, symbol := range symbols {
			got, ok := bars[symbol]
			if !ok {
				// A symbol with a short history, or none at all, is
				// worth warning about but not worth abandoning the
				// backtest over: the strategy will hit the same
				// shortfall when it asks for the history itself.
				a.logger.Warn("warmup has a short history",
					"symbol", symbol,
					"lookback", lookback,
				)
				continue
			}
			a.logger.Info("warmup complete",
				"symbol", symbol,
				"lookback", lookback,
				"bars", len(got),
			)
		}
	}
	return nil
}

func (a *App) runLive(parent context.Context) error {
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
		Session:      a.session,
	})
	if err != nil {
		return fmt.Errorf("app: live context: %w", err)
	}

	engine, err := live.New(live.Config{
		Broker:       a.liveBroker,
		Executor:     a.executor,
		Logger:       a.logger,
		Risk:         a.risk,
		Portfolio:    a.portfolio.(*portfolio.Live),
		Clock:        a.clock,
		Session:      a.session,
		Market:       a.market,
		IsTradingDay: a.tradingDay,
		Fills:        a.fills,
		FillWebhook:  a.liveCfg.FillWebhook,
		Observer:     a.tearsheet,
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
	return engine.Run(parent, ctx)
}
