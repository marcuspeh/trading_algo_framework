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
	hasCash   bool
	nav       float64
	hasNav    bool
}

// LiveSnapshot is the broker's view of the account.
//
// Cash is separate from NAV because the broker reports a net asset
// value but not the cash in it. The reconciler derives cash from the
// NAV and the position marks, and says so explicitly when it cannot:
// a cash figure that is unknown is not the same as a cash figure of
// zero.
type LiveSnapshot struct {
	// Positions are the broker's positions.
	Positions []order.Position

	// NAV is the account's net asset value.
	NAV float64

	// HasNAV reports whether the broker gave a NAV.
	HasNAV bool

	// Cash is the derived cash balance.
	Cash float64

	// HasCash reports whether cash could be derived.
	HasCash bool
}

func NewLive() *Live {
	return &Live{positions: map[string]order.Position{}}
}

// Sync overwrites the mirror with a broker snapshot.
func (p *Live) Sync(snapshot LiveSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positions = map[string]order.Position{}
	for _, pos := range snapshot.Positions {
		if pos.Quantity == 0 {
			continue
		}
		p.positions[pos.Symbol] = pos
	}
	p.nav = snapshot.NAV
	p.hasNav = snapshot.HasNAV
	p.cash = snapshot.Cash
	p.hasCash = snapshot.HasCash
}

// Cash returns the derived cash balance. It is zero when the cash
// could not be derived.
func (p *Live) Cash() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cash
}

// CashKnown reports whether the cash balance could be derived.
func (p *Live) CashKnown() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.hasCash
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

// Value returns the broker-reported NAV when available.
//
// Without a NAV the value falls back to the cash plus the cost basis
// of the positions, which is only meaningful when the cash is known.
func (p *Live) Value() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.hasNav {
		return p.nav
	}
	if !p.hasCash {
		return 0
	}
	total := p.cash
	for _, pos := range p.positions {
		total += pos.Quantity * pos.AvgPrice
	}
	return total
}
