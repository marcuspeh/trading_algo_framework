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
	"time"

	// Embed the IANA database so the NYSE location resolves even on
	// hosts without a system zoneinfo directory (scratch containers,
	// minimal CI images).
	_ "time/tzdata"
)

// NYSE is the location all framework dates are anchored to.
var NYSE = mustLoadNYSE()

func mustLoadNYSE() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// time/tzdata is embedded above, so this is unreachable.
		panic("clock: America/New_York unavailable: " + err.Error())
	}
	return loc
}

type Clock interface {
	Now() time.Time
	Today() time.Time
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
