// Package portfolio provides Portfolio implementations for backtest and
// live modes.
//
// The Portfolio interface lives in the root trading-framework package.
// This package supplies concrete implementations only.
package portfolio

import (
	"sort"
	"sync"

	"github.com/marcuspeh/go-tools/util"
	tf "github.com/marcuspeh/trading-framework"
)

// MemPortfolio is an in-memory portfolio driven by simulated fills.
// It is the default Portfolio for Backtest mode.
//
// All methods are safe for concurrent use.
type MemPortfolio struct {
	mu        sync.RWMutex
	cash      float64
	positions map[string]tf.Position
	prices    map[string]float64 // last mark price per symbol
}

// NewMem constructs an in-memory portfolio seeded with starting cash.
func NewMem(startingCash float64) *MemPortfolio {
	return &MemPortfolio{
		cash:      startingCash,
		positions: map[string]tf.Position{},
		prices:    map[string]float64{},
	}
}

// Cash returns the current available cash.
func (p *MemPortfolio) Cash() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cash
}

// Position returns the current position in symbol. Zero-valued when
// the symbol has no position.
func (p *MemPortfolio) Position(symbol string) tf.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pos, ok := p.positions[symbol]
	if !ok {
		return tf.Position{Symbol: symbol}
	}
	return pos
}

// Positions returns all open positions sorted by symbol for determinism.
func (p *MemPortfolio) Positions() []tf.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	all := make([]tf.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		all = append(all, pos)
	}
	out := util.Filter(all, func(pos tf.Position, _ int) bool {
		return pos.Quantity != 0
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

// Value returns the total portfolio value: cash + sum of (quantity *
// last mark price). Positions without a mark price contribute zero.
func (p *MemPortfolio) Value() float64 {
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

// ApplyFill mutates portfolio state for a single fill.
//
// side is Buy or Sell. quantity is always positive. price is the fill
// price. The function updates cash and the position, computing a new
// weighted average cost for buys that increase an existing long
// position. Selling reduces quantity (and average cost stays the same).
func (p *MemPortfolio) ApplyFill(symbol string, side tf.Side, quantity, price float64) {
	if quantity <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	notional := quantity * price
	pos := p.positions[symbol]
	pos.Symbol = symbol // ensure symbol is always set

	switch {
	case side == tf.Buy:
		if pos.Quantity < 0 {
			// Covering a short. Reduce short quantity first; any
			// remaining opens a new long.
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
			// Remaining opens a new long.
			pos = tf.Position{
				Symbol:   symbol,
				Quantity: quantity,
				AvgPrice: price,
			}
			p.cash -= quantity * price
			p.positions[symbol] = pos
		} else {
			// Opening or adding to a long position.
			newQty := pos.Quantity + quantity
			pos.AvgPrice = (pos.Quantity*pos.AvgPrice + quantity*price) / newQty
			pos.Quantity = newQty
			p.cash -= notional
			p.positions[symbol] = pos
		}

	case side == tf.Sell:
		if pos.Quantity > 0 {
			// Closing some or all of a long; any remaining opens a short.
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
			// Remaining opens a new short.
			pos = tf.Position{
				Symbol:   symbol,
				Quantity: -quantity,
				AvgPrice: price,
			}
			p.cash += quantity * price
			p.positions[symbol] = pos
		} else {
			// Opening or adding to a short position.
			newQty := pos.Quantity - quantity // more negative
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

// MarkPrice sets the latest mark price for a symbol. Used by the
// backtest engine to compute PortfolioValue() between fills.
func (p *MemPortfolio) MarkPrice(symbol string, price float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prices[symbol] = price
}

// MarkPrices sets mark prices for many symbols at once.
func (p *MemPortfolio) MarkPrices(prices map[string]float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for s, px := range prices {
		p.prices[s] = px
	}
}

// SetCash overrides the current cash balance (used by reconciliation).
func (p *MemPortfolio) SetCash(cash float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cash = cash
}
