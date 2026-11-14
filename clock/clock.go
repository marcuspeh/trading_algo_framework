// Package clock provides the time abstraction used by the framework.
//
// Strategies never call time.Now() directly; they read time through the
// injected Clock (exposed via Context).
//
// Every date the framework reasons about - bar timestamps, job slots,
// trading-day checks - is anchored on NYSE (US/Eastern) wall-clock
// time, because that is the calendar the market-data service and the
// exchange both use.
package clock

import (
	"fmt"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"

	// Embed the IANA database so the NYSE location resolves even on
	// hosts without a system zoneinfo directory (scratch containers,
	// minimal CI images).
	_ "time/tzdata"
)

// NYSE is the location all framework dates are anchored to.
var NYSE = calendar.NYSE

type Clock interface {
	Now() time.Time
	Today() time.Time
}

// TimeOfDay is a wall-clock time of day, in the exchange's own timezone.
//
// It is the value a Job is scheduled against when the schedule is a
// fixed time rather than an offset from the session.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// Time is a convenience constructor.
func Time(hour, minute int) TimeOfDay {
	return TimeOfDay{Hour: hour, Minute: minute}
}

func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

func (t TimeOfDay) Equal(o TimeOfDay) bool {
	return t.Hour == o.Hour && t.Minute == o.Minute
}

func (t TimeOfDay) Validate() error {
	if t.Hour < 0 || t.Hour > 23 {
		return fmt.Errorf("invalid hour: %d", t.Hour)
	}
	if t.Minute < 0 || t.Minute > 59 {
		return fmt.Errorf("invalid minute: %d", t.Minute)
	}
	return nil
}

// StartOfDay returns midnight of t's calendar date in t's own location.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// NYSEStartOfDay returns midnight of t's calendar date in the NYSE
// location.
//
// The calendar date is taken from t as written rather than converted,
// so a caller that means "the trading day of 2024-01-02" gets that day
// regardless of the zone its time was expressed in.
func NYSEStartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, NYSE)
}

// NYSETime re-expresses t's calendar fields in the NYSE location.
//
// The fields are read as wall-clock values rather than converted, so
// "16:00" stays "16:00" instead of shifting with the source zone.
func NYSETime(t time.Time) time.Time {
	y, m, d := t.Date()
	h, min, s := t.Clock()
	return time.Date(y, m, d, h, min, s, t.Nanosecond(), NYSE)
}
