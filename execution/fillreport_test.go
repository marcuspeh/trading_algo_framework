package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/broker/mock"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// fillBars is a single session's bar.
type fillBars map[string]order.Bar

func (b fillBars) Bar(symbol string, date time.Time) (order.Bar, bool) {
	bar, ok := b[symbol]
	if !ok {
		return order.Bar{}, false
	}
	return bar, true
}

// A broker that fills at once reports the price it filled at, so a
// backtest's logs say what happened rather than always reporting an
// unfilled order.
func TestExecutor_ReportsTheFillPrice(t *testing.T) {
	session := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	clk := clock.NewBacktest(session)
	clk.Set(session)

	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: fillBars{
			"AAPL": {Symbol: "AAPL", Timestamp: session, Open: 100, High: 110, Low: 90, Close: 105},
		},
		Session: clock.NewSession(clk, published.New()),
	})

	exec := execution.NewBroker(m)
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Filled {
		t.Error("the order was not reported as filled")
	}
	if res.FillPrice != 105 {
		t.Errorf("fill price=%f want 105", res.FillPrice)
	}
	if res.ID == "" {
		t.Error("the result carried no order id")
	}
}
