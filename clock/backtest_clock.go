package clock

import (
	"sync"
	"time"
)

// BacktestClock is a deterministic clock driven by the backtest engine.
// Strategies read time through this Clock; they never call time.Now().
type BacktestClock struct {
	mu      sync.RWMutex
	current time.Time
}

// NewBacktest returns a BacktestClock seeded at the supplied time.
//
// The seed's calendar date is read as written and anchored on the NYSE
// calendar, with the time-of-day normalized to midnight.
func NewBacktest(start time.Time) *BacktestClock {
	return &BacktestClock{current: NYSEStartOfDay(start)}
}

// Set replaces the current simulated time. The date component is used
// for Today(); the time-of-day component is used for Now().
func (c *BacktestClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = NYSETime(t)
}

// Advance moves the current simulated time forward to t.
func (c *BacktestClock) Advance(t time.Time) {
	c.Set(t)
}

func (c *BacktestClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *BacktestClock) Today() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return StartOfDay(c.current)
}
