// Package clock provides the time abstraction used by the framework.
//
// Strategies never call time.Now() directly; they read time through the
// injected Clock (exposed via Context).
package clock

import "time"

type Clock interface {
	Now() time.Time
	Today() time.Time
}

// StartOfDay returns midnight in the supplied location.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
