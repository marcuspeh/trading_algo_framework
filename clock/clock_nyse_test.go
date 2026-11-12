package clock_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
)

// The framework anchors every date on the NYSE calendar, so a date
// written in any zone keeps its calendar day.
func TestNYSEStartOfDay_KeepsCalendarDate(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"utc", time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE)},
		{"midday utc", time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE)},
		{"singapore", time.Date(2024, 1, 2, 0, 0, 0, 0, mustLoad(t, "Asia/Singapore")),
			time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE)},
		{"late evening utc", time.Date(2024, 1, 2, 23, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 2, 0, 0, 0, 0, clock.NYSE)},
	}
	for _, c := range cases {
		got := clock.NYSEStartOfDay(c.in)
		if !got.Equal(c.want) {
			t.Errorf("%s: NYSEStartOfDay(%v)=%v want %v", c.name, c.in, got, c.want)
		}
	}
}

// NYSETime re-expresses wall-clock fields in NYSE without shifting them.
func TestNYSETime_KeepsTimeOfDay(t *testing.T) {
	in := time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)
	got := clock.NYSETime(in)
	want := time.Date(2024, 1, 2, 16, 0, 0, 0, clock.NYSE)
	if !got.Equal(want) {
		t.Fatalf("NYSETime(%v)=%v want %v", in, got, want)
	}
}

// The real clock defaults to the NYSE calendar.
func TestRealClock_DefaultsToNYSE(t *testing.T) {
	c := clock.NewReal()
	if got := c.Now().Location().String(); got != "America/New_York" {
		t.Fatalf("Now() location=%s want America/New_York", got)
	}
}

// A backtest clock must land on the same calendar whatever zone the
// seed was written in.
func TestBacktestClock_SameDayAcrossZones(t *testing.T) {
	utc := clock.NewBacktest(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	sgt := clock.NewBacktest(time.Date(2024, 1, 2, 0, 0, 0, 0, mustLoad(t, "Asia/Singapore")))
	if !utc.Today().Equal(sgt.Today()) {
		t.Fatalf("Today() differs: utc=%v sgt=%v", utc.Today(), sgt.Today())
	}
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tz %s unavailable: %v", name, err)
	}
	return loc
}
