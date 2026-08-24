// simple-strategy is a minimal example showing how a strategy uses
// the trading-framework package.
//
// It registers two Jobs (refresh-universe at 09:00, rebalance at 16:00)
// and runs a backtest over a 10-day window using in-memory fake
// market data and constituents.
//
// Run from the repo root:
//
//	go run ./examples/simple-strategy
package main

import (
	"fmt"
	"os"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/internal/clients"
)

// Refresher captures the universe constituents as of the current date
// and logs them. Real strategies might refresh an internal cache here.
func Refresher(ctx tf.Context, date time.Time) ([]tf.Order, error) {
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

// Rebalance is the example strategy. It checks the trading-day gate,
// fetches constituents, looks up the latest bars, and emits a tiny
// order list. Real strategies replace the body with their signal.
func Rebalance(ctx tf.Context, date time.Time) ([]tf.Order, error) {
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

	ctx.Logger().Info("rebalance",
		"date", date.Format("2006-01-02"),
		"target", target,
		"bars", len(bars),
		"cash", ctx.Cash(),
		"portfolio_value", ctx.PortfolioValue(),
	)

	pos := ctx.Position(target)
	if pos.Quantity > 0 {
		return nil, nil
	}

	limit := bars[len(bars)-1].Close
	return []tf.Order{{
		Symbol:   target,
		Side:     tf.Buy,
		Quantity: 10,
		Type:     tf.Limit,
		Limit:    &limit,
	}}, nil
}

// fakeBars builds a tiny deterministic OHLC dataset for AAPL/MSFT/NVDA.
func fakeBars() map[string][]tf.Bar {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	out := map[string][]tf.Bar{}
	prices := map[string]float64{
		"AAPL": 180.0,
		"MSFT": 370.0,
		"NVDA": 480.0,
	}
	for sym, p := range prices {
		bars := make([]tf.Bar, 0, 10)
		for d := 0; d < 10; d++ {
			date := start.AddDate(0, 0, d)
			if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
				continue
			}
			open := p
			close := p + float64(d)*0.5
			high := close + 1.0
			low := open - 0.5
			bars = append(bars, tf.Bar{
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

	cfg := &exampleConfig{store: map[string]string{
		"strategy":      "simple",
		"max_positions": "5",
	}}

	a := app.NewApp(
		app.WithMode(tf.Backtest),
		app.WithBacktestRange(
			time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		),
		app.WithStartingCash(10_000),
		app.WithMarket(market),
		app.WithConstituents(constituents),
		app.WithConfig(cfg),
		app.WithStdoutLogger(),
		app.WithBarSource(barSrc),
	)

	if err := a.RegisterJob(tf.Job{
		Name:    "refresh-universe",
		Time:    tf.Time(9, 0),
		Handler: Refresher,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register refresh-universe:", err)
		os.Exit(1)
	}
	if err := a.RegisterJob(tf.Job{
		Name:    "rebalance",
		Time:    tf.Time(16, 0),
		Handler: Rebalance,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register rebalance:", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}
}

// exampleConfig is a minimal in-memory tf.Config used only for the demo.
type exampleConfig struct {
	store map[string]string
}

func (c *exampleConfig) Get(key string) (string, error) {
	if v, ok := c.store[key]; ok {
		return v, nil
	}
	return "", tf.ErrNotFound
}

func (c *exampleConfig) Watch(key string, out any, factory any) (tf.Watcher, error) {
	return nil, fmt.Errorf("example: Watch not supported")
}
