package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

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
}

func NewBacktest(log Logger) *Backtest {
	if log == nil {
		log = nopLogger{}
	}
	return &Backtest{
		byKey: map[string]job.Job{},
		log:   log,
	}
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

// FireAt fires every Job whose TimeOfDay matches `at` (compared against
// at's hour/minute in its location), in name order. Returns the first
// non-nil error from the Handler.
func (s *Backtest) FireAt(ctx context.Context, jc JobContext, h Handler, at time.Time) error {
	slot := job.TimeOfDay{Hour: at.Hour(), Minute: at.Minute()}
	var matched []job.Job
	s.mu.Lock()
	for _, j := range s.jobs {
		if j.Time.Equal(slot) {
			matched = append(matched, j)
		}
	}
	s.mu.Unlock()
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })

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
