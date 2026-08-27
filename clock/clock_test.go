package clock_test

import (
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
)

func TestBacktestClock_NowAndToday(t *testing.T) {
	start := time.Date(2024, 1, 2, 14, 30, 0, 0, time.UTC)
	c := clock.NewBacktest(start)

	// NewBacktest normalizes time-of-day to zero.
	wantNow := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if got := c.Now(); !got.Equal(wantNow) {
		t.Fatalf("Now()=%v want %v", got, wantNow)
	}
	if got := c.Today(); !got.Equal(wantNow) {
		t.Fatalf("Today()=%v want %v", got, wantNow)
	}
}

func TestBacktestClock_SetAdvances(t *testing.T) {
	c := clock.NewBacktest(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	c.Set(time.Date(2024, 1, 3, 9, 0, 0, 0, time.UTC))

	if got := c.Today(); !got.Equal(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Today()=%v", got)
	}
	if got := c.Now().Hour(); got != 9 {
		t.Fatalf("Now().Hour()=%d", got)
	}
}

func TestBacktestClock_AdvanceAlias(t *testing.T) {
	c := clock.NewBacktest(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	c.Advance(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC))
	if got := c.Today().Day(); got != 5 {
		t.Fatalf("Today().Day()=%d", got)
	}
}

func TestRealClock_TodayTruncatesTimeOfDay(t *testing.T) {
	c := clock.NewReal()
	today := c.Today()
	if today.Hour() != 0 || today.Minute() != 0 || today.Second() != 0 || today.Nanosecond() != 0 {
		t.Fatalf("RealClock.Today() returned non-midnight: %v", today)
	}
}

func TestRealClock_LocationHonored(t *testing.T) {
	// Pick a fixed location. UTC+8.
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Skipf("Singapore tz not available: %v", err)
	}
	c := clock.NewRealIn(loc)
	if c.Now().Location().String() != loc.String() {
		t.Fatalf("Now() location=%s want %s", c.Now().Location(), loc)
	}
}
