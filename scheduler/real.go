package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
)

// Real waits on the wall clock and fires Jobs at their scheduled
// TimeOfDay. The injected Clock supplies Now().
type Real struct {
	mu    sync.Mutex
	jobs  []job.Job
	byKey map[string]job.Job
	log   Logger
	clock clock.Clock
}

func NewReal(c clock.Clock, log Logger) *Real {
	if c == nil {
		c = clock.NewReal()
	}
	if log == nil {
		log = nopLogger{}
	}
	return &Real{
		byKey: map[string]job.Job{},
		clock: c,
		log:   log,
	}
}

func (s *Real) Register(j job.Job) error {
	if err := j.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byKey[j.Name]; exists {
		return fmt.Errorf("scheduler: duplicate job name %q", j.Name)
	}
	s.byKey[j.Name] = j
	s.jobs = append(s.jobs, j)
	return nil
}

func (s *Real) Jobs() []job.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]job.Job, len(s.jobs))
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

func (s *Real) HasJobs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs) > 0
}

func (s *Real) Run(ctx context.Context, jc JobContext, h Handler) error {
	if h == nil {
		return errors.New("scheduler: nil handler")
	}
	if jc == nil {
		return errors.New("scheduler: nil job context")
	}
	if !s.HasJobs() {
		return errors.New("scheduler: no jobs registered")
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		now := s.clock.Now()
		next, j, found := s.nextPending(now)
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

		if err := s.fire(ctx, jc, h, j, next); err != nil {
			return err
		}
	}
}

// fire calls the Job's Handler, then forwards the orders to the
// user-supplied Handler for risk validation and execution.
func (s *Real) fire(ctx context.Context, jc JobContext, h Handler, j job.Job, at time.Time) error {
	date := at
	s.log.Info("job start", "name", j.Name, "at", at.Format(time.RFC3339))
	orders, err := j.Handler(jc, date)
	if err != nil {
		s.log.Error("job error", "name", j.Name, "err", err.Error())
		return fmt.Errorf("job %q: %w", j.Name, err)
	}
	s.log.Info("job end", "name", j.Name, "orders", len(orders))
	return h(ctx, jc, j, date, orders)
}

func (s *Real) nextPending(now time.Time) (time.Time, job.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) == 0 {
		return time.Time{}, job.Job{}, false
	}

	type pending struct {
		at  time.Time
		job job.Job
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

func atTime(ref time.Time, tod job.TimeOfDay) time.Time {
	return time.Date(ref.Year(), ref.Month(), ref.Day(),
		tod.Hour, tod.Minute, 0, 0, ref.Location())
}
