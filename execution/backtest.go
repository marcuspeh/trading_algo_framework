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
	open      map[string]order.Order
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
		open:      map[string]order.Order{},
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

	asOf := e.clock.Now()
	if asOf.Hour() == 0 && asOf.Minute() == 0 && asOf.Second() == 0 {
		asOf = e.clock.Today()
	}

	price, filled, ok := e.fills.Fill(o, asOf)
	id := fmt.Sprintf("bt-%d", e.nextID.Add(1))

	res := order.Result{ID: id}
	if !ok || !filled {
		e.mu.Lock()
		e.open[id] = o
		e.mu.Unlock()
		return res, nil
	}

	e.portfolio.ApplyFill(o.Symbol, o.Side, o.Quantity, price)
	e.portfolio.MarkPrice(o.Symbol, price)

	res.Filled = true
	res.FillPrice = price
	return res, nil
}

func (e *Backtest) Positions(_ context.Context) ([]order.Position, error) {
	return e.portfolio.Positions(), nil
}

func (e *Backtest) OpenOrders(_ context.Context) ([]order.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]order.Order, 0, len(e.open))
	for _, o := range e.open {
		out = append(out, o)
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
