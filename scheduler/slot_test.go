package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/scheduler"
)

func noopHandler(context.Context, scheduler.JobContext, job.Job, time.Time, []order.Order) error {
	return nil
}

// stubContext is a job.Context the scheduler accepts but the test
// handlers never call into.
type stubContext struct{ job.Context }

// A job scheduled for the close fires at 16:00 on an ordinary day and
// at 13:00 on an early-close day. This is the whole point of a
// close-relative slot: the exchange decides when the session ends.
func TestReal_CloseRelativeSlotMovesWithTheSession(t *testing.T) {
	c := clock.NewBacktest(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	s := scheduler.NewReal(c, nil)
	s.SetSession(clock.NewSession(c, published.New()))
	if err := s.Register(job.Job{
		Name: "close", Time: job.AtClose(),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	// 2026-11-27 is the Friday after Thanksgiving - an early close.
	slot, jobs, found := s.NextPendingSlot(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	if !found {
		t.Fatal("expected a slot")
	}
	if slot.Hour() != 13 || slot.Minute() != 0 {
		t.Fatalf("early-close slot=%v want 13:00 ET", slot)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}

	// An ordinary day closes at 16:00.
	c2 := clock.NewBacktest(time.Date(2026, 12, 1, 0, 0, 0, 0, calendar.NYSE))
	s2 := scheduler.NewReal(c2, nil)
	s2.SetSession(clock.NewSession(c2, published.New()))
	if err := s2.Register(job.Job{
		Name: "close", Time: job.AtClose(),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	slot2, _, found2 := s2.NextPendingSlot(time.Date(2026, 12, 1, 0, 0, 0, 0, calendar.NYSE))
	if !found2 {
		t.Fatal("expected a slot")
	}
	if slot2.Hour() != 16 || slot2.Minute() != 0 {
		t.Fatalf("slot=%v want 16:00 ET", slot2)
	}
}

// BeforeClose moves with the close as well: 15:00 normally, 12:00 on
// an early-close day.
func TestReal_BeforeCloseMovesWithTheSession(t *testing.T) {
	c := clock.NewBacktest(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	s := scheduler.NewReal(c, nil)
	s.SetSession(clock.NewSession(c, published.New()))
	if err := s.Register(job.Job{
		Name: "preload", Time: job.BeforeClose(time.Hour),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	slot, _, found := s.NextPendingSlot(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	if !found {
		t.Fatal("expected a slot")
	}
	if slot.Hour() != 12 {
		t.Fatalf("slot=%v want 12:00 ET on an early-close day", slot)
	}
}

// A job before the open fires at its wall-clock time, whatever the
// session does later that day.
func TestReal_PreOpenSlotIsUnaffectedByEarlyClose(t *testing.T) {
	c := clock.NewBacktest(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	s := scheduler.NewReal(c, nil)
	s.SetSession(clock.NewSession(c, published.New()))
	if err := s.Register(job.Job{
		Name: "cancel-stale", Time: job.Time(7, 30),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	slot, _, found := s.NextPendingSlot(time.Date(2026, 11, 27, 0, 0, 0, 0, calendar.NYSE))
	if !found {
		t.Fatal("expected a slot")
	}
	if slot.Hour() != 7 || slot.Minute() != 30 {
		t.Fatalf("slot=%v want 07:30 ET", slot)
	}
}

// A job that fails must not end the live session: the scheduler logs
// the error and carries on to the next job in the slot.
func TestReal_FailedJobDoesNotStopTheLoop(t *testing.T) {
	slot, clk := currentMinuteClock(t)
	s := scheduler.NewReal(clk, nil)

	var mu sync.Mutex
	fired := map[string]int{}
	failing := job.Job{
		Name: "a_fails", Time: job.FixedTime(slot.Hour(), slot.Minute()),
		Handler: func(job.Context, time.Time) ([]order.Order, error) {
			mu.Lock()
			fired["a_fails"]++
			mu.Unlock()
			return nil, context.DeadlineExceeded
		},
	}
	healthy := job.Job{
		Name: "b_runs", Time: job.FixedTime(slot.Hour(), slot.Minute()),
		Handler: func(job.Context, time.Time) ([]order.Order, error) {
			mu.Lock()
			fired["b_runs"]++
			mu.Unlock()
			return nil, nil
		},
	}
	if err := s.Register(failing); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(healthy); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, &stubContext{}, noopHandler)
	}()

	waitFor(t, &mu, fired, "b_runs")
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if fired["a_fails"] == 0 {
		t.Fatal("expected the failing job to have run")
	}
	if fired["b_runs"] == 0 {
		t.Fatal("the job after the failing one never ran - the loop stopped")
	}
}

// Two jobs registered at the same slot must both fire. The scheduler
// used to return only one, so the alphabetically-first job ran and the
// other was silently skipped until the next day.
func TestReal_AllJobsAtASlotFire(t *testing.T) {
	slot, clk := currentMinuteClock(t)
	s := scheduler.NewReal(clk, nil)

	var mu sync.Mutex
	fired := map[string]int{}
	mk := func(name string) job.Job {
		return job.Job{
			Name: name, Time: job.FixedTime(slot.Hour(), slot.Minute()),
			Handler: func(job.Context, time.Time) ([]order.Order, error) {
				mu.Lock()
				fired[name]++
				mu.Unlock()
				return nil, nil
			},
		}
	}
	for _, n := range []string{"a_job", "b_job"} {
		if err := s.Register(mk(n)); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, &stubContext{}, noopHandler) }()

	waitFor(t, &mu, fired, "a_job")
	waitFor(t, &mu, fired, "b_job")
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if fired["a_job"] == 0 || fired["b_job"] == 0 {
		t.Fatalf("fired=%v want both jobs", fired)
	}
}

// currentMinuteClock returns the current minute in ET along with a
// clock positioned just before it, so a job scheduled for that minute
// fires within a second or two.
func currentMinuteClock(t *testing.T) (time.Time, *fixedClock) {
	t.Helper()
	now := time.Now().In(calendar.NYSE)
	slot := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, calendar.NYSE)
	return slot, &fixedClock{t: slot.Add(-2 * time.Second)}
}

// waitFor blocks until the named job has fired.
func waitFor(t *testing.T, mu *sync.Mutex, fired map[string]int, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := fired[name]
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %q never fired", name)
}

// fixedClock reports a fixed time.
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fixedClock) Today() time.Time { return c.Now() }
