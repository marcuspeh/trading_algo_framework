package portfolio

import (
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
)

// NYSE holiday calendar.
//
// The rules live in the calendar package, which is the framework's
// single source of truth for trading sessions. These wrappers keep the
// historical portfolio API working.

// NYSETradingDay reports whether the NYSE is open on the given date.
// Weekends and observed market holidays are closed.
func NYSETradingDay(date time.Time) bool {
	return calendar.TradingDay(date)
}

// NYSEEarlyClose reports whether the NYSE ends the session early on the
// given date, closing at 13:00 ET instead of 16:00.
func NYSEEarlyClose(date time.Time) bool {
	return calendar.EarlyClose(date)
}
