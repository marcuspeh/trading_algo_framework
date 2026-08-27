package execution

import (
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// DefaultFill is the MVP deterministic fill model.
//
//   - Market orders fill at the bar's Close price for the order date.
//   - Limit buys fill if Low <= Limit (price = Limit).
//   - Limit sells fill if High >= Limit (price = Limit).
//   - No slippage, no transaction cost.
type DefaultFill struct {
	Bars BarProvider
}

func NewDefaultFill(bars BarProvider) *DefaultFill {
	return &DefaultFill{Bars: bars}
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
		return bar.Close, true, true
	case order.Limit:
		if o.Limit == nil {
			return 0, false, true
		}
		limit := *o.Limit
		switch o.Side {
		case order.Buy:
			if bar.Low <= limit {
				return limit, true, true
			}
		case order.Sell:
			if bar.High >= limit {
				return limit, true, true
			}
		}
		return 0, false, true
	default:
		return 0, false, true
	}
}
