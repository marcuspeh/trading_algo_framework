package portfolio

import (
	"sort"
	"sync"

	"github.com/marcuspeh/trading-framework/order"
)

// Live is the read-only mirror Portfolio for live mode. The reconciler
// overwrites its state with broker truth on every reconciliation.
type Live struct {
	mu        sync.RWMutex
	positions map[string]order.Position
	cash      float64
	nav       float64
	hasNav    bool
}

func NewLive() *Live {
	return &Live{positions: map[string]order.Position{}}
}

// Sync overwrites the mirror with broker-reported positions and NAV.
func (p *Live) Sync(positions []order.Position, nav float64, hasNav bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positions = map[string]order.Position{}
	p.cash = 0
	for _, pos := range positions {
		if pos.Quantity == 0 {
			continue
		}
		p.positions[pos.Symbol] = pos
	}
	p.nav = nav
	p.hasNav = hasNav
}

func (p *Live) Cash() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cash
}

func (p *Live) Position(symbol string) order.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pos, ok := p.positions[symbol]
	if !ok {
		return order.Position{Symbol: symbol}
	}
	return pos
}

func (p *Live) Positions() []order.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]order.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		out = append(out, pos)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

// Value returns the broker-reported NAV when available; otherwise falls
// back to cash + sum of position cost basis.
func (p *Live) Value() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.hasNav {
		return p.nav
	}
	total := p.cash
	for _, pos := range p.positions {
		total += pos.Quantity * pos.AvgPrice
	}
	return total
}
