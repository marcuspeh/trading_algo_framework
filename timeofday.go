package tradingframework

import "fmt"

// TimeOfDay is a wall-clock time of day used to schedule Jobs.
//
// Times are interpreted in the local timezone of the runtime, or in the
// timezone of the simulated clock for backtests.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// Time is a convenience constructor for TimeOfDay.
func Time(hour, minute int) TimeOfDay {
	return TimeOfDay{Hour: hour, Minute: minute}
}

// String returns "HH:MM".
func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// Equal reports whether two times of day are identical.
func (t TimeOfDay) Equal(o TimeOfDay) bool {
	return t.Hour == o.Hour && t.Minute == o.Minute
}

// Validate ensures the time of day is within a valid 24-hour range.
func (t TimeOfDay) Validate() error {
	if t.Hour < 0 || t.Hour > 23 {
		return fmt.Errorf("invalid hour: %d", t.Hour)
	}
	if t.Minute < 0 || t.Minute > 59 {
		return fmt.Errorf("invalid minute: %d", t.Minute)
	}
	return nil
}