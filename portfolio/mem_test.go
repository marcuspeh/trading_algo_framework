package portfolio_test

import (
	"testing"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/portfolio"
)

func TestMemPortfolio_ApplyFillBuy(t *testing.T) {
	p := portfolio.NewMem(10_000)
	p.ApplyFill("AAPL", tf.Buy, 100, 50)
	if p.Cash() != 10_000-5000 {
		t.Fatalf("cash: %f", p.Cash())
	}
	pos := p.Position("AAPL")
	if pos.Quantity != 100 || pos.AvgPrice != 50 {
		t.Fatalf("unexpected pos: %+v", pos)
	}
}

func TestMemPortfolio_ApplyFillBuyAddsWeightedAvg(t *testing.T) {
	p := portfolio.NewMem(10_000)
	p.ApplyFill("AAPL", tf.Buy, 100, 50)
	p.ApplyFill("AAPL", tf.Buy, 100, 70)
	pos := p.Position("AAPL")
	if pos.Quantity != 200 {
		t.Fatalf("qty: %f", pos.Quantity)
	}
	if pos.AvgPrice != 60 {
		t.Fatalf("avg: %f", pos.AvgPrice)
	}
}

func TestMemPortfolio_ApplyFillSellPartial(t *testing.T) {
	p := portfolio.NewMem(10_000)
	p.ApplyFill("AAPL", tf.Buy, 100, 50)
	p.ApplyFill("AAPL", tf.Sell, 40, 60)
	pos := p.Position("AAPL")
	if pos.Quantity != 60 {
		t.Fatalf("qty: %f", pos.Quantity)
	}
	// Avg cost stays the same after a partial sell.
	if pos.AvgPrice != 50 {
		t.Fatalf("avg: %f", pos.AvgPrice)
	}
	if p.Cash() != 10_000-5000+40*60 {
		t.Fatalf("cash: %f", p.Cash())
	}
}

func TestMemPortfolio_ApplyFillSellFlipsToShort(t *testing.T) {
	p := portfolio.NewMem(10_000)
	p.ApplyFill("AAPL", tf.Buy, 100, 50)
	p.ApplyFill("AAPL", tf.Sell, 150, 60)
	pos := p.Position("AAPL")
	if pos.Quantity != -50 {
		t.Fatalf("qty: %f", pos.Quantity)
	}
	if pos.AvgPrice != 60 {
		t.Fatalf("avg: %f", pos.AvgPrice)
	}
}

func TestMemPortfolio_ValueUsesMark(t *testing.T) {
	p := portfolio.NewMem(1_000)
	p.ApplyFill("AAPL", tf.Buy, 10, 100)
	p.MarkPrice("AAPL", 110)
	if got := p.Value(); got != 1_000-1_000+10*110 {
		t.Fatalf("value: %f", got)
	}
}

func TestMemPortfolio_PositionsSortedAndFiltersZero(t *testing.T) {
	p := portfolio.NewMem(0)
	p.ApplyFill("AAPL", tf.Buy, 10, 100)
	p.ApplyFill("MSFT", tf.Sell, 5, 200)
	pos := p.Positions()
	if len(pos) != 2 || pos[0].Symbol != "AAPL" || pos[1].Symbol != "MSFT" {
		t.Fatalf("positions: %+v", pos)
	}
	p.ApplyFill("AAPL", tf.Sell, 10, 100)
	if len(p.Positions()) != 1 {
		t.Fatalf("zero position should be filtered: %+v", p.Positions())
	}
}

func TestMirrorPortfolio_SyncWins(t *testing.T) {
	m := portfolio.NewMirror()
	m.Sync([]tf.Position{
		{Symbol: "AAPL", Quantity: 10, AvgPrice: 100},
		{Symbol: "MSFT", Quantity: -3, AvgPrice: 50},
	}, 5000, true)
	if m.Value() != 5000 {
		t.Fatalf("value: %f", m.Value())
	}
	if m.Position("AAPL").Quantity != 10 {
		t.Fatal("AAPL position mismatch")
	}
	if m.Position("MSFT").Quantity != -3 {
		t.Fatal("MSFT position mismatch")
	}
	// Empty out.
	m.Sync(nil, 0, false)
	if m.Value() != 0 {
		t.Fatalf("expected zero after empty sync, got %f", m.Value())
	}
}

func TestMirrorPortfolio_FallsBackToNotionalWhenNoNav(t *testing.T) {
	m := portfolio.NewMirror()
	m.Sync([]tf.Position{{Symbol: "AAPL", Quantity: 10, AvgPrice: 100}}, 0, false)
	if got := m.Value(); got != 1000 {
		t.Fatalf("value: %f", got)
	}
}
