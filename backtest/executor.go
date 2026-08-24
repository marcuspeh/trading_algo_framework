package backtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// Executor applies fills to a MemPortfolio using a FillModel, and tracks
// open orders for inspection.
type Executor struct {
	mu        sync.Mutex
	portfolio *portfolio.MemPortfolio
	fills     FillModel
	clock     *Clock
	nextID    atomic.Uint64
	open      map[string]tf.Order
}

func NewExecutor(p *portfolio.MemPortfolio, f FillModel, c *Clock) *Executor {
	if p == nil {
		p = portfolio.NewMem(0)
	}
	if f == nil {
		f = nopFillModel{}
	}
	if c == nil {
		c = NewClock(time.Now())
	}
	return &Executor{
		portfolio: p,
		fills:     f,
		clock:     c,
		open:      map[string]tf.Order{},
	}
}

// Submit attempts to fill the order at the simulated current time.
// Without fill, returns OrderResult with Filled=false.
func (e *Executor) Submit(_ context.Context, order tf.Order) (tf.OrderResult, error) {
	if order.Symbol == "" {
		return tf.OrderResult{}, fmt.Errorf("backtest executor: empty symbol")
	}
	if order.Quantity <= 0 {
		return tf.OrderResult{}, fmt.Errorf("backtest executor: non-positive quantity")
	}
	if order.Type == tf.Limit && (order.Limit == nil || *order.Limit <= 0) {
		return tf.OrderResult{}, fmt.Errorf("backtest executor: limit order missing price")
	}

	asOf := e.clock.Now()
	if asOf.Hour() == 0 && asOf.Minute() == 0 && asOf.Second() == 0 {
		asOf = e.clock.Date()
	}

	price, filled, ok := e.fills.Fill(order, asOf)
	id := fmt.Sprintf("bt-%d", e.nextID.Add(1))

	res := tf.OrderResult{ID: id}
	if !ok || !filled {
		e.mu.Lock()
		e.open[id] = order
		e.mu.Unlock()
		return res, nil
	}

	e.portfolio.ApplyFill(order.Symbol, order.Side, order.Quantity, price)
	e.portfolio.MarkPrice(order.Symbol, price)

	res.Filled = true
	res.FillPrice = price
	return res, nil
}

func (e *Executor) Positions(_ context.Context) ([]tf.Position, error) {
	return e.portfolio.Positions(), nil
}

// OpenOrders returns orders that did not fill (limits that didn't trigger,
// or orders on dates with no bar data).
func (e *Executor) OpenOrders(_ context.Context) ([]tf.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]tf.Order, 0, len(e.open))
	for _, o := range e.open {
		out = append(out, o)
	}
	return out, nil
}

func (e *Executor) Cancel(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.open[id]; !ok {
		return fmt.Errorf("backtest executor: no open order with id %q", id)
	}
	delete(e.open, id)
	return nil
}

func (e *Executor) Portfolio() *portfolio.MemPortfolio {
	return e.portfolio
}

type nopFillModel struct{}

func (nopFillModel) Fill(tf.Order, time.Time) (float64, bool, bool) { return 0, false, false }
