package mock_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/broker/mock"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// bars is a fixed set of daily bars, keyed by date.
type bars map[string]order.Bar

func (b bars) Bar(symbol string, date time.Time) (order.Bar, bool) {
	bar, ok := b[date.Format("2006-01-02")]
	if !ok {
		return order.Bar{}, false
	}
	bar.Symbol = symbol
	return bar, true
}

// session builds a session whose clock reads the given moment.
//
// The backtest clock normalises a seed to midnight, so the time is set
// explicitly - a mock placed at 09:00 must anchor to the session
// before it, not to the same session.
func session(t *testing.T, at time.Time) *clock.Session {
	t.Helper()
	clk := clock.NewBacktest(at)
	clk.Set(at)
	return clock.NewSession(clk, published.New())
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, calendar.NYSE)
}

// A market order fills at the close of the session the strategy is
// reading - the same price it saw.
func TestMock_MarketOrderFillsAtTheSessionClose(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 100, High: 210, Low: 195, Close: 205},
		},
		Session: session(t, at),
	})

	_, err := m.PlaceMarketOrder(context.Background(), broker.MarketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}

	pos := p.Position("AAPL")
	if pos.Quantity != 10 {
		t.Fatalf("quantity=%f want 10", pos.Quantity)
	}
	if pos.AvgPrice != 205 {
		t.Errorf("fill price=%f want the session close 205", pos.AvgPrice)
	}
	if cash := p.Cash(); cash != 10_000-10*205 {
		t.Errorf("cash=%f", cash)
	}
}

// A resting limit order is never evaluated against a session that
// closed before it was submitted: an order must not fill at a price
// printed before it existed.
func TestMock_LimitOrderDoesNotFillAtAPreExistingPrice(t *testing.T) {
	// The order is submitted at 09:00 on Jan 3, so the session it is
	// first evaluated against is Jan 3's.
	at := time.Date(2024, 1, 3, 9, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-02": {Timestamp: day(2024, 1, 2), Open: 100, High: 110, Low: 90, Close: 95},
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 100, High: 110, Low: 90, Close: 95},
		},
		Session: session(t, at),
	})

	limit := 100.0
	res, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "GTC",
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Submitted" {
		t.Fatalf("status=%q want Submitted", res.Status)
	}

	// Evaluating Jan 2 - the session that closed before the order
	// existed - must not fill it, even though that bar's low was
	// below the limit.
	m.AdvanceSession(day(2024, 1, 2))
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("the order filled at a price printed before it existed: qty=%f", got)
	}

	// Evaluating Jan 3 fills it, because that bar trades below the
	// limit too.
	m.AdvanceSession(day(2024, 1, 3))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("quantity=%f want 10", got)
	}
}

// An order resting across sessions fills exactly once.
func TestMock_RestingOrderFillsExactlyOnce(t *testing.T) {
	at := time.Date(2024, 1, 3, 9, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 100, High: 110, Low: 90, Close: 105},
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 110, Low: 90, Close: 105},
		},
		Session: session(t, at),
	})

	limit := 95.0
	if _, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "GTC",
	}, "key-1"); err != nil {
		t.Fatal(err)
	}

	m.AdvanceSession(day(2024, 1, 3))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("quantity=%f want 10", got)
	}

	// The order is gone, so a later session must not fill it again.
	m.AdvanceSession(day(2024, 1, 4))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("the order filled twice: qty=%f", got)
	}

	opens, err := m.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 0 {
		t.Fatalf("expected no resting orders, got %d", len(opens))
	}
}

// A day order expires at the close of the session it was submitted for.
func TestMock_DayOrderExpires(t *testing.T) {
	at := time.Date(2024, 1, 3, 9, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-03": {Timestamp: day(2024, 1, 3), Open: 200, High: 210, Low: 195, Close: 205},
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 200, High: 210, Low: 195, Close: 205},
		},
		Session: session(t, at),
	})

	limit := 100.0
	if _, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "DAY",
	}, "key-1"); err != nil {
		t.Fatal(err)
	}

	// Jan 3's session closes and the order does not trigger.
	m.AdvanceSession(day(2024, 1, 3))
	opens, err := m.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 0 {
		t.Fatalf("a day order should have expired, %d still resting", len(opens))
	}
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("quantity=%f want 0", got)
	}
}

// The same idempotency key returns the same order, the way a real
// broker does.
func TestMock_IdempotencyKeyIsHonoured(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-03": {Timestamp: day(2024, 1, 3), Close: 100},
		},
		Session: session(t, at),
	})

	req := broker.MarketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
	}
	first, err := m.PlaceMarketOrder(context.Background(), req, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.PlaceMarketOrder(context.Background(), req, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if first.OrderId != second.OrderId {
		t.Errorf("the same key produced two orders: %d and %d", first.OrderId, second.OrderId)
	}
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Errorf("quantity=%f want 10", got)
	}
}

// A resting order's limit price can be changed.
func TestMock_UpdateOrder(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 110, Low: 90, Close: 105},
		},
		Session: session(t, at),
	})

	limit := 95.0
	res, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "GTC",
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}

	newLimit := 85.0
	if _, err := m.UpdateOrder(context.Background(), res.OrderId, broker.UpdateOrderRequest{
		LmtPrice: &newLimit,
	}); err != nil {
		t.Fatal(err)
	}

	// The bar's low is 90, above the new limit, so the order must not
	// fill - it would have filled at the old limit of 95.
	m.AdvanceSession(day(2024, 1, 4))
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("the order filled at the old limit: qty=%f", got)
	}
}

// A bracket's entry fills first, and its protective legs only become
// live afterwards.
func TestMock_BracketStopLossTriggersAfterTheEntryFills(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			// The entry triggers (low 96 is below the 102 limit) but
			// the stop at 95 is not reached.
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 105, Low: 96, Close: 100},
			// The next session reaches the stop.
			"2024-01-05": {Timestamp: day(2024, 1, 5), Open: 100, High: 102, Low: 80, Close: 90},
		},
		Session: session(t, at),
	})

	res, err := m.PlaceBracketOrder(context.Background(), broker.BracketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice:  102,
		TPPrice:   120,
		SLPrice:   95,
		ParentTIF: "GTC",
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.ParentOrderId == 0 {
		t.Error("the bracket was not acknowledged")
	}

	// The entry fills and the position opens.
	m.AdvanceSession(day(2024, 1, 4))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("quantity=%f want 10", got)
	}

	// The stop triggers in the next session and closes the position.
	m.AdvanceSession(day(2024, 1, 5))
	pos := p.Position("AAPL")
	if pos.Quantity != 0 {
		t.Fatalf("quantity=%f want 0", pos.Quantity)
	}
}

// Slippage and commission are applied to a fill.
func TestMock_CostsAreApplied(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-03": {Timestamp: day(2024, 1, 3), Close: 100},
		},
		Session:            session(t, at),
		SlippageBps:        10, // 0.1%
		Commission:         1,
		CommissionPerShare: 0.01,
	})

	if _, err := m.PlaceMarketOrder(context.Background(), broker.MarketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
	}, "key-1"); err != nil {
		t.Fatal(err)
	}

	// 100 + 0.1% = 100.1 per share, plus 1 + 10*0.01 commission.
	want := 10_000 - 10*100.1 - 1 - 10*0.01
	if got := p.Cash(); got < want-0.001 || got > want+0.001 {
		t.Errorf("cash=%f want %f", got, want)
	}
}

// A repeated idempotency key reports what actually happened to the
// order: a cancelled order is cancelled, not filled.
func TestMock_RepeatedKeyReportsTheRealStatus(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 110, Low: 90, Close: 105},
		},
		Session: session(t, at),
	})

	limit := 95.0
	res, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "GTC",
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}

	if err := m.CancelOrder(context.Background(), res.OrderId); err != nil {
		t.Fatal(err)
	}

	again, err := m.PlaceLimitOrder(context.Background(), broker.LimitOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice: limit,
		TIF:      "GTC",
	}, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if again.OrderId != res.OrderId {
		t.Errorf("the repeated key produced a new order %d, want %d", again.OrderId, res.OrderId)
	}
	if again.Status != "Cancelled" {
		t.Errorf("status=%q want Cancelled", again.Status)
	}
}

// A stop-loss fires when the market falls through it, and fills at the
// stop or worse.
//
// Arming the stop as a limit order on the opposite side would fire it
// when the price rose to the stop instead, and would leave the
// position unprotected exactly when it needed protecting.
func TestMock_StopLossFiresOnADecline(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			// The entry fills here.
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 105, Low: 96, Close: 100},
			// The next session crashes through the 95 stop.
			"2024-01-05": {Timestamp: day(2024, 1, 5), Open: 94, High: 94, Low: 80, Close: 82},
		},
		Session: session(t, at),
	})

	if _, err := m.PlaceBracketOrder(context.Background(), broker.BracketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice:  102,
		TPPrice:   120,
		SLPrice:   95,
		ParentTIF: "GTC",
	}, "key-1"); err != nil {
		t.Fatal(err)
	}

	// The entry fills and the position opens.
	m.AdvanceSession(day(2024, 1, 4))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("quantity=%f want 10", got)
	}

	// The market falls through the stop and the position closes.
	m.AdvanceSession(day(2024, 1, 5))
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("the stop did not fire: quantity=%f", got)
	}
	// The entry filled at 100 and the stop at 94, the session's open,
	// which is below the stop.
	if got, want := p.Cash(), float64(10_000-10*100+10*94); got != want {
		t.Errorf("cash=%f want %f", got, want)
	}
}

// A take-profit fires when the market rises through it.
func TestMock_TakeProfitFiresOnARise(t *testing.T) {
	at := time.Date(2024, 1, 3, 16, 0, 0, 0, calendar.NYSE)
	p := portfolio.NewBacktest(10_000)
	m := mock.New(mock.Config{
		Portfolio: p,
		Bars: bars{
			"2024-01-04": {Timestamp: day(2024, 1, 4), Open: 100, High: 105, Low: 96, Close: 100},
			"2024-01-05": {Timestamp: day(2024, 1, 5), Open: 110, High: 125, Low: 108, Close: 122},
		},
		Session: session(t, at),
	})

	if _, err := m.PlaceBracketOrder(context.Background(), broker.BracketOrderRequest{
		OrderRequest: broker.OrderRequest{
			Contract:      broker.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
		},
		LmtPrice:  102,
		TPPrice:   120,
		SLPrice:   95,
		ParentTIF: "GTC",
	}, "key-1"); err != nil {
		t.Fatal(err)
	}

	m.AdvanceSession(day(2024, 1, 4))
	if got := p.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("quantity=%f want 10", got)
	}

	m.AdvanceSession(day(2024, 1, 5))
	if got := p.Position("AAPL").Quantity; got != 0 {
		t.Fatalf("the take-profit did not fire: quantity=%f", got)
	}
	// The entry filled at 100 and the take-profit at 120, the target,
	// not at the session's open of 110.
	if got, want := p.Cash(), float64(10_000-10*100+10*120); got != want {
		t.Errorf("cash=%f want %f", got, want)
	}
}

// The mock needs a session, and a session needs the exchange calendar
// for its boundaries. With neither, the mock panics rather than guess
// when a session ended.
func TestMock_NewRequiresASessionOrACalendar(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic when neither a session nor a calendar is supplied")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value = %v want a string", r)
		}
		if !strings.Contains(msg, "session or a calendar") {
			t.Errorf("panic message = %q want it to name what is missing", msg)
		}
	}()
	mock.New(mock.Config{})
}

// A calendar alone is enough: the session's boundaries come from it.
func TestMock_NewAcceptsACalendar(t *testing.T) {
	m := mock.New(mock.Config{Calendar: published.New()})
	if m == nil {
		t.Fatal("expected a mock")
	}
}
