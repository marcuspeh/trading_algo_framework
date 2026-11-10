package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// day returns midnight on the given date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// clockAt returns a backtest clock positioned on the given date.
func clockAt(y int, m time.Month, d int) *clock.BacktestClock {
	return clock.NewBacktest(day(y, m, d))
}

func TestDefaultFill_SlippageMovesPriceAgainstTrader(t *testing.T) {
	clk := clockAt(2024, 1, 2)
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {Timestamp: day(2024, 1, 2), Close: 100}},
	}}
	// 10 bps = 0.1%.
	fill := &execution.DefaultFill{Bars: bars, SlippageBps: 10}
	exec := execution.NewBacktest(p, fill, clk)

	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FillPrice != 100.1 {
		t.Fatalf("buy fill=%f want 100.1", res.FillPrice)
	}

	// A sell slips down.
	p2 := portfolio.NewBacktest(10_000)
	exec2 := execution.NewBacktest(p2, fill, clk)
	res2, err := exec2.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Sell, Quantity: 10, Type: order.Market,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.FillPrice != 99.9 {
		t.Fatalf("sell fill=%f want 99.9", res2.FillPrice)
	}
}

func TestDefaultFill_LimitOrderNeverFillsPastLimit(t *testing.T) {
	clk := clockAt(2024, 1, 2)
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {Timestamp: day(2024, 1, 2), Open: 100, High: 105, Low: 95, Close: 100}},
	}}
	// 100 bps of slippage would push a limit buy of 100 up to 101, but
	// a limit order must never fill above its limit.
	fill := &execution.DefaultFill{Bars: bars, SlippageBps: 100}
	exec := execution.NewBacktest(p, fill, clk)

	limit := 100.0
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Filled {
		t.Fatal("expected fill")
	}
	if res.FillPrice > limit {
		t.Fatalf("fill=%f exceeds limit %f", res.FillPrice, limit)
	}
}

func TestDefaultFill_CommissionIsCharged(t *testing.T) {
	clk := clockAt(2024, 1, 2)
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {Timestamp: day(2024, 1, 2), Close: 100}},
	}}
	fill := &execution.DefaultFill{Bars: bars, Commission: 5, CommissionPerShare: 0.01}
	exec := execution.NewBacktest(p, fill, clk)

	if _, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	}); err != nil {
		t.Fatal(err)
	}
	// 10 shares at 100 = 1000 spent, plus 5 + 10*0.01 = 5.10 commission.
	want := 10_000 - 1_000 - 5.1
	if got := p.Cash(); got != want {
		t.Fatalf("cash=%f want %f", got, want)
	}
}

func TestBacktestExecutor_RestingLimitOrderFillsOnLaterDay(t *testing.T) {
	clk := clockAt(2024, 1, 2)
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {
			"2024-01-02": {Timestamp: day(2024, 1, 2), Open: 200, High: 210, Low: 195, Close: 205},
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 90, High: 95, Low: 85, Close: 90},
		},
	}}
	exec := execution.NewBacktest(p, execution.NewDefaultFill(bars), clk)

	limit := 100.0
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Filled {
		t.Fatal("should not fill on day 1 when Low=195")
	}

	// The order rests and fills when day 2 trades through the limit.
	exec.AdvanceDay(day(2024, 1, 3))

	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("qty=%f want 10", got)
	}
	opens, err := exec.OpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 0 {
		t.Fatalf("expected no resting orders, got %d", len(opens))
	}
}

func TestBacktestExecutor_RestingOrderStaysOpenWhenNotTriggered(t *testing.T) {
	clk := clockAt(2024, 1, 2)
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {
			"2024-01-02": {Timestamp: day(2024, 1, 2), Open: 200, High: 210, Low: 195, Close: 205},
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 200, High: 210, Low: 195, Close: 205},
		},
	}}
	exec := execution.NewBacktest(p, execution.NewDefaultFill(bars), clk)

	limit := 100.0
	if _, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit,
	}); err != nil {
		t.Fatal(err)
	}

	exec.AdvanceDay(day(2024, 1, 3))

	opens, err := exec.OpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 1 {
		t.Fatalf("expected order to stay open, got %d", len(opens))
	}
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("qty=%f want 0", got)
	}
}
