package tradingframework

import "time"

// NYSEWeekdays is a minimal IsTradingDay predicate that returns true for
// any weekday. It ignores US market holidays; strategies that need a real
// exchange calendar should supply their own TradingDayFunc to the App.
func NYSEWeekdays(date time.Time) bool {
	d := date.Weekday()
	return d != time.Saturday && d != time.Sunday
}

type TradingDayFunc func(date time.Time) bool
