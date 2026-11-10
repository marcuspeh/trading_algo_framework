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

A full runnable example lives in [`examples/simple-strategy`](examples/simple-strategy):

```sh
go run ./examples/simple-strategy
```

## Concepts

### Job

The only abstraction a strategy writes. A `Job` has a name, a `TimeOfDay`, and a handler:

```go
type Handler func(ctx Context, date time.Time) ([]order.Order, error)
```

`job.JobHandler` is an alias for `job.Handler`, if you prefer the longer name.

There are no `OnDay`, `OnConstituentChange`, or lifecycle callbacks. Every strategy action is a Job.

A Job fires every day at its `TimeOfDay`. To run on a subset of days, check `ctx.IsTradingDay(date)` inside the handler - the default predicate is the NYSE calendar, which excludes weekends and observed market holidays.

### Context

`job.Context` is what the handler receives. It is the same shape in backtest and live:

- `Now() time.Time`, `Today() time.Time` — driven by the injected `Clock`
- `Mode() Mode` — `job.ModeBacktest` or `job.ModeLive`
- `Bars(symbol string, lookback int) ([]order.Bar, error)`
- `Constituents(etf string) ([]string, error)`
- `Position(symbol) order.Position`, `Positions() []order.Position`
- `Cash() float64`, `PortfolioValue() float64`
- `IsTradingDay(date) bool`
- `Logger() Logger`, `Config() Config`

### Mode-specific implementations

| Component  | Backtest                        | Live                              |
|------------|---------------------------------|-----------------------------------|
| Clock      | `clock.BacktestClock`           | `clock.RealClock`                 |
| Scheduler  | `scheduler.Backtest` (no sleep) | `scheduler.Real` (waits wall clk) |
| Executor   | `execution.Backtest` (sim fills)| `execution.Live` (broker client)  |
| Portfolio  | `portfolio.Backtest`            | `portfolio.Live` (mirrors broker) |

The same `Job` handler runs against any of these. No scattered `if mode == "backtest"` branching in user code.

### Live wiring

Live needs a broker client. Any implementation of `broker.Client` works - the bundled HTTP client satisfies it directly:

```go
brokerClient := clients.NewBrokerClient(ctx, clients.BrokerConfig{
    BaseURL: "http://localhost",
    Port:    5000,
})
marketClient := clients.NewMarket(marketdata.NewClient("http://localhost:8001"))
universeClient := clients.NewConstituents(clients.NewETFClient("http://localhost:8001"))

a := app.NewApp(
    app.WithLive(app.LiveConfig{Broker: brokerClient}),
    app.WithMarketClient(marketClient),
    app.WithUniverseClient(universeClient),
    app.WithConfigClient(myConfigClient),
)
```

`LiveConfig` defaults `Portfolio` to `portfolio.NewLive()` and runs `execution.Reconciler` on startup so the local mirror matches the broker before any Job fires.

### Configuration

`job.Config` is a key/value store the strategy reads through `ctx.Config()`. The `config` package provides a file-backed implementation:

```go
cfg, err := config.NewFile("strategy.conf")
```

The file format is `key = value`, one per line; `#` starts a comment. Values may be quoted.

### Indicators

Technical-analysis indicators live in their own library, [indicators-go](https://github.com/marcuspeh/indicators-go), which this framework depends on: SMA, EMA, RSI, MACD, Bollinger Bands, ATR, OBV, ROC and MinMax. They are validated against the reference TA-Lib implementation.

The framework's `indicator` package adapts the library to the framework's bar type and provides convenience helpers:

```go
sma := indicator.NewSMA(20)
bars, _ := ctx.Bars("AAPL", 50)
value := indicator.Feed(sma, "AAPL", bars)
```

You can also import the library directly:

```go
import indicators "github.com/marcuspeh/indicators-go"
```

### Transaction costs

Backtest fills can model slippage and commission, so results are not unrealistically clean:

```go
app.WithBacktest(app.BacktestConfig{
    // ...
    SlippageBps:       5,   // 0.05% worse than the reference price
    Commission:        1,   // flat fee per fill
    CommissionPerShare: 0.01,
})
```

Limit orders that do not trigger stay resting and are retried on each subsequent day until they fill or are cancelled.

### Warmup

`AddWarmup` prefetches history before the first Job runs, so the opening bar of the backtest is not spent on a cold fetch:

```go
a.AddWarmup("AAPL", 50)
```

## Package layout

```
trading-framework/
├── app/         # NewApp, options, mode stacks
├── backtest/    # backtest engine loop
├── broker/      # broker.Client interface + wire types
├── clients/     # broker + ETF HTTP clients and adapters
├── clock/       # BacktestClock, RealClock
├── config/      # file-backed job.Config
├── context/     # Context implementation
├── examples/    # simple-strategy
├── execution/   # Executor interface + Backtest/Live/Reconciler
├── indicator/   # adapter for the indicators-go library
├── job/         # Job, Handler, Context, TimeOfDay
├── live/        # live engine loop
├── logging/     # text logger
├── market/      # market.Client interface
├── order/       # Order, Bar, Position, Result
├── pipeline/    # log -> risk -> executor
├── portfolio/   # Portfolio interface + Backtest/Live + NYSE calendar
├── risk/        # risk.Manager (Basic implementation)
├── scheduler/   # Scheduler interface + Backtest/Real
└── universe/    # universe.Client interface
```

## Testing

```sh
go test ./...
```

The framework is designed so that tests don't require external services. The example uses `clients.StaticBars` and `clients.StaticConstituents` for fakes.

The indicators-go library cross-checks its maths against Python TA-Lib in its own test suite. That test skips itself when `python3` with `pandas`, `numpy` and `ta-lib` is not installed.
