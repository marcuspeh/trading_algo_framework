package portfolio

import (
	"sort"
	"sync"

	"github.com/marcuspeh/go-tools/util"
	"github.com/marcuspeh/trading-framework/order"
)

// Backtest is the simulated Portfolio for backtest mode. Safe for
// concurrent use.
type Backtest struct {
	mu        sync.RWMutex
	cash      float64
	positions map[string]order.Position
	prices    map[string]float64
}

func NewBacktest(startingCash float64) *Backtest {
	return &Backtest{
		cash:      startingCash,
		positions: map[string]order.Position{},
		prices:    map[string]float64{},
	}
}

func (p *Backtest) Cash() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cash
}

func (p *Backtest) Position(symbol string) order.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pos, ok := p.positions[symbol]
	if !ok {
		return order.Position{Symbol: symbol}
	}
	return pos
}

func (p *Backtest) Positions() []order.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	all := make([]order.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		all = append(all, pos)
	}
	out := util.Filter(all, func(pos order.Position, _ int) bool {
		return pos.Quantity != 0
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

// Value returns cash + sum of (quantity * last mark price). Positions
// without a mark price contribute zero.
func (p *Backtest) Value() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	total := p.cash
	for _, pos := range p.positions {
		px, ok := p.prices[pos.Symbol]
		if !ok {
			continue
		}
		total += pos.Quantity * px
	}
	return total
}

// ApplyFill mutates portfolio state for a single fill. quantity is always
// positive.
func (p *Backtest) ApplyFill(symbol string, side order.Side, quantity, price float64) {
	if quantity <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	notional := quantity * price
	pos := p.positions[symbol]
	pos.Symbol = symbol

	switch {
	case side == order.Buy:
		if pos.Quantity < 0 {
			// Covering a short.
			cover := quantity
			if cover > -pos.Quantity {
				cover = -pos.Quantity
			}
			pos.Quantity += cover
			quantity -= cover
			if pos.Quantity == 0 {
				delete(p.positions, symbol)
			} else {
				p.positions[symbol] = pos
			}
			if quantity == 0 {
				p.prices[symbol] = price
				return
			}
			pos = order.Position{Symbol: symbol, Quantity: quantity, AvgPrice: price}
			p.cash -= quantity * price
			p.positions[symbol] = pos
		} else {
			newQty := pos.Quantity + quantity
			pos.AvgPrice = (pos.Quantity*pos.AvgPrice + quantity*price) / newQty
			pos.Quantity = newQty
			p.cash -= notional
			p.positions[symbol] = pos
		}

	case side == order.Sell:
		if pos.Quantity > 0 {
			close := quantity
			if close > pos.Quantity {
				close = pos.Quantity
			}
			pos.Quantity -= close
			p.cash += close * price
			quantity -= close
			if pos.Quantity == 0 {
				delete(p.positions, symbol)
			} else {
				p.positions[symbol] = pos
			}
			if quantity == 0 {
				p.prices[symbol] = price
				return
			}
			pos = order.Position{Symbol: symbol, Quantity: -quantity, AvgPrice: price}
			p.cash += quantity * price
			p.positions[symbol] = pos
		} else {
			newQty := pos.Quantity - quantity
			totalCost := (-pos.Quantity)*pos.AvgPrice + quantity*price
			if newQty != 0 {
				pos.AvgPrice = totalCost / (-newQty)
			}
			pos.Quantity = newQty
			p.cash += notional
			p.positions[symbol] = pos
		}
	}

	p.prices[symbol] = price
}

// MarkPrice sets the latest mark price for a symbol.
func (p *Backtest) MarkPrice(symbol string, price float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prices[symbol] = price
}
