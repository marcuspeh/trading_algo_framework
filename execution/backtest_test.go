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

type staticBars struct {
	byDate map[string]map[string]order.Bar
}

func (s *staticBars) Bar(symbol string, date time.Time) (order.Bar, bool) {
	d := date.Format("2006-01-02")
	if m, ok := s.byDate[symbol]; ok {
		b, ok := m[d]
		return b, ok
	}
	return order.Bar{}, false
}

func TestBacktestExecutor_MarketOrderFillsAtClose(t *testing.T) {
	clk := clock.NewBacktest(time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC))
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Close: 100}},
	}}
	exec := execution.NewBacktest(p, execution.NewDefaultFill(bars), clk)

	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Filled {
		t.Fatalf("expected fill")
	}
	if res.FillPrice != 100 {
		t.Fatalf("price=%f", res.FillPrice)
	}
	if p.Position("AAPL").Quantity != 10 {
		t.Fatalf("qty=%f", p.Position("AAPL").Quantity)
	}
}

func TestBacktestExecutor_LimitTriggersBelowLow(t *testing.T) {
	clk := clock.NewBacktest(time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC))
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {
			Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Open:      110, High: 115, Low: 95, Close: 105,
		}},
	}}
	exec := execution.NewBacktest(p, execution.NewDefaultFill(bars), clk)

	limit := 100.0
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Filled || res.FillPrice != 100 {
		t.Fatalf("res=%+v", res)
	}
}

func TestBacktestExecutor_LimitNoTriggerStaysOpen(t *testing.T) {
	clk := clock.NewBacktest(time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC))
	p := portfolio.NewBacktest(10_000)
	bars := &staticBars{byDate: map[string]map[string]order.Bar{
		"AAPL": {"2024-01-02": {
			Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Open:      200, High: 210, Low: 195, Close: 205,
		}},
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
		t.Fatalf("should not fill at 100 when Low=195")
	}
	opens, err := exec.OpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 1 {
		t.Fatalf("opens=%d", len(opens))
	}
}

func TestBacktestExecutor_Validates(t *testing.T) {
	exec := execution.NewBacktest(portfolio.NewBacktest(0), nil, nil)
	if _, err := exec.Submit(context.Background(), order.Order{}); err == nil {
		t.Fatal("expected empty-symbol error")
	}
	if _, err := exec.Submit(context.Background(), order.Order{Symbol: "X", Quantity: -1, Type: order.Market}); err == nil {
		t.Fatal("expected qty error")
	}
	limit := 0.0
	if _, err := exec.Submit(context.Background(), order.Order{Symbol: "X", Quantity: 1, Type: order.Limit, Limit: &limit}); err == nil {
		t.Fatal("expected limit error")
	}
}
