package published_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/calendar/published"
)

// date parses a YYYY-MM-DD string.
func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return d
}

// The published NYSE holiday calendars.
//
// Source: the NYSE Group holiday calendar, as served by the
// market-data-service.
var publishedHolidays = map[int][]string{
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

// The published NYSE early-close calendars.
//
// Source: the NYSE Group holiday and early closings calendar.
var publishedEarlyCloses = map[int][]string{
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

// The provider's holidays are exactly the published calendar's, and no
// other day is closed.
func TestProvider_HolidaysMatchThePublishedCalendar(t *testing.T) {
	p := published.New()
	for year, published := range publishedHolidays {
		for _, s := range published {
			d := date(t, s)
			trading, err := p.TradingDay(d)
			if err != nil {
				t.Fatalf("%s: %v", s, err)
			}
			if trading {
				t.Errorf("%s should be closed", s)
			}
		}

		// No other weekday in the year is closed.
		for d := date(t, yearStart(year)); d.Year() == year; d = d.AddDate(0, 0, 1) {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			if contains(published, d.Format("2006-01-02")) {
				continue
			}
			trading, err := p.TradingDay(d)
			if err != nil {
				t.Fatalf("%s: %v", d.Format("2006-01-02"), err)
			}
			if !trading {
				t.Errorf("unexpected closure on %s", d.Format("2006-01-02"))
			}
		}
	}
}

// The provider's early closes are exactly the published calendar's, and
// no other day closes early.
func TestProvider_EarlyClosesMatchThePublishedCalendar(t *testing.T) {
	p := published.New()
	for year, published := range publishedEarlyCloses {
		for _, s := range published {
			early, err := p.EarlyClose(date(t, s))
			if err != nil {
				t.Fatalf("%s: %v", s, err)
			}
			if !early {
				t.Errorf("%s should close early", s)
			}
		}

		// No other weekday in the year closes early.
		for d := date(t, yearStart(year)); d.Year() == year; d = d.AddDate(0, 0, 1) {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			if contains(published, d.Format("2006-01-02")) {
				continue
			}
			early, err := p.EarlyClose(d)
			if err != nil {
				t.Fatalf("%s: %v", d.Format("2006-01-02"), err)
			}
			if early {
				t.Errorf("unexpected early close on %s", d.Format("2006-01-02"))
			}
		}
	}
}

// The NYSE closed on January 9th 2025 for the National Day of Mourning
// for President Carter. A rule-based calendar cannot know that: the day
// is an ordinary Thursday. The published calendar can, and the provider
// serves it.
func TestProvider_ClosesForTheNationalDayOfMourning(t *testing.T) {
	trading, err := published.New().TradingDay(date(t, "2025-01-09"))
	if err != nil {
		t.Fatal(err)
	}
	if trading {
		t.Fatal("2025-01-09 should be closed for the National Day of Mourning")
	}
}

// New Year's Day 2028 falls on a Saturday, and the NYSE observes no
// replacement holiday: Friday December 31st 2027 is an ordinary
// trading day.
//
// A rule-based calendar that assumes the federal weekend rule would
// close it. This is the case that distinguishes the published calendar
// from the rules, so it is pinned here.
func TestProvider_DoesNotCloseForNewYearOnASaturday(t *testing.T) {
	p := published.New()

	trading, err := p.TradingDay(date(t, "2027-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !trading {
		t.Fatal("2027-12-31 should be a trading day: no New Year holiday is observed for it")
	}

	early, err := p.EarlyClose(date(t, "2027-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if early {
		t.Fatal("2027-12-31 should be a full session, not an early close")
	}
}

// A year the published calendars do not cover is an error rather than a
// guess.
func TestProvider_RefusesAnUncoveredYear(t *testing.T) {
	if _, err := published.New().TradingDay(date(t, "2030-01-02")); err == nil {
		t.Fatal("expected an error for a year with no published calendar")
	}
}

// The session times are only defined for a day the exchange traded on.
func TestProvider_SessionTimesOnANonSessionDay(t *testing.T) {
	p := published.New()

	if _, err := p.SessionClose(date(t, "2024-01-06")); err == nil {
		t.Fatal("expected an error for the close of a day with no session")
	}
	if _, err := p.SessionOpen(date(t, "2024-01-06")); err == nil {
		t.Fatal("expected an error for the open of a day with no session")
	}
}

// The session open and close are the published times.
func TestProvider_SessionTimes(t *testing.T) {
	p := published.New()

	open, err := p.SessionOpen(date(t, "2024-01-03"))
	if err != nil {
		t.Fatal(err)
	}
	if open.Hour() != 9 || open.Minute() != 30 {
		t.Errorf("open = %v want 09:30", open)
	}

	close, err := p.SessionClose(date(t, "2024-01-03"))
	if err != nil {
		t.Fatal(err)
	}
	if close.Hour() != 16 || close.Minute() != 0 {
		t.Errorf("close = %v want 16:00", close)
	}

	// An early-close day closes at 13:00.
	close, err = p.SessionClose(date(t, "2024-11-29"))
	if err != nil {
		t.Fatal(err)
	}
	if close.Hour() != 13 || close.Minute() != 0 {
		t.Errorf("close = %v want 13:00 on an early-close day", close)
	}
}

// yearStart returns the first day of the year in YYYY-MM-DD form.
func yearStart(year int) string {
	return fmt.Sprintf("%04d-01-01", year)
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
