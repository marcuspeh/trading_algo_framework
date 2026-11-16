// Package published is a calendar.Provider backed by the published
// NYSE holiday and early-close calendars.
//
// It exists for the framework's own tests and examples, and for a
// strategy that needs a calendar without a running market-data-service.
// The dates below are the exchange's published schedules - there are no
// rules here, only the dates the NYSE published.
//
// A live strategy should use calendar.NewService instead, so it
// trades on the exchange's current schedule rather than a copy of one
// that was published years ago.
//
// The provider covers the years it has data for and refuses the ones
// it does not, so a test that wanders outside the published range
// fails loudly instead of being answered by a guess.
package published

import (
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
)

// Session times, in ET. The NYSE equity session opens at 09:30 and
// closes at 16:00, or 13:00 on a day it closes early.
const (
	openHour         = 9
	openMinute       = 30
	closeHour        = 16
	closeMinute      = 0
	earlyCloseHour   = 13
	earlyCloseMinute = 0
)

// holidays lists the full closures, by year.
//
// Source: the exchange calendar the market-data-service serves, which
// is the NYSE's published schedule.
var holidays = map[int][]string{
	2023: {
		"2023-01-02", // New Year's Day observed (January 1 is a Sunday)
		"2023-01-16", // MLK Day
		"2023-02-20", // Washington's Birthday
		"2023-04-07", // Good Friday
		"2023-05-29", // Memorial Day
		"2023-06-19", // Juneteenth
		"2023-07-04", // Independence Day
		"2023-09-04", // Labor Day
		"2023-11-23", // Thanksgiving
		"2023-12-25", // Christmas Day
	},
	2024: {
		"2024-01-01", // New Year's Day
		"2024-01-15", // MLK Day
		"2024-02-19", // Washington's Birthday
		"2024-03-29", // Good Friday
		"2024-05-27", // Memorial Day
		"2024-06-19", // Juneteenth
		"2024-07-04", // Independence Day
		"2024-09-02", // Labor Day
		"2024-11-28", // Thanksgiving
		"2024-12-25", // Christmas Day
	},
	2025: {
		"2025-01-01", // New Year's Day
		"2025-01-09", // National day of mourning for President Carter
		"2025-01-20", // MLK Day
		"2025-02-17", // Washington's Birthday
		"2025-04-18", // Good Friday
		"2025-05-26", // Memorial Day
		"2025-06-19", // Juneteenth
		"2025-07-04", // Independence Day
		"2025-09-01", // Labor Day
		"2025-11-27", // Thanksgiving
		"2025-12-25", // Christmas Day
	},
	2026: {
		"2026-01-01", // New Year's Day
		"2026-01-19", // MLK Day
		"2026-02-16", // Washington's Birthday
		"2026-04-03", // Good Friday
		"2026-05-25", // Memorial Day
		"2026-06-19", // Juneteenth
		"2026-07-03", // Independence Day observed (July 4 is a Saturday)
		"2026-09-07", // Labor Day
		"2026-11-26", // Thanksgiving
		"2026-12-25", // Christmas Day
	},
	2027: {
		"2027-01-01", // New Year's Day
		"2027-01-18", // MLK Day
		"2027-02-15", // Washington's Birthday
		"2027-03-26", // Good Friday
		"2027-05-31", // Memorial Day
		"2027-06-18", // Juneteenth observed (June 19 is a Saturday)
		"2027-07-05", // Independence Day observed (July 4 is a Sunday)
		"2027-09-06", // Labor Day
		"2027-11-25", // Thanksgiving
		"2027-12-24", // Christmas Day observed (December 25 is a Saturday)
	},
	2028: {
		"2028-01-17", // MLK Day
		"2028-02-21", // Washington's Birthday
		"2028-04-14", // Good Friday
		"2028-05-29", // Memorial Day
		"2028-06-19", // Juneteenth
		"2028-07-04", // Independence Day
		"2028-09-04", // Labor Day
		"2028-11-23", // Thanksgiving
		"2028-12-25", // Christmas Day
	},
	2029: {
		"2029-01-01", // New Year's Day
		"2029-01-15", // MLK Day
		"2029-02-19", // Washington's Birthday
		"2029-03-30", // Good Friday
		"2029-05-28", // Memorial Day
		"2029-06-19", // Juneteenth
		"2029-07-04", // Independence Day
		"2029-09-03", // Labor Day
		"2029-11-22", // Thanksgiving
		"2029-12-25", // Christmas Day
	},
}

// earlyCloses lists the days the session ended at 13:00 ET.
//
// Source: the NYSE holiday and early closings calendars.
var earlyCloses = map[int][]string{
	2023: {
		"2023-07-03", // Day before Independence Day
		"2023-11-24", // Friday after Thanksgiving
	},
	2024: {
		"2024-07-03", // Day before Independence Day
		"2024-11-29", // Friday after Thanksgiving
		"2024-12-24", // Christmas Eve
	},
	2025: {
		"2025-07-03", // Day before Independence Day
		"2025-11-28", // Friday after Thanksgiving
		"2025-12-24", // Christmas Eve
	},
	2026: {
		"2026-11-27", // Friday after Thanksgiving
		"2026-12-24", // Christmas Eve
	},
	2027: {
		"2027-11-26", // Friday after Thanksgiving
	},
	2028: {
		"2028-07-03", // Day before Independence Day
		"2028-11-24", // Friday after Thanksgiving
	},
	2029: {
		"2029-07-03", // Day before Independence Day
		"2029-11-23", // Friday after Thanksgiving
		"2029-12-24", // Christmas Eve
	},
}

// Provider is a calendar.Provider over the published calendars.
type Provider struct{}

// Compile-time proof that the fake satisfies the provider interface.
var _ calendar.Provider = Provider{}

// New returns a calendar.Provider over the published NYSE calendars.
func New() calendar.Provider { return Provider{} }

// TradingDay reports whether the NYSE held a session on the date.
//
// It panics when the date's year is not covered by the published
// calendars, which is what a test wants: an uncovered date should
// stop the test, not answer a guess.
func TradingDay(date time.Time) bool {
	trading, err := Provider{}.TradingDay(date)
	if err != nil {
		panic(err)
	}
	return trading
}

// EarlyClose reports whether the session on the date was shortened.
func EarlyClose(date time.Time) bool {
	early, err := Provider{}.EarlyClose(date)
	if err != nil {
		panic(err)
	}
	return early
}

// TradingDay reports whether the NYSE held a session on the date.
func (p Provider) TradingDay(date time.Time) (bool, error) {
	if err := p.covered(date); err != nil {
		return false, err
	}
	if isWeekend(date) {
		return false, nil
	}
	return !contains(holidays[date.Year()], date.Format("2006-01-02")), nil
}

// EarlyClose reports whether the session on the date was shortened.
func (p Provider) EarlyClose(date time.Time) (bool, error) {
	if err := p.covered(date); err != nil {
		return false, err
	}
	return contains(earlyCloses[date.Year()], date.Format("2006-01-02")), nil
}

// SessionOpen returns the open of the session on the date.
func (p Provider) SessionOpen(date time.Time) (time.Time, error) {
	trading, err := p.TradingDay(date)
	if err != nil {
		return time.Time{}, err
	}
	if !trading {
		return time.Time{}, fmt.Errorf("%w: %s", calendar.ErrNoSession, date.Format("2006-01-02"))
	}
	return sessionTime(date, openHour, openMinute), nil
}

// SessionClose returns the close of the session on the date.
func (p Provider) SessionClose(date time.Time) (time.Time, error) {
	trading, err := p.TradingDay(date)
	if err != nil {
		return time.Time{}, err
	}
	if !trading {
		return time.Time{}, fmt.Errorf("%w: %s", calendar.ErrNoSession, date.Format("2006-01-02"))
	}
	early, err := p.EarlyClose(date)
	if err != nil {
		return time.Time{}, err
	}
	if early {
		return sessionTime(date, earlyCloseHour, earlyCloseMinute), nil
	}
	return sessionTime(date, closeHour, closeMinute), nil
}

// covered reports whether the provider has a published calendar for the
// date's year.
func (p Provider) covered(date time.Time) error {
	if _, ok := holidays[date.Year()]; ok {
		return nil
	}
	return fmt.Errorf("published: no published NYSE calendar for %d", date.Year())
}

// sessionTime returns the given wall-clock time on the date, in ET.
func sessionTime(date time.Time, hour, minute int) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, hour, minute, 0, 0, calendar.NYSE)
}

// isWeekend reports whether the date falls on a Saturday or Sunday.
func isWeekend(d time.Time) bool {
	wd := d.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// contains reports whether the list holds the given date.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
