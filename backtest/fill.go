package backtest

import (
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

type FillModel interface {
	Fill(order tf.Order, asOf time.Time) (fillPrice float64, filled bool, ok bool)
}

type BarProvider interface {
	Bar(symbol string, date time.Time) (tf.Bar, bool)
}

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

func (d *DefaultFill) Fill(order tf.Order, asOf time.Time) (float64, bool, bool) {
	if d.Bars == nil {
		return 0, false, false
	}
	bar, ok := d.Bars.Bar(order.Symbol, asOf)
	if !ok {
		return 0, false, false
	}
	switch order.Type {
	case tf.Market:
		return bar.Close, true, true
	case tf.Limit:
		if order.Limit == nil {
			return 0, false, true
		}
		limit := *order.Limit
		switch order.Side {
		case tf.Buy:
			if bar.Low <= limit {
				return limit, true, true
			}
		case tf.Sell:
			if bar.High >= limit {
				return limit, true, true
			}
		}
		return 0, false, true
	default:
		return 0, false, true
	}
}
