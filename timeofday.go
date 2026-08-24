package tradingframework

import "fmt"

type TimeOfDay struct {
	Hour   int
	Minute int
}

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
