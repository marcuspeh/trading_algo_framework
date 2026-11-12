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
	// Short sold 10 @ 100 (+1000), bought 15 @ 90 (-1350).
	if got := p.Cash(); got != 9_650 {
		t.Fatalf("cash=%f want 9650", got)
	}
}

// Covering a short must debit cash for the shares bought back. The
// framework used to update the position without moving cash, so a
// short that was covered reported a profit that never happened.
func TestBacktest_CoveringShortDebitsCash(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Sell, 10, 100) // short 10 @ 100
	if got := p.Cash(); got != 11_000 {
		t.Fatalf("after short: cash=%f want 11000", got)
	}

	p.ApplyFill("AAPL", order.Buy, 3, 90) // cover 3 @ 90
	if got := p.Cash(); got != 10_730 {
		t.Fatalf("after partial cover: cash=%f want 10730", got)
	}
	if got := p.Position("AAPL").Quantity; got != -7 {
		t.Fatalf("after partial cover: qty=%f want -7", got)
	}
}

func TestBacktest_FullCoverOfShortDebitsCash(t *testing.T) {
	p := portfolio.NewBacktest(10_000)
	p.ApplyFill("AAPL", order.Sell, 10, 100) // short 10 @ 100
	p.ApplyFill("AAPL", order.Buy, 10, 90)   // cover all @ 90

	// 10000 + 1000 (short proceeds) - 900 (buy back) = 10100, which
	// books the 100 of profit the short actually made.
	if got := p.Cash(); got != 10_100 {
		t.Fatalf("cash=%f want 10100", got)
	}
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("qty=%f want 0", got)
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
