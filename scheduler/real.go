package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
)

// maxSlotSearchDays bounds how far ahead NextPendingSlot looks for a
// trading day. It exists so a trading-day predicate that rejects every
// day cannot spin forever.
const maxSlotSearchDays = 400

// Real waits on the wall clock and fires Jobs at their scheduled time.
// The injected Clock supplies Now().
type Real struct {
	mu    sync.Mutex
	jobs  []job.Job
	byKey map[string]job.Job
	log   Logger
	clock clock.Clock

	// tradingDay reports whether the exchange is open on a date. The
	// default accepts every day; the app installs the NYSE calendar.
	tradingDay func(time.Time) bool

	// lastFired is the slot time most recently fired. The next-slot
	// search starts after it, so a clock sitting exactly on a slot
	// boundary cannot fire the same slot twice.
	lastFired time.Time

	// beforeJob runs before each job's handler. It is how the live
	// engine refreshes the portfolio from the broker before a job
	// reads it.
	beforeJob BeforeJob

	// session supplies the session boundaries that close-relative
	// slots resolve against.
	session job.Session
}

// BeforeJob is called before a job's handler runs.
type BeforeJob func(ctx context.Context, jc JobContext, j job.Job, date time.Time) error

func NewReal(c clock.Clock, log Logger) *Real {
	if c == nil {
		c = clock.NewReal()
	}
	if log == nil {
		log = nopLogger{}
	}
	return &Real{
		byKey:      map[string]job.Job{},
		clock:      c,
		log:        log,
		tradingDay: func(time.Time) bool { return true },
		session:    calendar.NYSESession{},
	}
}

// SetSession sets the session that close-relative slots resolve
// against.
//
// The default is the NYSE calendar. A strategy that replaces the
// session close replaces it here too.
func (s *Real) SetSession(sess job.Session) {
	if sess == nil {
		sess = calendar.NYSESession{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = sess
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

// SetTradingDay sets the predicate that decides which dates are
// trading days. Jobs do not fire on a non-trading day.
func (s *Real) SetTradingDay(fn func(time.Time) bool) {
	if fn == nil {
		fn = func(time.Time) bool { return true }
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tradingDay = fn
}

// SetBeforeJob sets the hook that runs before each job's handler.
//
// It is how the live engine refreshes the portfolio from the broker
// before a job reads it. An error is logged and the job still runs:
// a stale portfolio is better than no job at all.
func (s *Real) SetBeforeJob(fn BeforeJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beforeJob = fn
}

func (s *Real) Jobs() []job.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]job.Job, len(s.jobs))
	copy(out, s.jobs)
	sort.SliceStable(out, func(i, j int) bool {
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
		next, jobs, found := s.nextPending(now)
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
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}

		if err := ctx.Err(); err != nil {
			return nil
		}

		// Every job registered at this slot fires. A job that fails is
		// logged and the session continues: one transient error must not
		// end a trading run.
		for _, j := range jobs {
			if err := ctx.Err(); err != nil {
				return nil
			}
			if err := s.fire(ctx, jc, h, j, next); err != nil {
				s.log.Error("job failed", "name", j.Name, "err", err.Error())
			}
		}
		s.setLastFired(next)
	}
}

// fire calls the Job's Handler, then forwards the orders to the
// user-supplied Handler for risk validation and execution.
func (s *Real) fire(ctx context.Context, jc JobContext, h Handler, j job.Job, at time.Time) error {
	date := at

	s.mu.Lock()
	before := s.beforeJob
	s.mu.Unlock()
	if before != nil {
		if err := before(ctx, jc, j, date); err != nil {
			s.log.Error("before-job hook failed", "name", j.Name, "err", err.Error())
		}
	}

	s.log.Info("job start", "name", j.Name, "at", at.Format(time.RFC3339))
	orders, err := j.Handler(jc, date)
	if err != nil {
		s.log.Error("job error", "name", j.Name, "err", err.Error())
		return fmt.Errorf("job %q: %w", j.Name, err)
	}
	s.log.Info("job end", "name", j.Name, "orders", len(orders))
	return h(ctx, jc, j, date, orders)
}

// NextPendingSlot returns the next slot at or after `now`, together
// with every job registered at that slot.
//
// The search walks forward day by day, skipping days the exchange is
// closed, so a slot scheduled for the close lands on 13:00 on an
// early-close day and 16:00 on an ordinary one.
//
// A slot whose time has already passed is not run late; the next
// occurrence is returned instead.
func (s *Real) NextPendingSlot(now time.Time) (time.Time, []job.Job, bool) {
	return s.nextPending(now)
}

func (s *Real) nextPending(now time.Time) (time.Time, []job.Job, bool) {
	s.mu.Lock()
	jobs := make([]job.Job, len(s.jobs))
	copy(jobs, s.jobs)
	tradingDay := s.tradingDay
	lastFired := s.lastFired
	s.mu.Unlock()

	if len(jobs) == 0 {
		return time.Time{}, nil, false
	}

	// The search starts strictly after the last fired slot, so a clock
	// sitting exactly on a slot boundary does not re-fire it.
	searchFrom := now
	if !lastFired.IsZero() && !searchFrom.After(lastFired) {
		searchFrom = lastFired.Add(time.Nanosecond)
	}

	// The walk is on the NYSE calendar, so a clock in another zone
	// still resolves slots against the exchange's dates.
	day := now.In(clock.NYSE)
	for i := 0; i < maxSlotSearchDays; i++ {
		if tradingDay(day) {
			if slot, matched, ok := s.slotOn(jobs, day, searchFrom); ok {
				return slot, matched, true
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}, nil, false
}

// slotOn returns the earliest slot on the given day that is at or
// after searchFrom, together with the jobs registered at it.
//
// Each job's slot is resolved against that day, so a close-relative
// slot resolves to the day's actual close.
func (s *Real) slotOn(jobs []job.Job, day, searchFrom time.Time) (time.Time, []job.Job, bool) {
	type candidate struct {
		at  time.Time
		job job.Job
	}

	var candidates []candidate
	for _, j := range jobs {
		at := j.Time.Resolve(day, s.session)
		if at.Before(searchFrom) {
			continue
		}
		candidates = append(candidates, candidate{at: at, job: j})
	}
	if len(candidates) == 0 {
		return time.Time{}, nil, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].at.Equal(candidates[j].at) {
			return candidates[i].at.Before(candidates[j].at)
		}
		return candidates[i].job.Name < candidates[j].job.Name
	})

	first := candidates[0]
	matched := []job.Job{first.job}
	for _, c := range candidates[1:] {
		if c.at.Equal(first.at) {
			matched = append(matched, c.job)
		}
	}
	return first.at, matched, true
}

// setLastFired records the slot that was just fired.
func (s *Real) setLastFired(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFired = at
}
