// Package backtest provides the deterministic clock, fill model, and
// executor used for backtesting.
//
// The same Context type used in live mode drives backtest Jobs. The
// only difference is which Clock/Portfolio/Executor the App wires in.
package backtest

import (
	"sync"
	"time"
)

// Clock is a deterministic clock used by backtests. The framework's
// internal Clock interface (used by Context.Now/Date) is satisfied by
// this type. The engine calls SetDate/SetNow to step simulated time.
type Clock struct {
	mu   sync.RWMutex
	date time.Time
	now  time.Time
}

// NewClock returns a Clock seeded at the supplied date (time-of-day zero).
func NewClock(start time.Time) *Clock {
	d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return &Clock{date: d, now: d}
}

// Now returns the simulated wall-clock time.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Date returns the simulated trading date (time-of-day zero).
func (c *Clock) Date() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.date
}

// SetDate moves the simulated date forward to the supplied date
// (time-of-day zero). Does not move Now().
func (c *Clock) SetDate(d time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.date = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

// SetNow updates the simulated wall-clock to the supplied time.
func (c *Clock) SetNow(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// AdvanceDate moves both Date and Now forward by one calendar day.
// Time-of-day of Now is preserved.
func (c *Clock) AdvanceDate() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.date = c.date.AddDate(0, 0, 1)
	c.now = time.Date(c.date.Year(), c.date.Month(), c.date.Day(),
		c.now.Hour(), c.now.Minute(), c.now.Second(), c.now.Nanosecond(),
		c.date.Location())
	return c.date
}
