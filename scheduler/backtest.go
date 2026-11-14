package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/job"
)

// Backtest is a deterministic scheduler driven by the backtest engine.
// The engine advances the BacktestClock and calls FireAt for each
// (date, time) slot that has any scheduled Jobs. Backtest never sleeps.
type Backtest struct {
	mu    sync.Mutex
	jobs  []job.Job
	byKey map[string]job.Job
	log   Logger

	// session supplies the session boundaries that close-relative
	// slots resolve against.
	session job.Session
}

func NewBacktest(log Logger) *Backtest {
	if log == nil {
		log = nopLogger{}
	}
	return &Backtest{
		byKey:   map[string]job.Job{},
		log:     log,
		session: calendar.NYSESession{},
	}
}

// SetSession sets the session that close-relative slots resolve
// against.
//
// The default is the NYSE calendar. A strategy that replaces the
// session close replaces it here too.
func (s *Backtest) SetSession(sess job.Session) {
	if sess == nil {
		sess = calendar.NYSESession{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = sess
}

func (s *Backtest) Register(j job.Job) error {
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

func (s *Backtest) Jobs() []job.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]job.Job, len(s.jobs))
	copy(out, s.jobs)
	// Jobs are sorted by name. A relative slot has no fixed time to sort
	// by - it resolves differently on a normal day and an early-close
	// day - so name order is what keeps a backtest deterministic.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *Backtest) HasJobs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs) > 0
}

// Run blocks until ctx is cancelled. The backtest engine drives
// execution by calling FireAt for each simulated (date, time) slot.
//
// Backtest.Run itself only validates the configuration and waits for
// ctx to be cancelled. Use FireAt from the engine to fire Jobs in
// deterministic order.
func (s *Backtest) Run(ctx context.Context, jc JobContext, h Handler) error {
	if h == nil {
		return errors.New("scheduler: nil handler")
	}
	if jc == nil {
		return errors.New("scheduler: nil job context")
	}
	if !s.HasJobs() {
		return errors.New("scheduler: no jobs registered")
	}
	<-ctx.Done()
	return nil
}

// FireAt fires every Job whose slot resolves to `at`, in name order.
//
// Each job's slot is resolved against the date of `at`, so a job
// scheduled for the close fires at 16:00 on an ordinary day and at
// 13:00 on an early-close day.
//
// Returns the first non-nil error from the Handler.
func (s *Backtest) FireAt(ctx context.Context, jc JobContext, h Handler, at time.Time) error {
	matched := s.JobsAt(at)
	for _, j := range matched {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.fire(ctx, jc, h, j, at); err != nil {
			return err
		}
	}
	return nil
}

// JobsAt returns the jobs whose slot resolves to the given time, sorted
// by name.
//
// Slots are matched on wall-clock time of day rather than on the
// instant, so a caller may pass the time in any location.
func (s *Backtest) JobsAt(at time.Time) []job.Job {
	s.mu.Lock()
	jobs := make([]job.Job, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.Unlock()

	var matched []job.Job
	for _, j := range jobs {
		if sameTimeOfDay(j.Time.Resolve(at, s.session), at) {
			matched = append(matched, j)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})
	return matched
}

// sameTimeOfDay reports whether two times carry the same wall-clock
// hour and minute.
func sameTimeOfDay(a, b time.Time) bool {
	return a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

func (s *Backtest) fire(ctx context.Context, jc JobContext, h Handler, j job.Job, at time.Time) error {
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
