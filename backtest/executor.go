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

// Executor is the backtest Executor. It applies fills to a MemPortfolio
// using a FillModel, and tracks open/closed orders for inspection.
type Executor struct {
	mu        sync.Mutex
	portfolio *portfolio.MemPortfolio
	fills     FillModel
	clock     *Clock
	nextID    atomic.Uint64
	open      map[string]tf.Order
}

// NewExecutor constructs a BacktestExecutor.
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
// On fill, applies the fill to the portfolio and returns a populated
// OrderResult. Without fill, returns OrderResult with Filled=false.
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
		// Use Date() for fills so we always pull the same-day bar.
		asOf = e.clock.Date()
	}

	price, filled, ok := e.fills.Fill(order, asOf)
	id := fmt.Sprintf("bt-%d", e.nextID.Add(1))

	res := tf.OrderResult{ID: id}
	if !ok {
		// No bar data — keep the order "open" but don't apply a fill.
		e.mu.Lock()
		e.open[id] = order
		e.mu.Unlock()
		return res, nil
	}
	if !filled {
		// Bar existed but order didn't trigger. Leave open.
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

// Positions returns the current portfolio positions.
func (e *Executor) Positions(_ context.Context) ([]tf.Position, error) {
	return e.portfolio.Positions(), nil
}

// OpenOrders returns a snapshot of orders that did not fill (e.g.
// limit orders that didn't trigger, or orders on dates with no bar).
func (e *Executor) OpenOrders(_ context.Context) ([]tf.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]tf.Order, 0, len(e.open))
	for _, o := range e.open {
		out = append(out, o)
	}
	return out, nil
}

// Cancel removes an open order by framework-assigned ID.
func (e *Executor) Cancel(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.open[id]; !ok {
		return fmt.Errorf("backtest executor: no open order with id %q", id)
	}
	delete(e.open, id)
	return nil
}

// Portfolio returns the underlying MemPortfolio for the engine to mark
// prices and read state.
func (e *Executor) Portfolio() *portfolio.MemPortfolio {
	return e.portfolio
}

type nopFillModel struct{}

func (nopFillModel) Fill(tf.Order, time.Time) (float64, bool, bool) { return 0, false, false }
