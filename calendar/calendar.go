// Package calendar computes NYSE trading sessions.
//
// Every date the framework reasons about is anchored on the NYSE
// calendar: which days the exchange trades, which days it closes
// early, and when each session opens and closes.
//
// The package is a leaf - it depends on nothing in the framework, so
// clock, clients, scheduler and portfolio can all use it without
// creating an import cycle.
package calendar

import (
	"sync"
	"time"

	// Embed the IANA database so the NYSE location resolves even on
	// hosts without a system zoneinfo directory (scratch containers,
	// minimal CI images).
	_ "time/tzdata"
)

// NYSE is the location every session time is expressed in.
var NYSE = mustLoadNYSE()

func mustLoadNYSE() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// time/tzdata is embedded above, so this is unreachable.
		panic("calendar: America/New_York unavailable: " + err.Error())
	}
	return loc
}

// Session times, in ET.
const (
	// OpenHour and OpenMinute are the regular equity session open.
	OpenHour   = 9
	OpenMinute = 30

	// CloseHour and CloseMinute are the regular equity session close.
	CloseHour   = 16
	CloseMinute = 0

	// EarlyCloseHour and EarlyCloseMinute are the close on a day the
	// exchange ends the session early.
	EarlyCloseHour   = 13
	EarlyCloseMinute = 0
)

// TradingDayFunc is the signature of an exchange-calendar predicate.
type TradingDayFunc func(date time.Time) bool

// SessionOpen returns the open of the session on the given date, in ET.
//
// The date is taken as written rather than converted, so a caller that
// means "the session of 2024-01-02" gets that day regardless of the
// zone its time was expressed in.
func SessionOpen(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, OpenHour, OpenMinute, 0, 0, NYSE)
}

// SessionClose returns the close of the session on the given date, in ET:
// 13:00 on an early-close day, 16:00 otherwise.
//
// The date is taken as written rather than converted.
func SessionClose(date time.Time) time.Time {
	y, m, d := date.Date()
	hour, minute := CloseHour, CloseMinute
	if EarlyClose(date) {
		hour, minute = EarlyCloseHour, EarlyCloseMinute
	}
	return time.Date(y, m, d, hour, minute, 0, 0, NYSE)
}

// EarlyClose reports whether the NYSE ends the session early on the
// given date, closing at 13:00 ET instead of 16:00.
//
// The exchange closes early at 1:00 p.m. ET on:
//
//   - the Friday after Thanksgiving;
//   - Christmas Eve, December 24, when it is a trading day;
//   - July 3rd, when July 4th falls on a Tuesday, Wednesday, Thursday
//     or Friday.
//
// The last rule has a subtlety worth spelling out. When July 4th falls
// on a Saturday the holiday is observed the preceding Friday, so July
// 3rd is a full closure rather than an early close; when July 4th falls
// on a Sunday the holiday is observed the following Monday and July
// 3rd is an ordinary trading day. The same holds for Christmas: when
// December 25th falls on a Saturday, December 24th is the observed
// holiday, so it is closed outright rather than closing early.
//
// New Year's Eve is deliberately not an early close. The 1:00 p.m.
// close on December 31rd belongs to the fixed-income calendar; the
// NYSE equity session runs to 4:00 p.m.
func EarlyClose(date time.Time) bool {
	switch date.Month() {
	case time.July:
		return date.Day() == 3 && july4IsWeekday(date.Year())
	case time.November:
		return sameDate(date, fridayAfterThanksgiving(date.Year()))
	case time.December:
		return date.Day() == 24 && TradingDay(date)
	}
	return false
}

// PreviousTradingDay returns the most recent trading day strictly
// before the given date.
//
// The result carries the same time of day as the input, so a caller can
// step a timestamp back a session without disturbing its clock time.
func PreviousTradingDay(date time.Time) time.Time {
	return stepTradingDay(date, -1)
}

// NextTradingDay returns the next trading day strictly after the given
// date, keeping the same time of day.
func NextTradingDay(date time.Time) time.Time {
	return stepTradingDay(date, 1)
}

// stepTradingDay walks the calendar day by day in the given direction
// until it finds a trading day.
func stepTradingDay(date time.Time, step int) time.Time {
	candidate := date
	for i := 0; i < 14; i++ {
		candidate = candidate.AddDate(0, 0, step)
		if TradingDay(candidate) {
			return candidate
		}
	}
	// Unreachable: the NYSE never closes for more than a week.
	return candidate
}

// sameDate reports whether two times fall on the same calendar date.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// fridayAfterThanksgiving returns the Friday following Thanksgiving in
// the given year.
func fridayAfterThanksgiving(year int) time.Time {
	thanksgiving := nthWeekday(year, time.November, time.Thursday, 4)
	return thanksgiving.AddDate(0, 0, 1)
}

// july4IsWeekday reports whether July 4th in the given year falls on a
// weekday that leaves a trading day on July 3rd to close early.
//
// July 4th on a Monday leaves July 3rd on a Sunday, and July 4th on a
// weekend is observed on an adjacent weekday that closes the exchange
// outright, so neither produces an early close.
func july4IsWeekday(year int) bool {
	july4 := time.Date(year, time.July, 4, 0, 0, 0, 0, time.UTC)
	switch july4.Weekday() {
	case time.Tuesday, time.Wednesday, time.Thursday, time.Friday:
		return true
	}
	return false
}

// nyseHolidayCache caches the computed holiday set per year.
var nyseHolidayCache sync.Map // map[int]map[string]struct{}

// TradingDay reports whether the NYSE is open on the given date.
// Weekends and observed market holidays are closed.
//
// The holiday set is computed per year and cached, so repeated calls
// across a backtest are cheap.
func TradingDay(date time.Time) bool {
	if d := date.Weekday(); d == time.Saturday || d == time.Sunday {
		return false
	}
	year := date.Year()
	holidays, ok := nyseHolidayCache.Load(year)
	if !ok {
		computed := nyseHolidays(year)
		nyseHolidayCache.Store(year, computed)
		holidays = computed
	}
	_, closed := holidays.(map[string]struct{})[date.Format("2006-01-02")]
	return !closed
}

// nyseHolidays returns the set of closed dates for the given year, in
// "2006-01-02" form.
//
// The exchange is closed on New Year's Day, Martin Luther King Jr.
// Day, Washington's Birthday, Good Friday, Memorial Day, Juneteenth,
// Independence Day, Labor Day, Thanksgiving and Christmas Day.
//
// Weekend rule: when a holiday falls on a Saturday the exchange is
// closed on the preceding Friday; when it falls on a Sunday the
// exchange is closed on the following Monday. New Year's Day is
// special-cased, because a Saturday January 1 is observed on December
// 31 of the previous year.
//
// The rules are implemented arithmetically so any year is covered
// without a data table.
func nyseHolidays(year int) map[string]struct{} {
	out := map[string]struct{}{}

	// Fixed-date holidays. Saturday holidays are observed the preceding
	// Friday, Sunday holidays the following Monday.
	for _, h := range []struct {
		month time.Month
		day   int
	}{
		{time.January, 1},   // New Year's Day
		{time.June, 19},     // Juneteenth
		{time.July, 4},      // Independence Day
		{time.December, 25}, // Christmas Day
	} {
		markObserved(out, time.Date(year, h.month, h.day, 0, 0, 0, 0, time.UTC))
	}

	// New Year's Day of the following year may be observed in this one,
	// when January 1 falls on a Saturday.
	nextJan1 := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)
	if nextJan1.Weekday() == time.Saturday {
		out[nextJan1.AddDate(0, 0, -1).Format("2006-01-02")] = struct{}{}
	}

	// Floating holidays.
	markObserved(out, nthWeekday(year, time.January, time.Monday, 3))    // MLK Day
	markObserved(out, nthWeekday(year, time.February, time.Monday, 3))   // Washington's Birthday
	markObserved(out, goodFriday(year))                                  // Good Friday
	markObserved(out, lastWeekday(year, time.May, time.Monday))          // Memorial Day
	markObserved(out, nthWeekday(year, time.September, time.Monday, 1))  // Labor Day
	markObserved(out, nthWeekday(year, time.November, time.Thursday, 4)) // Thanksgiving

	return out
}

// markObserved adds the observed date for a holiday to the closed set.
func markObserved(out map[string]struct{}, d time.Time) {
	switch d.Weekday() {
	case time.Saturday:
		d = d.AddDate(0, 0, -1)
	case time.Sunday:
		d = d.AddDate(0, 0, 1)
	}
	out[d.Format("2006-01-02")] = struct{}{}
}

// nthWeekday returns the n-th given weekday of a month, 1-based.
func nthWeekday(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+7*(n-1))
}

// lastWeekday returns the last given weekday of a month.
func lastWeekday(year int, month time.Month, weekday time.Weekday) time.Time {
	// First day of the next month, minus one day, is the last day.
	next := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	back := (int(next.Weekday()) - int(weekday) + 7) % 7
	return next.AddDate(0, 0, -back)
}

// goodFriday returns the Good Friday date for a year using the
// anonymous Gregorian computus.
func goodFriday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1

	easter := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return easter.AddDate(0, 0, -2)
}
