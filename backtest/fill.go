package backtest

import (
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

// FillModel decides whether and at what price an order fills in
// backtest mode. The interface exists so the fill assumptions can
// evolve without touching strategy code.
type FillModel interface {
	// Fill returns price, filled, ok. When ok is false the order is
	// ignored (e.g. no bar data on the requested date).
	Fill(order tf.Order, asOf time.Time) (fillPrice float64, filled bool, ok bool)
}

// BarProvider supplies the daily bar for a symbol on a given date.
// Implementations are responsible for returning ErrNotFound (or
// ok=false) when the bar is not available.
type BarProvider interface {
	Bar(symbol string, date time.Time) (tf.Bar, bool)
}

// DefaultFill is the MVP deterministic fill model.
//
// Fill assumptions:
//
//   - Market orders fill at the bar's Close price for the order date.
//   - Limit buy orders fill if Low <= Limit; fill price = Limit.
//   - Limit sell orders fill if High >= Limit; fill price = Limit.
//   - Stop orders (Type other than Market/Limit) do not fill.
//
// There is no slippage and no transaction cost. Strategies that need
// them should implement their own FillModel.
type DefaultFill struct {
	Bars BarProvider
}

// NewDefaultFill returns a DefaultFill backed by the supplied BarProvider.
func NewDefaultFill(bars BarProvider) *DefaultFill {
	return &DefaultFill{Bars: bars}
}

// Fill implements FillModel.
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
