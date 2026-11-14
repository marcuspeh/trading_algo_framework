package job

import (
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
)

// TimeOfDay is the wall-clock time a Job is scheduled at when its
// schedule is a fixed time rather than an offset from the session.
type TimeOfDay = clock.TimeOfDay

// Time is a convenience constructor for a fixed wall-clock schedule.
func Time(hour, minute int) Slot {
	return FixedTime(hour, minute)
}

// anchor identifies which end of the trading session a relative
// schedule is measured from.
type anchor int

const (
	anchorNone anchor = iota
	anchorOpen
	anchorClose
)

// Slot is when a Job fires.
//
// A slot is either a fixed wall-clock time - 07:30 every day, whatever
// the market is doing - or an offset from a session boundary, which
// makes it move with the session:
//
//	job.AtClose()        16:00 on a normal day, 13:00 on an early-close day
//	job.BeforeClose(1h) 15:00 on a normal day, 12:00 on an early-close day
//	job.AtOpen()         09:30
//	job.BeforeOpen(2h)   07:30
//
// The relative forms exist because the session end is not a constant.
// A strategy that wants to act at the close must run at 16:00 on an
// ordinary day and at 13:00 on the day after Thanksgiving, and that
// difference is the exchange's, not the strategy's.
//
// A relative slot is resolved against the session of the day it fires
// on, so it is only meaningful on a trading day.
type Slot struct {
	fixed  TimeOfDay
	anchor anchor
	offset time.Duration
}

// FixedTime returns a slot that fires at the given wall-clock time
// every day.
func FixedTime(hour, minute int) Slot {
	return Slot{fixed: TimeOfDay{Hour: hour, Minute: minute}}
}

// AtOpen returns a slot that fires when the session opens.
func AtOpen() Slot {
	return Slot{anchor: anchorOpen}
}

// BeforeOpen returns a slot that fires the given duration before the
// session opens.
func BeforeOpen(d time.Duration) Slot {
	return Slot{anchor: anchorOpen, offset: -d}
}

// AfterOpen returns a slot that fires the given duration after the
// session opens.
func AfterOpen(d time.Duration) Slot {
	return Slot{anchor: anchorOpen, offset: d}
}

// AtClose returns a slot that fires when the session closes - 16:00 ET
// normally, 13:00 ET on an early-close day.
func AtClose() Slot {
	return Slot{anchor: anchorClose}
}

// BeforeClose returns a slot that fires the given duration before the
// session closes.
func BeforeClose(d time.Duration) Slot {
	return Slot{anchor: anchorClose, offset: -d}
}

// AfterClose returns a slot that fires the given duration after the
// session closes.
func AfterClose(d time.Duration) Slot {
	return Slot{anchor: anchorClose, offset: d}
}

// IsRelative reports whether the slot is measured from a session
// boundary rather than being a fixed wall-clock time.
func (s Slot) IsRelative() bool { return s.anchor != anchorNone }

// String describes the slot for logs and error messages.
func (s Slot) String() string {
	switch s.anchor {
	case anchorOpen:
		return fmt.Sprintf("open%s", formatOffset(s.offset))
	case anchorClose:
		return fmt.Sprintf("close%s", formatOffset(s.offset))
	}
	return s.fixed.String()
}

// Validate reports whether the slot can be scheduled.
func (s Slot) Validate() error {
	if s.anchor != anchorNone {
		return nil
	}
	return s.fixed.Validate()
}

// Resolve returns the wall-clock time the slot fires at on the given
// date.
//
// A fixed slot resolves to the same time every day. A relative slot
// resolves against the session of that date, so it lands on 13:00 on an
// early-close day and 16:00 on an ordinary one.
func (s Slot) Resolve(date time.Time) time.Time {
	switch s.anchor {
	case anchorOpen:
		return calendar.SessionOpen(date).Add(s.offset)
	case anchorClose:
		return calendar.SessionClose(date).Add(s.offset)
	}
	y, m, d := date.Date()
	return time.Date(y, m, d, s.fixed.Hour, s.fixed.Minute, 0, 0, calendar.NYSE)
}

// formatOffset renders a signed duration for a slot description.
func formatOffset(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}
