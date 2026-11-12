package clock

import "time"

// RealClock returns the current wall-clock time, optionally projected
// into a configured location.
type RealClock struct {
	loc *time.Location
}

// NewReal returns a RealClock on the NYSE calendar. That is the
// exchange's zone, so "today" and job slots line up with the market.
func NewReal() *RealClock {
	return &RealClock{loc: NYSE}
}

// NewRealIn returns a RealClock that always reports time in the supplied
// location. A nil location falls back to the NYSE calendar.
func NewRealIn(loc *time.Location) *RealClock {
	if loc == nil {
		loc = NYSE
	}
	return &RealClock{loc: loc}
}

func (c *RealClock) Now() time.Time {
	return time.Now().In(c.loc)
}

func (c *RealClock) Today() time.Time {
	return StartOfDay(c.Now())
}
