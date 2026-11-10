package execution

import (
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// DefaultFill is the deterministic fill model.
//
//   - Market orders fill at the bar's Close price for the order date.
//   - Limit buys fill if Low <= Limit (price = Limit).
//   - Limit sells fill if High >= Limit (price = Limit).
//   - Slippage is applied against the trader, then commission is charged
//     on the resulting notional.
//
// The zero value is a frictionless fill, matching the historical
// behaviour. Set SlippageBps and Commission to model real trading costs.
type DefaultFill struct {
	Bars BarProvider

	// SlippageBps is the adverse price movement applied to every fill,
	// in basis points of the fill price (1 bp = 0.01%). 5 means fills
	// happen 0.05% worse than the reference price.
	SlippageBps float64

	// Commission is charged per fill, in account currency, regardless of
	// size. Set CommissionPerShare to model per-share fees instead.
	Commission float64

	// CommissionPerShare is charged per unit of quantity.
	CommissionPerShare float64
}

func NewDefaultFill(bars BarProvider) *DefaultFill {
	return &DefaultFill{Bars: bars}
}

// NewFillWithCosts builds a fill model with slippage and commission.
func NewFillWithCosts(bars BarProvider, slippageBps, commission float64) *DefaultFill {
	return &DefaultFill{Bars: bars, SlippageBps: slippageBps, Commission: commission}
}

func (d *DefaultFill) Fill(o order.Order, asOf time.Time) (float64, bool, bool) {
	if d.Bars == nil {
		return 0, false, false
	}
	bar, ok := d.Bars.Bar(o.Symbol, asOf)
	if !ok {
		return 0, false, false
	}
	switch o.Type {
	case order.Market:
		return d.applySlippage(bar.Close, o.Side), true, true
	case order.Limit:
		if o.Limit == nil {
			return 0, false, true
		}
		limit := *o.Limit
		switch o.Side {
		case order.Buy:
			if bar.Low <= limit {
				// A limit buy never fills above its limit; slippage is
				// capped by the limit price.
				price := d.applySlippage(limit, o.Side)
				if price > limit {
					price = limit
				}
				return price, true, true
			}
		case order.Sell:
			if bar.High >= limit {
				// A limit sell never fills below its limit.
				price := d.applySlippage(limit, o.Side)
				if price < limit {
					price = limit
				}
				return price, true, true
			}
		}
		return 0, false, true
	default:
		return 0, false, true
	}
}

// applySlippage moves the reference price against the trader.
func (d *DefaultFill) applySlippage(price float64, side order.Side) float64 {
	if d.SlippageBps == 0 || price == 0 {
		return price
	}
	adj := price * d.SlippageBps / 10_000
	if side == order.Buy {
		return price + adj
	}
	return price - adj
}

// CommissionFor returns the total commission charged for a fill.
func (d *DefaultFill) CommissionFor(quantity float64) float64 {
	return d.Commission + d.CommissionPerShare*quantity
}
