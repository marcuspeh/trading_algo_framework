// Package scheduler runs registered Jobs at their scheduled TimeOfDay.
//
// The scheduler is intentionally simple: it knows nothing about trading
// calendars, market hours, or jobs' return values. The caller supplies a
// JobProcessor that handles validation, risk, and execution.
//
// The same Scheduler is used for Backtest (deterministic step-through)
// and Live (real-time wait) modes.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

type JobProcessor interface {
	ProcessJob(ctx tf.Context, job tf.Job, date time.Time, orders []tf.Order) error
}

type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type Clock interface {
	Now() time.Time
}

type Scheduler struct {
	mu        sync.Mutex
	jobs      []tf.Job
	jobsByKey map[string]tf.Job
	log       Logger
}

func New(log Logger) *Scheduler {
	if log == nil {
		log = nopLogger{}
	}
	return &Scheduler{
		jobsByKey: map[string]tf.Job{},
		log:       log,
	}
}

func (s *Scheduler) RegisterJob(job tf.Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobsByKey[job.Name]; exists {
		return fmt.Errorf("scheduler: duplicate job name %q", job.Name)
	}
	s.jobsByKey[job.Name] = job
	s.jobs = append(s.jobs, job)
	return nil
}

func (s *Scheduler) Jobs() []tf.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tf.Job, len(s.jobs))
	copy(out, s.jobs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Time.Hour != out[j].Time.Hour {
			return out[i].Time.Hour < out[j].Time.Hour
		}
		if out[i].Time.Minute != out[j].Time.Minute {
			return out[i].Time.Minute < out[j].Time.Minute
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *Scheduler) HasJobs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs) > 0
}

// RunBacktest deterministically steps through every (date, time) slot at
// which any job is scheduled, invoking each job whose TimeOfDay matches.
func (s *Scheduler) RunBacktest(
	ctx context.Context,
	proc JobProcessor,
	tfCtx tf.Context,
	clockDate time.Time,
	clockNow time.Time,
) error {
	if proc == nil {
		return errors.New("scheduler: nil processor")
	}
	if tfCtx == nil {
		return errors.New("scheduler: nil trading context")
	}

	for _, slot := range s.uniqueSlots() {
		slotNow := atTime(clockDate, slot)
		if slotNow.Before(clockNow) {
			continue
		}
		for _, job := range s.jobsAt(slot) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.invoke(proc, tfCtx, clockDate, slotNow, job); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunLive blocks until ctx is cancelled, firing each job at its TimeOfDay
// based on the live wall clock. If today's slot has already passed, the
// scheduler fires the job immediately.
func (s *Scheduler) RunLive(
	ctx context.Context,
	clock Clock,
	proc JobProcessor,
	tfCtx tf.Context,
) error {
	if clock == nil {
		return errors.New("scheduler: nil clock")
	}
	if proc == nil {
		return errors.New("scheduler: nil processor")
	}
	if tfCtx == nil {
		return errors.New("scheduler: nil trading context")
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		now := clock.Now()
		next, job, found := s.nextPending(now)
		if !found {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		wait := next.Sub(now)
		if wait > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(wait):
			}
		}

		if err := ctx.Err(); err != nil {
			return nil
		}

		date := next
		if err := s.invoke(proc, tfCtx, date, next, job); err != nil {
			return err
		}
	}
}

func (s *Scheduler) invoke(
	proc JobProcessor,
	tfCtx tf.Context,
	date time.Time,
	at time.Time,
	job tf.Job,
) error {
	s.log.Info("job start", "name", job.Name, "at", at.Format(time.RFC3339))

	orders, err := job.Handler(tfCtx, date)
	if err != nil {
		s.log.Error("job error", "name", job.Name, "err", err.Error())
		return fmt.Errorf("job %q: %w", job.Name, err)
	}

	s.log.Info("job end", "name", job.Name, "orders", len(orders))

	if err := proc.ProcessJob(tfCtx, job, date, orders); err != nil {
		s.log.Error("order pipeline error", "name", job.Name, "err", err.Error())
		return err
	}
	return nil
}

func (s *Scheduler) uniqueSlots() []tf.TimeOfDay {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[tf.TimeOfDay]struct{}{}
	out := []tf.TimeOfDay{}
	for _, j := range s.jobs {
		if _, ok := seen[j.Time]; ok {
			continue
		}
		seen[j.Time] = struct{}{}
		out = append(out, j.Time)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hour != out[j].Hour {
			return out[i].Hour < out[j].Hour
		}
		return out[i].Minute < out[j].Minute
	})
	return out
}

func (s *Scheduler) jobsAt(slot tf.TimeOfDay) []tf.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []tf.Job{}
	for _, j := range s.jobs {
		if j.Time.Equal(slot) {
			out = append(out, j)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// nextPending returns the earliest upcoming (job, time) pair at or after now.
func (s *Scheduler) nextPending(now time.Time) (time.Time, tf.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) == 0 {
		return time.Time{}, tf.Job{}, false
	}

	type pending struct {
		at  time.Time
		job tf.Job
	}
	var candidates []pending

	for _, j := range s.jobs {
		today := atTime(now, j.Time)
		if !today.Before(now) {
			candidates = append(candidates, pending{at: today, job: j})
		}
		tomorrow := today.Add(24 * time.Hour)
		candidates = append(candidates, pending{at: tomorrow, job: j})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].at.Equal(candidates[j].at) {
			return candidates[i].at.Before(candidates[j].at)
		}
		return candidates[i].job.Name < candidates[j].job.Name
	})
	first := candidates[0]
	return first.at, first.job, true
}

func atTime(ref time.Time, tod tf.TimeOfDay) time.Time {
	return time.Date(
		ref.Year(), ref.Month(), ref.Day(),
		tod.Hour, tod.Minute, 0, 0,
		ref.Location(),
	)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
