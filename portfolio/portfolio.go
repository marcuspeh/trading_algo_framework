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

// NYSEWeekdays is the default IsTradingDay predicate. It returns true
// for any weekday that is not an observed NYSE market holiday.
//
// Strategies that need a different exchange calendar should supply their
// own TradingDayFunc to the App.
func NYSEWeekdays(date time.Time) bool {
	return NYSETradingDay(date)
}
