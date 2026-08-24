package tradingframework

import "time"

// NYSEWeekdays is a minimal IsTradingDay helper that returns true for any
// weekday. It ignores US market holidays. Strategies that need a proper
// exchange calendar should supply their own TradingDayFunc to the App.
//
// Usage:
//
//	app := tradingframework.NewApp(
//	    tradingframework.WithTradingDayFunc(tradingframework.NYSEWeekdays),
//	)
func NYSEWeekdays(date time.Time) bool {
	d := date.Weekday()
	return d != time.Saturday && d != time.Sunday
}

// TradingDayFunc is the signature of an exchange-calendar predicate.
type TradingDayFunc func(date time.Time) bool
