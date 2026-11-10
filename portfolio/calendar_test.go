package portfolio_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/portfolio"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNYSETradingDay_RegularWeekdays(t *testing.T) {
	// 2024-01-03 is a Wednesday, a normal trading day.
	if !portfolio.NYSETradingDay(date(2024, time.January, 3)) {
		t.Fatal("expected 2024-01-03 to be a trading day")
	}
	// 2024-01-06 is a Saturday.
	if portfolio.NYSETradingDay(date(2024, time.January, 6)) {
		t.Fatal("expected Saturday to be closed")
	}
	// 2024-01-07 is a Sunday.
	if portfolio.NYSETradingDay(date(2024, time.January, 7)) {
		t.Fatal("expected Sunday to be closed")
	}
}

func TestNYSETradingDay_Holidays2024(t *testing.T) {
	closed := []time.Time{
		date(2024, time.January, 1),   // New Year's Day (Monday)
		date(2024, time.January, 15),  // MLK Day (third Monday)
		date(2024, time.February, 19), // Washington's Birthday (third Monday)
		date(2024, time.March, 29),    // Good Friday
		date(2024, time.May, 27),      // Memorial Day (last Monday)
		date(2024, time.June, 19),     // Juneteenth (Wednesday)
		date(2024, time.July, 4),      // Independence Day (Thursday)
		date(2024, time.September, 2), // Labor Day (first Monday)
		date(2024, time.November, 28), // Thanksgiving (fourth Thursday)
		date(2024, time.December, 25), // Christmas (Wednesday)
	}
	for _, d := range closed {
		if portfolio.NYSETradingDay(d) {
			t.Errorf("expected %s to be closed", d.Format("2006-01-02"))
		}
	}
}

func TestNYSETradingDay_OpenOnSurroundingDays(t *testing.T) {
	open := []time.Time{
		date(2024, time.January, 2),   // day after New Year's
		date(2024, time.January, 16),  // day after MLK
		date(2024, time.March, 28),    // day before Good Friday
		date(2024, time.July, 3),      // day before Independence Day
		date(2024, time.December, 24), // Christmas Eve
	}
	for _, d := range open {
		if !portfolio.NYSETradingDay(d) {
			t.Errorf("expected %s to be a trading day", d.Format("2006-01-02"))
		}
	}
}

func TestNYSETradingDay_WeekendHolidaysObservedOnAdjacentWeekday(t *testing.T) {
	// 2022-12-25 was a Sunday, so the exchange was closed the following
	// Monday, 2022-12-26.
	if portfolio.NYSETradingDay(date(2022, time.December, 26)) {
		t.Error("expected 2022-12-26 to be closed (Christmas observed)")
	}
	if !portfolio.NYSETradingDay(date(2022, time.December, 23)) {
		t.Error("expected 2022-12-23 to be a trading day")
	}

	// 2021-07-04 was a Sunday, so the exchange was closed Monday
	// 2021-07-05.
	if portfolio.NYSETradingDay(date(2021, time.July, 5)) {
		t.Error("expected 2021-07-05 to be closed (Independence Day observed)")
	}
	if !portfolio.NYSETradingDay(date(2021, time.July, 2)) {
		t.Error("expected 2021-07-02 to be a trading day")
	}

	// 2022-12-24 was a Saturday, so the exchange was closed the
	// preceding Friday, 2022-12-23.
	if !portfolio.NYSETradingDay(date(2022, time.December, 23)) {
		t.Error("expected 2022-12-23 to be a trading day")
	}
}

func TestNYSETradingDay_SaturdayNewYearObservedOnPriorFriday(t *testing.T) {
	// 2022-01-01 was a Saturday, so New Year's Day was observed Friday
	// 2021-12-31.
	if portfolio.NYSETradingDay(date(2021, time.December, 31)) {
		t.Error("expected 2021-12-31 to be closed (New Year observed)")
	}
	if !portfolio.NYSETradingDay(date(2021, time.December, 30)) {
		t.Error("expected 2021-12-30 to be a trading day")
	}
}

func TestNYSETradingDay_GoodFriday2025And2026(t *testing.T) {
	// Good Friday 2025 was April 18; 2026 was April 3.
	if portfolio.NYSETradingDay(date(2025, time.April, 18)) {
		t.Error("expected 2025-04-18 (Good Friday) to be closed")
	}
	if portfolio.NYSETradingDay(date(2026, time.April, 3)) {
		t.Error("expected 2026-04-03 (Good Friday) to be closed")
	}
}

func TestNYSEWeekdays_MatchesNYSETradingDay(t *testing.T) {
	for _, d := range []time.Time{
		date(2024, time.July, 4),
		date(2024, time.July, 5),
		date(2024, time.March, 29),
	} {
		if portfolio.NYSEWeekdays(d) != portfolio.NYSETradingDay(d) {
			t.Errorf("mismatch for %s", d.Format("2006-01-02"))
		}
	}
}
