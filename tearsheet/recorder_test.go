package tearsheet

import (
	"math"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// day is a date in the test's own zone.
func day(d int) time.Time {
	return time.Date(2025, 1, d, 16, 0, 0, 0, time.UTC)
}

func TestRecorder_MatchesFIFO(t *testing.T) {
	r := NewRecorder()

	// Two entries, then one exit that closes the older one.
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(2)})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 110, Time: day(3)})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 120, Time: day(4)})

	rep := r.Report(Meta{})
	if len(rep.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(rep.Trades))
	}

	trade := rep.Trades[0]
	if trade.EntryPrice != 100 {
		t.Errorf("expected the oldest entry at 100, got %v", trade.EntryPrice)
	}
	if trade.ExitPrice != 120 {
		t.Errorf("expected the exit at 120, got %v", trade.ExitPrice)
	}
	if trade.PnL != 200 {
		t.Errorf("expected a profit of 200, got %v", trade.PnL)
	}
	if trade.Side != "LONG" {
		t.Errorf("expected a LONG trade, got %q", trade.Side)
	}
	if trade.Duration != 48*time.Hour {
		t.Errorf("expected 48h held, got %v", trade.Duration)
	}
}

func TestRecorder_PartialClose(t *testing.T) {
	r := NewRecorder()

	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(2)})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 4, Price: 110, Time: day(3)})

	rep := r.Report(Meta{})
	if len(rep.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(rep.Trades))
	}
	if rep.Trades[0].Quantity != 4 {
		t.Errorf("expected 4 shares closed, got %v", rep.Trades[0].Quantity)
	}
	if rep.Trades[0].PnL != 40 {
		t.Errorf("expected a profit of 40, got %v", rep.Trades[0].PnL)
	}
	// The rest of the position is still open.
	if rep.Metrics.OpenPositions != 1 {
		t.Errorf("expected 1 open position, got %d", rep.Metrics.OpenPositions)
	}
}

func TestRecorder_FlipFromLongToShort(t *testing.T) {
	r := NewRecorder()

	// Buy 10, then sell 15: the 10 close the long, the 5 open a short.
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(2)})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 15, Price: 110, Time: day(3)})

	rep := r.Report(Meta{})
	if len(rep.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(rep.Trades))
	}
	if rep.Trades[0].PnL != 100 {
		t.Errorf("expected a profit of 100, got %v", rep.Trades[0].PnL)
	}
	if rep.Metrics.OpenPositions != 1 {
		t.Errorf("expected 1 open position, got %d", rep.Metrics.OpenPositions)
	}
}

func TestRecorder_ShortTradeProfitsWhenPriceFalls(t *testing.T) {
	r := NewRecorder()

	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 100, Time: day(2)})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 90, Time: day(3)})

	rep := r.Report(Meta{})
	if len(rep.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(rep.Trades))
	}
	if rep.Trades[0].Side != "SHORT" {
		t.Errorf("expected a SHORT trade, got %q", rep.Trades[0].Side)
	}
	if rep.Trades[0].PnL != 100 {
		t.Errorf("expected a profit of 100, got %v", rep.Trades[0].PnL)
	}
}

func TestRecorder_CommissionIsTakenOutOfTheProfit(t *testing.T) {
	r := NewRecorder()

	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(2), Commission: 2})
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 110, Time: day(3), Commission: 2})

	rep := r.Report(Meta{})
	if rep.Trades[0].PnL != 96 {
		t.Errorf("expected a profit of 96, got %v", rep.Trades[0].PnL)
	}
	if rep.Metrics.TotalFees != 4 {
		t.Errorf("expected 4 in fees, got %v", rep.Metrics.TotalFees)
	}
}

func TestRecorder_ExposureCountsTheDaysWithAPosition(t *testing.T) {
	r := NewRecorder()

	r.OnEquity(day(1), 10_000)
	r.OnEquity(day(2), 10_000)
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(2)})
	r.OnEquity(day(3), 10_100)
	r.OnEquity(day(4), 10_200)

	rep := r.Report(Meta{})
	if math.Abs(rep.Metrics.Exposure-0.5) > 1e-9 {
		t.Errorf("expected an exposure of 0.5, got %v", rep.Metrics.Exposure)
	}
}
