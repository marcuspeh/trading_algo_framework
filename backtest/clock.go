// Package backtest provides the deterministic clock, fill model, and
// executor used for backtesting. The same Context type used in live
// mode drives backtest Jobs; only the Clock/Portfolio/Executor differ.
package backtest

import (
	"sync"
	"time"
)

// Clock is the deterministic clock used by backtests. The engine calls
// SetDate/SetNow to step simulated time.
type Clock struct {
	mu   sync.RWMutex
	date time.Time
	now  time.Time
}

func NewClock(start time.Time) *Clock {
	d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return &Clock{date: d, now: d}
}

func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *Clock) Date() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.date
}

// SetDate moves the simulated date forward (time-of-day zero).
// Does not move Now.
func (c *Clock) SetDate(d time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.date = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

// SetNow updates the simulated wall-clock.
func (c *Clock) SetNow(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
