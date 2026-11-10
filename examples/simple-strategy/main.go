// simple-strategy is a minimal example showing how a strategy uses
// the trading-framework package.
//
// It registers two Jobs (refresh-universe at 09:00, rebalance at 16:00)
// and runs a backtest over a 10-day window using in-memory fake
// market data and constituents.
//
// The rebalance strategy uses a 5-day simple moving average: it buys
// when the last close is above its moving average, and sells when the
// position is held and the close drops below it.
//
// Run from the repo root:
//
//	go run ./examples/simple-strategy
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/config"
	"github.com/marcuspeh/trading-framework/indicator"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

func refresher(ctx job.Context, date time.Time) ([]order.Order, error) {
	if !ctx.IsTradingDay(date) {
		return nil, nil
	}
	syms, err := ctx.Constituents("SPY")
	if err != nil {
		return nil, err
	}
	ctx.Logger().Info("universe refreshed",
		"date", date.Format("2006-01-02"),
		"count", len(syms),
	)
	return nil, nil
}

func rebalance(ctx job.Context, date time.Time) ([]order.Order, error) {
	if !ctx.IsTradingDay(date) {
		return nil, nil
	}

	syms, err := ctx.Constituents("SPY")
	if err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, nil
	}

	target := syms[0]
	bars, err := ctx.Bars(target, 5)
	if err != nil {
		return nil, err
	}
	if len(bars) < 5 {
		return nil, nil
	}

	// The indicator maths runs on the framework's bars.
	sma := indicator.NewSMA(5)
	smaValue := indicator.Feed(sma, target, bars)
	last := bars[len(bars)-1].Close

	ctx.Logger().Info("rebalance",
		"date", date.Format("2006-01-02"),
		"target", target,
		"bars", len(bars),
		"close", last,
		"sma", smaValue,
		"cash", ctx.Cash(),
		"portfolio_value", ctx.PortfolioValue(),
	)

	pos := ctx.Position(target)

	// Price above its moving average: go long.
	if last > smaValue {
		if pos.Quantity > 0 {
			return nil, nil
		}
		return []order.Order{{
			Symbol:   target,
			Side:     order.Buy,
			Quantity: 10,
			Type:     order.Market,
		}}, nil
	}

	// Price below its moving average: close the long.
	if pos.Quantity > 0 {
		return []order.Order{{
			Symbol:   target,
			Side:     order.Sell,
			Quantity: pos.Quantity,
			Type:     order.Market,
		}}, nil
	}

	return nil, nil
}

func fakeBars() map[string][]order.Bar {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	out := map[string][]order.Bar{}
	prices := map[string]float64{
		"AAPL": 180.0,
		"MSFT": 370.0,
		"NVDA": 480.0,
	}
	for sym, p := range prices {
		bars := make([]order.Bar, 0, 10)
		for d := 0; d < 10; d++ {
			date := start.AddDate(0, 0, d)
			if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
				continue
			}
			open := p
			close := p + float64(d)*0.5
			high := close + 1.0
			low := open - 0.5
			bars = append(bars, order.Bar{
				Time:   date,
				Open:   open,
				High:   high,
				Low:    low,
				Close:  close,
				Volume: 1_000_000,
			})
		}
		out[sym] = bars
	}
	return out
}

func main() {
	bars := fakeBars()
	market := clients.NewStaticBars(bars)
	barSrc := clients.NewStaticBarSource(bars)

	constituentsByDate := map[string]map[string][]string{
		"SPY": {
			"2024-01-02": {"AAPL", "MSFT", "NVDA"},
		},
	}
	constituents := clients.NewStaticConstituents(constituentsByDate)

	cfg := config.NewMap(map[string]string{
		"strategy":      "simple",
		"max_positions": "5",
	})

	a := app.NewApp(
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			End:          time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
			StartingCash: 10_000,
			BarSource:    barSrc,
			SlippageBps:  5,
			Commission:   1,
		}),
		app.WithMarketClient(market),
		app.WithUniverseClient(constituents),
		app.WithConfigClient(cfg),
		app.WithStdoutLogger(),
	)

	// Prime the price history for AAPL before the first job runs.
	if err := a.AddWarmup("AAPL", 5); err != nil {
		fmt.Fprintln(os.Stderr, "warmup:", err)
		os.Exit(1)
	}

	if err := a.RegisterJob(job.Job{
		Name:    "refresh-universe",
		Time:    job.Time(9, 0),
		Handler: refresher,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register refresh-universe:", err)
		os.Exit(1)
	}
	if err := a.RegisterJob(job.Job{
		Name:    "rebalance",
		Time:    job.Time(16, 0),
		Handler: rebalance,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register rebalance:", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}
}
