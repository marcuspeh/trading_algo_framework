package portfolio_test

import (
	"testing"

	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

func TestBacktest_ApplyFillOpenLong(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Buy, 10, 100)

	if p.Cash() != 9_000 {
		t.Fatalf("cash=%f", p.Cash())
	}
	pos := p.Position("AAPL")
	if pos.Quantity != 10 || pos.AvgPrice != 100 {
		t.Fatalf("AAPL=%+v", pos)
	}
}

func TestBacktest_AddToLongUpdatesAverage(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Buy, 10, 100)
	p.ApplyFill("AAPL", order.Buy, 10, 120)

	pos := p.Position("AAPL")
	if pos.Quantity != 20 {
		t.Fatalf("qty=%f", pos.Quantity)
	}
	if pos.AvgPrice != 110 {
		t.Fatalf("avg=%f", pos.AvgPrice)
	}
}

func TestBacktest_SellClosesLongAndOpensShort(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Buy, 10, 100)
	p.ApplyFill("AAPL", order.Sell, 15, 110)

	pos := p.Position("AAPL")
	if pos.Quantity != -5 {
		t.Fatalf("qty=%f", pos.Quantity)
	}
	if pos.AvgPrice != 110 {
		t.Fatalf("avg=%f", pos.AvgPrice)
	}
}

func TestBacktest_BuyCoversShortThenOpensLong(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Sell, 10, 100)
	p.ApplyFill("AAPL", order.Buy, 15, 90)

	pos := p.Position("AAPL")
	if pos.Quantity != 5 {
		t.Fatalf("qty=%f", pos.Quantity)
	}
	if pos.AvgPrice != 90 {
		t.Fatalf("avg=%f", pos.AvgPrice)
	}
}

func TestBacktest_MarkPriceAffectsValue(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Buy, 10, 100)
	p.MarkPrice("AAPL", 110)
	if got := p.Value(); got != 9_000+10*110 {
		t.Fatalf("value=%f", got)
	}
}

func TestBacktest_PositionsSortedAndFiltered(t *testing.T) {
	p := portfolio.NewBacktest(0)
	p.ApplyFill("NVDA", order.Buy, 1, 1)
	p.ApplyFill("AAPL", order.Buy, 1, 1)
	p.ApplyFill("MSFT", order.Buy, 0, 1) // zero qty should be filtered
	ps := p.Positions()
	if len(ps) != 2 {
		t.Fatalf("positions=%d", len(ps))
	}
	if ps[0].Symbol != "AAPL" || ps[1].Symbol != "NVDA" {
		t.Fatalf("positions=%+v", ps)
	}
}

func TestLive_SyncOverrides(t *testing.T) {
	p := portfolio.NewLive()
	p.Sync([]order.Position{
		{Symbol: "AAPL", Quantity: 10, AvgPrice: 100},
		{Symbol: "MSFT", Quantity: -3, AvgPrice: 250},
	}, 5000, true)
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("AAPL=%f", got)
	}
	if got := p.Position("MSFT").Quantity; got != -3 {
		t.Fatalf("MSFT=%f", got)
	}
	if got := p.Value(); got != 5000 {
		t.Fatalf("value=%f", got)
	}
}

func TestLive_ValueFallsBackWhenNoNav(t *testing.T) {
	p := portfolio.NewLive()
	p.Sync([]order.Position{{Symbol: "AAPL", Quantity: 10, AvgPrice: 100}}, 0, false)
	if got := p.Value(); got != 1000 {
		t.Fatalf("value=%f", got)
	}
}
