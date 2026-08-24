package tradingframework

import "time"

// JobHandler is the single strategy execution abstraction. A Job runs at a
// scheduled time, receives a Context and the current simulated or live
// date, and may return zero or more orders.
type JobHandler func(ctx Context, date time.Time) ([]Order, error)

// Job is a registered unit of strategy work.
//
// There is intentionally no OnDay/OnStart/OnEnd lifecycle. Everything the
// strategy wants to run is a Job scheduled at a time of day.
type Job struct {
	Name    string
	Time    TimeOfDay
	Handler JobHandler
}

// Validate ensures the job is well-formed.
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

// ErrInvalidJob is returned by RegisterJob when a job fails validation.
type ErrInvalidJob struct {
	Name   string
	Reason string
}

func (e ErrInvalidJob) Error() string {
	return "invalid job " + e.Name + ": " + e.Reason
}