package tradingframework

import "time"

// JobHandler is the single strategy execution abstraction. The framework
// deliberately has no OnDay/OnStart/OnEnd lifecycle; everything the
// strategy wants to run is a Job scheduled at a time of day.
type JobHandler func(ctx Context, date time.Time) ([]Order, error)

type Job struct {
	Name    string
	Time    TimeOfDay
	Handler JobHandler
}

func (j Job) Validate() error {
	if j.Name == "" {
		return ErrInvalidJob{Name: j.Name, Reason: "name is required"}
	}
	if j.Handler == nil {
		return ErrInvalidJob{Name: j.Name, Reason: "handler is required"}
	}
	if err := j.Time.Validate(); err != nil {
		return ErrInvalidJob{Name: j.Name, Reason: err.Error()}
	}
	return nil
}

type ErrInvalidJob struct {
	Name   string
	Reason string
}

func (e ErrInvalidJob) Error() string {
	return "invalid job " + e.Name + ": " + e.Reason
}
