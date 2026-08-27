# trading-framework

A small Go framework for writing strategies that run identically in backtest and live modes.

The same `Job` handler produces orders in both modes. The framework wires mode-specific implementations of `Clock`, `Scheduler`, `Executor`, and `Portfolio` underneath the strategy; the strategy never branches on mode.

## Install

```sh
go get github.com/marcuspeh/trading-framework
```

Requires Go 1.23+.

## Quick start

```go
func rebalance(ctx job.Context, date time.Time) ([]order.Order, error) {
    if !ctx.IsTradingDay(date) {
        return nil, nil
    }
    symbols, err := ctx.Constituents("SPY")
    if err != nil {
        return nil, err
    }
    bars, err := ctx.Bars(symbols[0], 5)
    if err != nil {
        return nil, err
    }
    pos := ctx.Position(symbols[0])
    if pos.Quantity > 0 {
        return nil, nil
    }
    limit := bars[len(bars)-1].Close
    return []order.Order{{
        Symbol:   symbols[0],
        Side:     order.Buy,
        Quantity: 10,
        Type:     order.Limit,
        Limit:    &limit,
    }}, nil
}

func main() {
    a := app.NewApp(
        app.WithBacktest(app.BacktestConfig{
            Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
            End:          time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
            StartingCash: 10_000,
            BarSource:    barSource,
        }),
        app.WithMarketClient(marketClient),
        app.WithUniverseClient(universeClient),
        app.WithConfigClient(configClient),
    )
    a.RegisterJob(job.Job{
        Name:    "rebalance",
        Time:    job.Time(16, 0),
        Handler: rebalance,
    })
    if err := a.Run(); err != nil {
        log.Fatal(err)
    }
}
```

A full runnable example lives in [`examples/simple-strategy`](examples/simple-strategy).

## Concepts

### Job

The only abstraction a strategy writes. A `Job` has a name, a `TimeOfDay`, and a handler:

```go
type JobHandler func(ctx Context, date time.Time) ([]Order, error)
```

There are no `OnDay`, `OnConstituentChange`, or lifecycle callbacks. Every strategy action is a Job.

### Context

`job.Context` is what the handler receives. It is the same shape in backtest and live:

- `Now() time.Time`, `Today() time.Time` — driven by the injected `Clock`
- `Bars(symbol string, lookback int) ([]Bar, error)`
- `Constituents(etf string) ([]string, error)`
- `Position(symbol) Position`, `Positions() []Position`
- `Cash() float64`, `PortfolioValue() float64`
- `Mode() string` ("backtest" or "live")
- `IsTradingDay(date) bool`
- `Logger() Logger`, `Config() Config`

### Mode-specific implementations

| Component  | Backtest                        | Live                              |
|------------|---------------------------------|-----------------------------------|
| Clock      | `clock.BacktestClock`           | `clock.RealClock`                 |
| Scheduler  | `scheduler.Backtest` (no sleep) | `scheduler.Real` (waits wall clk) |
| Executor   | `execution.Backtest` (sim fills)| `execution.Live` (IBKR client)    |
| Portfolio  | `portfolio.Backtest`            | `portfolio.Live` (mirrors broker) |

The same `Job` handler runs against any of these. No scattered `if mode == "backtest"` branching in user code.

### Live wiring

Live needs an IBKR-style broker client:

```go
broker := clients.NewBrokerClient(ctx, clients.BrokerConfig{
    BaseURL: "http://localhost",
    Port:    5000,
})
a := app.NewApp(
    app.WithLive(app.LiveConfig{Broker: broker}),
    app.WithMarketClient(clients.NewMarket(broker)),
    app.WithUniverseClient(clients.NewConstituents(clients.NewETFClient(""))),
    app.WithConfigClient(myConfigClient),
)
```

`LiveConfig` defaults `Portfolio` to `portfolio.NewLive()` and runs `execution.Reconciler` on startup so the local mirror matches the broker before any Job fires.

## Package layout

```
trading-framework/
├── app/         # NewApp, options, mode stacks
├── backtest/    # backtest engine loop
├── clients/     # broker + ETF HTTP clients and adapters
├── clock/       # BacktestClock, RealClock
├── context/     # Context built per-call
├── examples/    # simple-strategy
├── execution/   # Executor interface + Backtest/Live/Reconciler
├── job/         # Job, JobHandler, Context, TimeOfDay
├── live/        # live engine loop
├── logging/     # text logger
├── market/      # market.Client interface
├── order/       # Order, Bar, Position, OrderResult
├── pipeline/    # log -> risk -> executor
├── portfolio/   # Portfolio interface + Backtest/Live
├── risk/        # risk.Manager (Basic implementation)
├── scheduler/   # Scheduler interface + Backtest/Real
└── universe/    # universe.Client interface
```

## Testing

```sh
go test ./...
```

The framework is designed so that tests don't require external services. The example uses `clients.StaticBars` and `clients.StaticConstituents` for fakes.

## Contributing

Issues and PRs welcome. Keep changes focused; the framework favors small interfaces over clever abstractions.