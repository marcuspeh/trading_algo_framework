package app_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// lifecycleBars returns a bar for every trading day in the range, with
// a price that moves so a limit order triggers on a known day.
func lifecycleBars() map[string][]order.Bar {
	out := map[string][]order.Bar{}
	day := time.Date(2024, 1, 2, 0, 0, 0, 0, calendar.NYSE)
	for i := 0; i < 10; i++ {
		if !calendar.TradingDay(day) {
			day = day.AddDate(0, 0, 1)
			continue
		}
		// The price falls steadily, so a buy limit at 95 triggers on
		// the fourth session.
		close := 100 - float64(i)*2
		out["AAPL"] = append(out["AAPL"], order.Bar{
			Symbol:    "AAPL",
			Timestamp: day,
			Open:      close + 1,
			High:      close + 2,
			Low:       close - 1,
			Close:     close,
			Volume:    1_000_000,
		})
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// A good-till-cancelled limit order placed at the close rests until the
// market trades through it, and the job that placed it sees the fill on
// a later day. This is the behaviour a strategy relies on when it places
// an entry order and leaves it working.
func TestApp_GTCOrderRestsUntilItFills(t *testing.T) {
	bars := lifecycleBars()

	var cashByDay []float64
	var placed []int
	handler := func(ctx job.Context, date time.Time) ([]order.Order, error) {
		cashByDay = append(cashByDay, ctx.Cash())
		if ctx.Position("AAPL").Quantity > 0 {
			// Already filled: do nothing more.
			return nil, nil
		}
		// Do not place a second order while one is already working.
		opens, err := ctx.OpenOrders()
		if err != nil {
			return nil, err
		}
		for _, o := range opens {
			if o.Symbol == "AAPL" {
				return nil, nil
			}
		}
		placed = append(placed, len(opens))
		limit := 95.0
		return []order.Order{{
			Symbol: "AAPL", Side: order.Buy, Quantity: 10,
			Type: order.Limit, Limit: &limit,
		}}, nil
	}

	a := app.NewApp(
		app.WithBacktest(app.BacktestConfig{
			Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, calendar.NYSE),
			End:          time.Date(2024, 1, 12, 0, 0, 0, 0, calendar.NYSE),
			StartingCash: 10_000,
		}),
		app.WithMarketClient(clients.NewStaticBars(bars)),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
	)
	if err := a.RegisterJob(job.Job{
		Name: "entry", Time: job.AtClose(), Handler: handler,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}

	// The order is placed on the first day and fills on the session
	// whose low trades through 95. The sessions' lows are 99, 97, 95
	// and 93, so the third session is the one that fills it.
	if len(cashByDay) < 5 {
		t.Fatalf("the handler ran %d times", len(cashByDay))
	}
	if cashByDay[0] != 10_000 {
		t.Fatalf("day 1 cash=%f want 10000", cashByDay[0])
	}
	if cashByDay[1] != 10_000 {
		t.Fatalf("day 2 cash=%f want 10000 (the order should still be resting)", cashByDay[1])
	}
	if cashByDay[2] != 10_000-10*95 {
		t.Fatalf("day 3 cash=%f want %f (the order should have filled)", cashByDay[2], float64(10_000-10*95))
	}

	// Only one order was ever placed: the strategy saw its own order
	// resting and did not add a second.
	if len(placed) != 1 {
		t.Errorf("the strategy placed %d orders, want 1", len(placed))
	}
}
