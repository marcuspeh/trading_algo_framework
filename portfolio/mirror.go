package portfolio

import (
	"sort"
	"sync"

	tf "github.com/marcuspeh/trading-framework"
)

// MirrorPortfolio is a read-only mirror of broker state. The broker is
// the source of truth; Sync overwrites local state on every reconciliation.
type MirrorPortfolio struct {
	mu        sync.RWMutex
	positions map[string]tf.Position
	cash      float64
	nav       float64
	hasNav    bool
}

func NewMirror() *MirrorPortfolio {
	return &MirrorPortfolio{
		positions: map[string]tf.Position{},
	}
}

func (p *MirrorPortfolio) Sync(positions []tf.Position, nav float64, hasNav bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positions = map[string]tf.Position{}
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

// Cash returns the local cash view (typically 0 in live mode since
// Value() reports broker NAV).
func (p *MirrorPortfolio) Cash() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cash
}

func (p *MirrorPortfolio) Position(symbol string) tf.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pos, ok := p.positions[symbol]
	if !ok {
		return tf.Position{Symbol: symbol}
	}
	return pos
}

func (p *MirrorPortfolio) Positions() []tf.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]tf.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		out = append(out, pos)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

// Value returns the broker-reported NAV when available; otherwise falls
// back to cash + sum of position cost basis.
func (p *MirrorPortfolio) Value() float64 {
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
