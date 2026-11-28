# trading-framework

A small Go framework for writing strategies that run identically in backtest
and live modes.

The same `Job` handler produces orders in both modes. The framework wires
mode-specific implementations of `Clock`, `Scheduler`, `Executor` and
`Portfolio` underneath the strategy; the strategy never branches on mode.

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
    bars, err := ctx.Bars(symbols[:1], 5)
    if err != nil {
        return nil, err
    }
    series := bars[symbols[0]]
    pos := ctx.Position(symbols[0])
    if pos.Quantity > 0 {
        return nil, nil
    }
    limit := series[len(series)-1].Close
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
        app.WithCalendarProvider(calendar.NewService(marketdata.NewClient("http://localhost:8001"))),
        app.WithBacktest(app.BacktestConfig{
            Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
            End:          time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
            StartingCash: 10_000,
        }),
        app.WithMarketClient(marketClient),
        app.WithUniverseClient(universeClient),
        app.WithConfigClient(configClient),
    )
    a.RegisterJob(job.Job{
        Name:    "rebalance",
        Time:    job.AtClose(),
        Handler: rebalance,
    })
    if err := a.Run(); err != nil {
        log.Fatal(err)
    }
}
```

A full runnable example lives in [`examples/simple-strategy`](examples/simple-strategy). It trades the SPY's
constituents with a moving-average crossover and reads its prices from the market-data-service, so the
service has to be running:

```sh
go run ./examples/simple-strategy
```

## Concepts

### Job

The only abstraction a strategy writes. A `Job` has a name, a `Slot` and a
handler:

```go
type Handler func(ctx Context, date time.Time) ([]order.Order, error)
```

`job.JobHandler` is an alias for `job.Handler`, if you prefer the longer name.

There are no `OnDay`, `OnConstituentChange`, or lifecycle callbacks. Every
strategy action is a Job.

### Slots

A Job fires every day at its slot. A slot is either a fixed wall-clock time
or an offset from a session boundary:

```go
job.Time(7, 30)             // 07:30, whatever the market is doing
job.Time(10, 30)            // 10:30, during the session
job.AtOpen()                 // 09:30
job.BeforeOpen(2 * time.Hour) // 07:30
job.AtClose()                // 16:00, or 13:00 on an early-close day
job.BeforeClose(time.Hour)   // 15:00, or 12:00 on an early-close day
```

The close-relative forms exist because the session end is not a constant. The
exchange closes at 13:00 on the day after Thanksgiving and on the other
early-close days, and a strategy that wants to act at the close should not
have to know which days those are.

A slot is resolved against the session of the day it fires on, so a
close-relative slot moves with the exchange.

### Context

`job.Context` is what the handler receives. It is the same shape in backtest
and live:

- `Now() time.Time`, `Today() time.Time` — driven by the injected `Clock`, on
  the NYSE (US/Eastern) calendar
- `Mode() Mode` — `job.ModeBacktest` or `job.ModeLive`
- `Bars(symbols []string, lookback int) (map[string][]order.Bar, error)`
- `Constituents(etf string) ([]string, error)`
- `Position(symbol) order.Position`, `Positions() []order.Position`
- `Cash() float64`, `PortfolioValue() float64`
- `OpenOrders() ([]order.Order, error)`, `CancelOrder(id string) error`,
  `UpdateOrder(id string, limit float64) error`
- `IsTradingDay(date) bool`
- `Logger() Logger`, `Config() Config`

`order.Bar` is an alias of the bar type from
[indicators-go](https://github.com/marcuspeh/indicators-go), so it carries
a `Symbol`, a `Timestamp`, OHLC prices and a `Volume`, and can be passed
to the indicator library as-is.

### The session anchor

Every date the framework handles is anchored on the NYSE calendar. The
**session anchor** is the most recent session whose close has passed, and
everything that touches market data resolves its date through it:

| A job fires | It reads | A market order fills at |
|---|---|---|
| before the close | the previous session | that session's close |
| at or after the close | the current session | the current session's close |

So a job at 09:00 on Tuesday reads Monday's close, and a job at 16:00 on
Tuesday reads Tuesday's. No mode can see a bar that had not been printed
yet.

The close is per date: 16:00 ET normally, 13:00 ET on an early-close day.

### The calendar

The `calendar` package owns the exchange calendar: which days the exchange
traded, which of them closed early, and when each session opened and closed.

The framework has no copy of the NYSE rules. The exchange publishes its
schedule and the
[market-data-service](https://github.com/marcuspeh/market-data-service)
serves it, so the calendar is a provider the strategy supplies:

```go
a := app.NewApp(
    app.WithCalendarProvider(calendar.NewService(marketdata.NewClient("http://localhost:8001"))),
    // ...
)
```

The provider asks the service about a date once and caches the answer. A
service failure fails the run: the framework will not trade on a schedule it
could not confirm.

`app.WithMarketCloseFunc` replaces the session close for a house view that
differs from the published one - a venue that closes at a different hour, or
a data vendor whose early closes are not the exchange's. Every close-relative
slot moves with it:

```go
a := app.NewApp(
    app.WithMarketCloseFunc(func(date time.Time) time.Time {
        return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, calendar.NYSE)
    }),
    // ...
)
```

`app.WithTradingDayFunc` replaces the trading days for a strategy that trades on
a different exchange's days, or on a schedule of its own:

```go
a := app.NewApp(
    app.WithTradingDayFunc(func(date time.Time) bool {
        return date.Weekday() != time.Saturday && date.Weekday() != time.Sunday
    }),
    // ...
)
```

On the days it names that the exchange was closed, the standard session - 09:30
to 16:00 ET - stands in, so a close-relative slot still resolves.

For a backtest that needs a calendar without a running market-data-service,
`calendar/published` serves the published NYSE holiday and early-close
calendars:

```go
a := app.NewApp(
    app.WithCalendarProvider(published.New()),
    // ...
)
```

It covers 2023 through 2029 and refuses the years it has no data for, so a
test that wanders outside that range fails loudly rather than being answered
by a guess. A live strategy should use `calendar.NewService` instead, so it
trades on the exchange's current schedule.

Jobs do not fire on a day the exchange is closed, in either mode.

### Mode-specific implementations

| Component  | Backtest                          | Live                              |
|------------|-----------------------------------|-----------------------------------|
| Clock      | `clock.BacktestClock`             | `clock.RealClock`                 |
| Scheduler  | `scheduler.Backtest` (no sleep)   | `scheduler.Real` (waits wall clk) |
| Executor   | `execution.Broker`                | `execution.Broker`                |
| Broker     | `broker/mock` (in memory)         | ibkr-proxy SDK client             |
| Portfolio  | `portfolio.Backtest`              | `portfolio.Live` (mirrors broker) |

The executor is the same in both modes. What differs is the broker behind it:
an in-memory mock for a backtest, the ibkr-proxy service for a live run.
That is what makes "the same handler produces the same orders" true rather
than merely intended.

### Market data

Bars and constituents come from the
[market-data-service SDK](https://github.com/marcuspeh/market-data-service),
and the backtest's fill model reads its prices from the same adapter - so
the strategy and the fill model can never see different ones.

`ctx.Bars(symbols, n)` returns the bars of each symbol as a map. A symbol
with fewer than `n` bars is left out of the map, rather than returned as a
short history an indicator would silently be computed over:

```go
bars, err := ctx.Bars([]string{"AAPL", "MSFT"}, 20)
if err != nil {
    return nil, err
}
series, ok := bars["AAPL"]
if !ok {
    // Not enough history yet.
}
```

The symbols are fetched in one batch request. The service answers a batch by
fetching its symbols a few at a time, so a strategy that reads the history
of a whole universe pays for one round trip rather than one request per
name.

The adapter keeps what it has fetched and asks the service only for what is
missing, so a backtest costs one bar per symbol per day rather than a
window.

A market client implements `market.Client`, which serves one symbol at a
time and a set of them at once:

```go
type Client interface {
    Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error)
    BarsBatch(ctx context.Context, symbols []string, lookback int, asOf time.Time) (map[string][]order.Bar, error)
}
```

`clients.NewMarket` implements it over the SDK; `clients.StaticBars` serves
bars from memory for tests.

### Broker

The broker wire format is owned by the
[ibkr-proxy SDK](https://github.com/marcuspeh/ibkr-proxy); the framework
aliases its types rather than restating them, so the two cannot drift.

A backtest runs against `broker/mock`, an in-memory broker that fills
orders against historical bars. A live run talks to the ibkr-proxy service:

```go
brokerClient := clients.NewBroker(ctx, clients.BrokerConfig{
    BaseURL: "http://localhost",
    Port:    5001,
})
calendarClient := calendar.NewService(marketdata.NewClient("http://localhost:8001"))
marketClient := clients.NewMarket(marketdata.NewClient("http://localhost:8001"), calendarClient)
universeClient := clients.NewConstituents(marketdata.NewClient("http://localhost:8001"))

a := app.NewApp(
    app.WithLive(app.LiveConfig{Broker: brokerClient}),
    app.WithCalendarProvider(calendarClient),
    app.WithMarketClient(marketClient),
    app.WithUniverseClient(universeClient),
    app.WithConfigClient(myConfigClient),
)
```

Every call to the broker carries a deadline. The SDK sets no timeout on its
HTTP client, so without one a stalled broker would hang the session.

A live run checks the broker is connected before it starts, and refreshes
the portfolio from the broker before every job - so a job that asks what it
already holds sees what the broker says it holds.

#### Fill reports

A live broker fills asynchronously: the order is acknowledged when it is
submitted, and the fill arrives later, on a channel of its own. The
ibkr-proxy pushes those fills to a webhook. The framework can listen for
them and log each fill as it happens:

```go
a := app.NewApp(
    app.WithLive(app.LiveConfig{
        Broker:       brokerClient,
        FillWebhook: "127.0.0.1:8000",
    }),
    ...
)
```

The proxy's `WEBHOOK_URL` has to point at it: the proxy posts to
`{WEBHOOK_URL}/fills`, and its default webhook URL is
`http://127.0.0.1:8000/webhook`. A fill is logged against the order that
filled when the framework placed it; a fill for an order it did not place -
a manual trade, or another client's - is logged as a warning.

### Orders

An order may carry protective legs and a time in force:

```go
order.Order{
    Symbol:   "AAPL",
    Side:     order.Buy,
    Quantity: 10,
    Type:     order.Limit,
    Limit:    &entry,
    TakeProfit: &target,
    StopLoss:   &stop,
    TIF:      "GTC",   // the default
}
```

An order with both legs and a limit price is sent as a bracket. An order
carrying legs but no limit price is rejected rather than silently sent as a
plain order.

The default time in force is `GTC`, so a resting limit order survives the
session - the same in backtest and live.

A strategy that gives its orders ids gets idempotency: the same order
submitted twice carries the same key, so the broker returns the first
response rather than placing it twice. The order type is part of the key,
because the broker's idempotency cache is shared across its market, limit
and bracket endpoints - a market order and a limit order sharing an id
must not collide there. The strategy's order id travels to the broker as
the order reference.

### How a backtest fills

The backtest fills against daily bars, which is worth knowing before
reading a result:

- A **market order** fills at the close of the session the strategy is
  anchored to. A job that fires before the close is anchored to the
  previous session, so its market order fills at that session's close -
  the price the strategy read. In live the same order fills at the live
  market price, so an overnight gap is captured in live but not in a
  backtest.
- A **limit order** rests and is first evaluated against the session
  that closes after it was submitted, never against a session that
  closed before it existed. It fills at its limit, or better if the
  session opened through it.
- A **bracket's take-profit** fires when the market reaches the target,
  and its **stop-loss** when the market reaches the stop - a stop fills
  at the stop or worse, so a session that opened through it fills at the
  open.
- A **day order** expires at the close of the session it was submitted
  for, whether or not the market has a bar for it.

### Transaction costs

Backtest fills can model slippage and commission, so results are not
unrealistically clean:

```go
app.WithBacktest(app.BacktestConfig{
    // ...
    SlippageBps:        5,   // 0.05% worse than the reference price
    Commission:         1,   // flat fee per fill
    CommissionPerShare: 0.01,
})
```

Limit orders that do not trigger rest until they fill or are cancelled.

### Warmup

`AddWarmup` prefetches history before the first Job runs, so the opening bar
of the backtest is not spent on a cold fetch:

```go
a.AddWarmup("AAPL", 50)
```

The warmup primes the same cache entry the first job reads, so it is not
thrown away.

### Configuration

`job.Config` is a key/value store the strategy reads through `ctx.Config()`.
The `config` package provides a file-backed implementation:

```go
cfg, err := config.NewFile("strategy.conf")
```

The file format is `key = value`, one per line; `#` starts a comment. Values
may be quoted.

### Indicators

Technical-analysis indicators live in their own library, [indicators-go](https://github.com/marcuspeh/indicators-go), which this framework depends on: SMA, EMA, RSI, MACD, Bollinger Bands, ATR, OBV, ROC and MinMax. They are validated against the reference TA-Lib implementation.

`order.Bar` is an alias of that library's `Bar`, so bars go straight into an indicator with no conversion:

```go
import indicators "github.com/marcuspeh/indicators-go"

sma := indicators.NewSMA(20)
bars, _ := ctx.Bars([]string{"AAPL"}, 50)
series := bars["AAPL"]
for i := range series {
    sma.Update(&series[i])
}
value := sma.Value()
```

### Tear sheets

Every run leaves a performance page behind. A backtest writes one file for
the run; a live run writes one file per trading day.

```text
tearsheets/backtest-2025-01-01_to-2025-02-01.html
tearsheets/live-2026-11-26.html
```

The page is a single self-contained HTML file. It fetches nothing, so it
opens from disk and travels well. It carries the run's headline numbers
(total return, CAGR, Sharpe, Sortino, max drawdown, volatility, win rate,
profit factor, exposure and fees), the portfolio's value at each close,
the drawdown from its running peak, the return of each month, and every
closed trade with its profit or loss.

No strategy code is involved: the engines report the portfolio's value
and the brokers report their fills, and the framework does the rest. Three
options move it:

```go
app.WithTearsheetDir("reports")    // where the files go (default "tearsheets")
app.WithTearsheetCurrency("EUR")   // what the amounts are in (default "USD")
app.WithStrategyID("momentum")     // what the strategy calls itself
```

A strategy that names itself gets its files named after it, so two
strategies can share a directory:

```text
tearsheets/momentum-backtest-2025-01-01_to-2025-02-01.html
tearsheets/momentum-live-2026-11-26.html
```

A live run's daily file holds the run from its start up to that day. The
file for a day is written when the next day starts, and the file for the
last day is written when the run stops.

Two things about a live tear sheet are worth knowing. Its value curve is
sampled when the framework reconciles with the broker - before each job
and once more when the run stops - so it holds the broker's own net asset
value at those moments rather than a continuous mark. And its currency is
the one the strategy says: the framework does not read it from the broker.

### Keeping the history

A tear sheet lives in the process that wrote it, so a live service that
restarts starts its report from nothing. A store keeps the run's events
instead, and the service picks them up where it left off:

```go
store, err := tearsheet.NewSQLiteStore("tearsheets.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

a := app.NewApp(
    app.WithStrategyID("momentum"),
    app.WithTearsheetStore(store),
    // ...
)
```

The store is a SQLite file: no server, and it survives the process that
wrote it. On startup the run's history is loaded back, so a service that
restarts carries on with the curve it had rather than beginning again.
The closed trades are rebuilt from the stored fills, so they are matched
the same way the run that recorded them matched them.

A store needs a strategy id. It holds the history of one strategy's runs,
and without an id there is no telling whose history it is. The id is also
what keys the stored runs: a live run is one run of the strategy, so it
keeps the same key however often the service restarts, while a backtest's
key carries its window, so the same window can be run again without
disturbing the last run.

The store is an interface, so the history can live somewhere else:

```go
type Store interface {
    Save(ctx context.Context, key string, run Run, evts Events) error
    Load(ctx context.Context, key string) (Run, Events, error)
    Runs(ctx context.Context, strategy string) ([]Run, error)
}
```

What is stored is the run's raw events - the portfolio's value at each
sample and the fills that moved it. The closed trades and the numbers on
the page are derived from them, which is why a loaded run reads the same
as the run that recorded it.

## Package layout

```
trading-framework/
├── app/         # NewApp, options, mode stacks
├── backtest/    # backtest engine loop
├── broker/      # broker.Client interface + wire types
├── broker/mock/ # in-memory broker for backtests
├── calendar/    # exchange calendar: Provider interface + service client
├── calendar/published/ # published NYSE calendars, for tests and examples
├── clients/     # broker client + market-data-service adapters
├── clock/       # BacktestClock, RealClock, Session
├── config/      # file-backed job.Config
├── context/     # Context implementation
├── examples/    # simple-strategy
├── execution/   # Executor + order translation + reconciler
├── fills/       # live fill webhook receiver
├── job/         # Job, Slot, Handler, Context
├── live/        # live engine loop
├── logging/     # text logger
├── market/      # market.Client interface
├── order/       # Order, Bar, Position, Result
├── pipeline/    # log -> risk -> executor
├── portfolio/   # Portfolio interface + Backtest/Live
├── risk/        # risk.Manager (Basic implementation)
├── scheduler/   # Scheduler interface + Backtest/Real
├── tearsheet/   # performance page: metrics + HTML renderer
└── universe/    # universe.Client interface
```

## Known limits

- The backtest fills against daily bars, so a market order placed
  before the close fills at the last completed close rather than at the
  live price. A live run captures an overnight gap; a backtest does not.
- Live positions are refreshed from the broker before each job, so a
  position that appears between two jobs is not visible until the next
  one.
- A live fill is named after the order the framework placed. A bracket's
  exit legs are reported by the reference their entry carried, so they
  are named only when the strategy gave its orders an id.
- Live cash is derived from the net asset value and the position marks,
  which are daily closes. It drifts from the broker's own cash during a
  session, and it is unknown rather than wrong when a position has no
  mark.
- The market-data service publishes a session's bar after the session
  closes, so a job at the close may find that day's bar missing and
  read the previous session's instead.
- The exchange calendar comes from the market-data-service, so the
  framework only knows the schedule the service can answer for.
  `app.WithMarketCloseFunc` replaces the session close for a house view
  that differs, and every close-relative slot moves with it.
- Intraday (minute and hour) bars are not supported. A job may run
  during the session, but it reads daily bars.

## Testing

```sh
go test ./...
```

The framework is designed so that tests don't require external services. The
example uses `clients.StaticBars` and `clients.StaticConstituents` for fakes.

The indicators-go library cross-checks its maths against Python TA-Lib in its own test suite. That test skips itself when `python3` with `pandas`, `numpy` and `ta-lib` is not installed.
