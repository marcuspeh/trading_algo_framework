package clock_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
)

// at builds a time on the NYSE calendar.
func at(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, clock.NYSE)
}

// day builds midnight on the NYSE calendar.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, clock.NYSE)
}

// The anchor is the most recent session whose close has passed.
//
// Before the close it is the previous session; at or after the close it
// is the current one. That is the rule that stops a job reading a bar
// that had not been printed yet.
func TestSession_Anchor(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before the open",
			now:  at(2024, time.January, 3, 7, 30),
			want: day(2024, time.January, 2),
		},
		{
			name: "during the session",
			now:  at(2024, time.January, 3, 11, 0),
			want: day(2024, time.January, 2),
		},
		{
			name: "at the close",
			now:  at(2024, time.January, 3, 16, 0),
			want: day(2024, time.January, 3),
		},
		{
			name: "after the close",
			now:  at(2024, time.January, 3, 20, 0),
			want: day(2024, time.January, 3),
		},
		{
			name: "on a Saturday",
			now:  at(2024, time.January, 6, 12, 0),
			want: day(2024, time.January, 5),
		},
		{
			name: "on a Sunday",
			now:  at(2024, time.January, 7, 12, 0),
			want: day(2024, time.January, 5),
		},
		{
			name: "on a holiday",
			now:  at(2024, time.January, 15, 12, 0),
			want: day(2024, time.January, 12),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := clock.NewSession(clock.NewBacktest(tc.now), nil, calendar.TradingDay)
			got := s.Anchor(tc.now)
			if !got.Equal(tc.want) {
				t.Errorf("Anchor=%v want %v", got, tc.want)
			}
		})
	}
}

// On an early-close day the session ends at 13:00, so a job at 14:00
// already reads that day's bar.
func TestSession_AnchorOnAnEarlyCloseDay(t *testing.T) {
	// 2026-11-27 is the Friday after Thanksgiving.
	now := at(2026, time.November, 27, 14, 0)
	s := clock.NewSession(clock.NewBacktest(now), nil, calendar.TradingDay)

	if got := s.Anchor(now); !got.Equal(day(2026, time.November, 27)) {
		t.Errorf("Anchor=%v want the early-close day itself", got)
	}
	if got := s.Close(day(2026, time.November, 27)); got.Hour() != 13 {
		t.Errorf("close=%v want 13:00 on an early-close day", got)
	}
}

// A job before the early close still reads the previous session.
//
// 2026-11-26 is Thanksgiving, so the session before the early-close
// Friday is the Wednesday.
func TestSession_AnchorBeforeAnEarlyClose(t *testing.T) {
	now := at(2026, time.November, 27, 11, 0)
	s := clock.NewSession(clock.NewBacktest(now), nil, calendar.TradingDay)

	if got := s.Anchor(now); !got.Equal(day(2026, time.November, 25)) {
		t.Errorf("Anchor=%v want the previous session", got)
	}
}

// The session close is per date, and the trading-day predicate is the
// one the session was built with.
func TestSession_CloseAndTradingDay(t *testing.T) {
	s := clock.NewSession(clock.NewBacktest(day(2024, time.January, 2)), nil, calendar.TradingDay)

	if got := s.Close(day(2024, time.January, 2)); got.Hour() != 16 {
		t.Errorf("close=%v want 16:00", got)
	}
	if !s.IsTradingDay(day(2024, time.January, 2)) {
		t.Error("expected 2024-01-02 to be a trading day")
	}
	if s.IsTradingDay(day(2024, time.January, 6)) {
		t.Error("expected Saturday to be closed")
	}
}

// A session built without a close function uses the NYSE calendar.
func TestSession_DefaultsToTheNYSECalendar(t *testing.T) {
	s := clock.NewSession(clock.NewBacktest(day(2024, time.January, 2)), nil, nil)

	// Without a predicate every day is a trading day, but the anchor
	// still respects the close: at midday on a Saturday, Saturday's
	// close has not passed, so the anchor is Friday.
	now := at(2024, time.January, 6, 12, 0)
	if got := s.Anchor(now); !got.Equal(day(2024, time.January, 5)) {
		t.Errorf("Anchor=%v want the previous day", got)
	}

	// After Saturday's close, it is Saturday itself.
	after := at(2024, time.January, 6, 20, 0)
	if got := s.Anchor(after); !got.Equal(day(2024, time.January, 6)) {
		t.Errorf("Anchor=%v want the same day", got)
	}
	if got := s.Close(day(2024, time.January, 2)); got.Hour() != 16 {
		t.Errorf("close=%v want 16:00", got)
	}
}
