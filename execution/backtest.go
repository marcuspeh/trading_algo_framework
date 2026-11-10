package execution

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// Backtest is the deterministic Executor for backtest mode. It applies
// fills to a portfolio.Backtest via a FillModel.
type Backtest struct {
	mu        sync.Mutex
	portfolio *portfolio.Backtest
	fills     FillModel
	clock     clock.Clock
	nextID    atomic.Uint64
	open      map[string]openOrder
}

// openOrder is a resting order awaiting a fill.
type openOrder struct {
	order     order.Order
	submitted time.Time
}

func NewBacktest(p *portfolio.Backtest, f FillModel, c clock.Clock) *Backtest {
	if p == nil {
		p = portfolio.NewBacktest(0)
	}
	if f == nil {
		f = nopFillModel{}
	}
	if c == nil {
		c = clock.NewBacktest(time.Now())
	}
	return &Backtest{
		portfolio: p,
		fills:     f,
		clock:     c,
		open:      map[string]openOrder{},
	}
}

func (e *Backtest) Submit(_ context.Context, o order.Order) (order.Result, error) {
	if o.Symbol == "" {
		return order.Result{}, fmt.Errorf("backtest executor: empty symbol")
	}
	if o.Quantity <= 0 {
		return order.Result{}, fmt.Errorf("backtest executor: non-positive quantity")
	}
	if o.Type == order.Limit && (o.Limit == nil || *o.Limit <= 0) {
		return order.Result{}, fmt.Errorf("backtest executor: limit order missing price")
	}

	asOf := e.asOf()
	price, filled, ok := e.fills.Fill(o, asOf)
	id := fmt.Sprintf("bt-%d", e.nextID.Add(1))

	res := order.Result{ID: id}
	if !ok || !filled {
		e.mu.Lock()
		e.open[id] = openOrder{order: o, submitted: asOf}
		e.mu.Unlock()
		return res, nil
	}

	e.applyFill(o, price)

	res.Filled = true
	res.FillPrice = price
	return res, nil
}

// AdvanceDay re-evaluates every resting order against the given day's
// bar. Resting orders are good-till-cancelled: an order that did not
// trigger on its submission day stays live and is retried on every
// subsequent day until it fills or is cancelled.
func (e *Backtest) AdvanceDay(date time.Time) {
	e.mu.Lock()
	ids := make([]string, 0, len(e.open))
	for id := range e.open {
		ids = append(ids, id)
	}
	e.mu.Unlock()

	for _, id := range ids {
		e.mu.Lock()
		resting, exists := e.open[id]
		e.mu.Unlock()
		if !exists {
			continue
		}

		price, filled, ok := e.fills.Fill(resting.order, date)
		if !ok || !filled {
			continue
		}

		e.applyFill(resting.order, price)

		e.mu.Lock()
		delete(e.open, id)
		e.mu.Unlock()
	}
}

// applyFill books a fill into the portfolio and charges any commission.
func (e *Backtest) applyFill(o order.Order, price float64) {
	e.portfolio.ApplyFill(o.Symbol, o.Side, o.Quantity, price)
	e.portfolio.MarkPrice(o.Symbol, price)

	if cm, ok := e.fills.(CostModel); ok {
		if fee := cm.CommissionFor(o.Quantity); fee > 0 {
			e.portfolio.ChargeFee(fee)
		}
	}
}

// asOf returns the time the order should be filled against.
func (e *Backtest) asOf() time.Time {
	now := e.clock.Now()
	if now.Hour() == 0 && now.Minute() == 0 && now.Second() == 0 {
		return e.clock.Today()
	}
	return now
}

func (e *Backtest) Positions(_ context.Context) ([]order.Position, error) {
	return e.portfolio.Positions(), nil
}

func (e *Backtest) OpenOrders(_ context.Context) ([]order.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]order.Order, 0, len(e.open))
	for _, resting := range e.open {
		out = append(out, resting.order)
	}
	return out, nil
}

func (e *Backtest) Cancel(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.open[id]; !ok {
		return fmt.Errorf("backtest executor: no open order with id %q", id)
	}
	delete(e.open, id)
	return nil
}

func (e *Backtest) Portfolio() *portfolio.Backtest { return e.portfolio }

type nopFillModel struct{}

func (nopFillModel) Fill(order.Order, time.Time) (float64, bool, bool) {
	return 0, false, false
}
