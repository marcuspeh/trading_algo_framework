package portfolio

import (
	"sync"
	"time"
)

// NYSE holiday calendar.
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

// nyseHolidayCache caches the computed holiday set per year.
var nyseHolidayCache sync.Map // map[int]map[string]struct{}

// NYSETradingDay reports whether the NYSE is open on the given date.
// Weekends and observed market holidays are closed.
//
// The holiday set is computed per year and cached, so repeated calls
// across a backtest are cheap.
func NYSETradingDay(date time.Time) bool {
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
