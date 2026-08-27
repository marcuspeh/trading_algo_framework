package clock

import "time"

// RealClock returns the current wall-clock time, optionally projected
// into a configured location.
type RealClock struct {
	loc *time.Location
}

// NewReal returns a RealClock using the system local timezone.
func NewReal() *RealClock {
	return &RealClock{loc: time.Local}
}

// NewRealIn returns a RealClock that always reports time in the supplied
// location. A nil location falls back to time.Local.
func NewRealIn(loc *time.Location) *RealClock {
	if loc == nil {
		loc = time.Local
	}
	return &RealClock{loc: loc}
}

func (c *RealClock) Now() time.Time {
	return time.Now().In(c.loc)
}

func (c *RealClock) Today() time.Time {
	return StartOfDay(c.Now())
}
