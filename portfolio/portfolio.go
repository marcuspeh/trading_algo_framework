// Package portfolio provides the Portfolio interface and its live and
// backtest implementations.
package portfolio

import (
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

type Portfolio interface {
	Cash() float64
	Position(symbol string) order.Position
	Positions() []order.Position
	Value() float64
}

// TradingDayFunc is the signature of an exchange-calendar predicate.
type TradingDayFunc func(date time.Time) bool

// NYSEWeekdays is a minimal IsTradingDay predicate that returns true for
// any weekday. It ignores US market holidays; strategies that need a real
// exchange calendar should supply their own TradingDayFunc to the App.
func NYSEWeekdays(date time.Time) bool {
	d := date.Weekday()
	return d != time.Saturday && d != time.Sunday
}
