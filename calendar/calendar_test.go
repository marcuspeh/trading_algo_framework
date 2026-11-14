package calendar_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestTradingDay_RegularWeekdays(t *testing.T) {
	// 2024-01-03 is a Wednesday, a normal trading day.
	if !calendar.TradingDay(date(2024, time.January, 3)) {
		t.Fatal("expected 2024-01-03 to be a trading day")
	}
	if calendar.TradingDay(date(2024, time.January, 6)) {
		t.Fatal("expected Saturday to be closed")
	}
	if calendar.TradingDay(date(2024, time.January, 7)) {
		t.Fatal("expected Sunday to be closed")
	}
}

func TestTradingDay_Holidays2024(t *testing.T) {
	closed := []time.Time{
		date(2024, time.January, 1),
		date(2024, time.January, 15),
		date(2024, time.February, 19),
		date(2024, time.March, 29),
		date(2024, time.May, 27),
		date(2024, time.June, 19),
		date(2024, time.July, 4),
		date(2024, time.September, 2),
		date(2024, time.November, 28),
		date(2024, time.December, 25),
	}
	for _, d := range closed {
		if calendar.TradingDay(d) {
			t.Errorf("expected %s to be closed", d.Format("2006-01-02"))
		}
	}
}

// The NYSE published its 2026 calendar: ten full closures and two early
// closes. The full-closure list here must match it exactly, because a
// job scheduled on a holiday would otherwise fire against a market that
// is shut.
func TestTradingDay_MatchesPublishedNYSE2026Calendar(t *testing.T) {
	// NYSE 2026 full closures.
	closed := []string{
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
	}
	for _, s := range closed {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		if calendar.TradingDay(d) {
			t.Errorf("expected %s to be closed", s)
		}
	}

	// Every other weekday in 2026 is open.
	for d := date(2026, time.January, 1); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
		if isWeekend(d) || calendar.TradingDay(d) {
			continue
		}
		if s := d.Format("2006-01-02"); !contains(closed, s) {
			t.Errorf("unexpected closure on %s", s)
		}
	}
}

// isWeekend reports whether the date falls on a Saturday or Sunday.
func isWeekend(d time.Time) bool {
	wd := d.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// The NYSE 2026 early closes are November 27 (the Friday after
// Thanksgiving) and December 24 (Christmas Eve, a Thursday).
func TestEarlyClose_MatchesPublishedNYSE2026Calendar(t *testing.T) {
	early := []string{
		"2026-11-27",
		"2026-12-24",
	}
	for _, s := range early {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		if !calendar.EarlyClose(d) {
			t.Errorf("expected %s to be an early close", s)
		}
	}

	// No other weekday in 2026 closes early.
	for d := date(2026, time.January, 1); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
		if !calendar.TradingDay(d) {
			continue
		}
		if !calendar.EarlyClose(d) {
			continue
		}
		if s := d.Format("2006-01-02"); !contains(early, s) {
			t.Errorf("unexpected early close on %s", s)
		}
	}
}

// The published NYSE early-close calendars for 2025, 2026, 2027 and 2028.
// Source: NYSE Group holiday and early closings calendar.
func TestEarlyClose_MatchesPublishedNYSECalendars(t *testing.T) {
	cases := map[string][]string{
		"2025": {"2025-07-03", "2025-11-28", "2025-12-24"},
		"2026": {"2026-11-27", "2026-12-24"},
		"2027": {"2027-11-26"},
		"2028": {"2028-07-03", "2028-11-24"},
	}
	for year, want := range cases {
		parsed, err := time.Parse("2006", year)
		if err != nil {
			t.Fatal(err)
		}
		y := parsed.Year()
		for d := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC); d.Year() == y; d = d.AddDate(0, 0, 1) {
			if !calendar.TradingDay(d) {
				continue
			}
			got := calendar.EarlyClose(d)
			wantEarly := contains(want, d.Format("2006-01-02"))
			if got != wantEarly {
				t.Errorf("%s: early close=%v want %v", d.Format("2006-01-02"), got, wantEarly)
			}
		}
	}
}

// New Year's Eve is not an NYSE equity early close. The 1:00 p.m. close
// on December 31st belongs to the fixed-income calendar; the equity
// session runs to 4:00 p.m.
func TestEarlyClose_NewYearsEveIsAFullSession(t *testing.T) {
	for _, s := range []string{
		"2024-12-31", // Tuesday
		"2025-12-31", // Wednesday
		"2026-12-31", // Thursday
	} {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		if !calendar.TradingDay(d) {
			t.Errorf("expected %s to be a trading day", s)
			continue
		}
		if calendar.EarlyClose(d) {
			t.Errorf("expected %s to be a full session, not an early close", s)
		}
	}

	// 2027-12-31 is a Friday, but New Year's Day 2028 falls on a
	// Saturday, so the holiday is observed on the preceding Friday and
	// the exchange is closed outright.
	d, err := time.Parse("2006-01-02", "2027-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.TradingDay(d) {
		t.Error("expected 2027-12-31 to be closed (New Year observed)")
	}
	if calendar.EarlyClose(d) {
		t.Error("expected 2027-12-31 not to be an early close")
	}
}

// When July 4th falls on a weekend the holiday is observed on an
// adjacent weekday, so there is no early close that year.
func TestEarlyClose_JulyWhenHolidayIsObserved(t *testing.T) {
	// 2026-07-04 is a Saturday, so the holiday is observed Friday
	// July 3rd, which is a full closure.
	if calendar.TradingDay(date(2026, time.July, 3)) {
		t.Error("expected 2026-07-03 to be closed (Independence Day observed)")
	}
	if calendar.EarlyClose(date(2026, time.July, 3)) {
		t.Error("expected 2026-07-03 not to be an early close")
	}

	// 2027-07-04 is a Sunday, so the holiday is observed Monday
	// July 5th. July 3rd is a Saturday, so there is no early close.
	if calendar.TradingDay(date(2027, time.July, 3)) {
		t.Error("expected 2027-07-03 to be closed (Saturday)")
	}
	if calendar.EarlyClose(date(2027, time.July, 3)) {
		t.Error("expected 2027-07-03 not to be an early close")
	}

	// 2024-07-04 is a Thursday, so July 3rd closes early.
	if !calendar.EarlyClose(date(2024, time.July, 3)) {
		t.Error("expected 2024-07-03 to be an early close")
	}
}

// When December 25th falls on a Saturday, December 24th is the observed
// holiday - a full closure, not an early close.
func TestEarlyClose_ChristmasEveWhenObserved(t *testing.T) {
	// 2027-12-25 is a Saturday, so December 24th is the observed
	// Christmas holiday.
	if calendar.TradingDay(date(2027, time.December, 24)) {
		t.Error("expected 2027-12-24 to be closed (Christmas observed)")
	}
	if calendar.EarlyClose(date(2027, time.December, 24)) {
		t.Error("expected 2027-12-24 not to be an early close")
	}

	// 2026-12-24 is an ordinary Thursday before Christmas.
	if !calendar.TradingDay(date(2026, time.December, 24)) {
		t.Error("expected 2026-12-24 to be a trading day")
	}
	if !calendar.EarlyClose(date(2026, time.December, 24)) {
		t.Error("expected 2026-12-24 to be an early close")
	}
}

// The Friday after Thanksgiving always closes early.
func TestEarlyClose_DayAfterThanksgiving(t *testing.T) {
	cases := map[string]string{
		"2025-11-28": "2025-11-27", // Thanksgiving
		"2026-11-27": "2026-11-26",
		"2027-11-26": "2027-11-25",
	}
	for early, thanksgiving := range cases {
		d, err := time.Parse("2006-01-02", early)
		if err != nil {
			t.Fatal(err)
		}
		if !calendar.EarlyClose(d) {
			t.Errorf("expected %s to be an early close", early)
		}
		th, err := time.Parse("2006-01-02", thanksgiving)
		if err != nil {
			t.Fatal(err)
		}
		if calendar.EarlyClose(th) {
			t.Errorf("expected %s (Thanksgiving) to be closed, not an early close", thanksgiving)
		}
	}
}

func TestSessionOpenAndClose(t *testing.T) {
	open := calendar.SessionOpen(date(2026, time.January, 2))
	if open.Hour() != 9 || open.Minute() != 30 {
		t.Errorf("open=%v want 09:30 ET", open)
	}
	if open.Location().String() != "America/New_York" {
		t.Errorf("open location=%s", open.Location())
	}

	// A normal session closes at 16:00.
	close := calendar.SessionClose(date(2026, time.January, 2))
	if close.Hour() != 16 || close.Minute() != 0 {
		t.Errorf("close=%v want 16:00 ET", close)
	}

	// An early-close session closes at 13:00.
	early := calendar.SessionClose(date(2026, time.November, 27))
	if early.Hour() != 13 || early.Minute() != 0 {
		t.Errorf("early close=%v want 13:00 ET", early)
	}
}

func TestPreviousAndNextTradingDay(t *testing.T) {
	// Friday 2026-01-02 back to Thursday 2026-01-01, which is a holiday.
	prev := calendar.PreviousTradingDay(date(2026, time.January, 2))
	if prev.Format("2006-01-02") != "2025-12-31" {
		t.Errorf("previous=%s want 2025-12-31", prev.Format("2006-01-02"))
	}

	// Monday 2026-01-05 forward to Tuesday, skipping the holiday.
	next := calendar.NextTradingDay(date(2026, time.January, 5))
	if next.Format("2006-01-02") != "2026-01-06" {
		t.Errorf("next=%s want 2026-01-06", next.Format("2006-01-02"))
	}

	// The time of day is preserved.
	if h := calendar.PreviousTradingDay(date(2026, time.January, 2).Add(9*time.Hour)).Hour(); h != 9 {
		t.Errorf("hour=%d want 9", h)
	}
}
