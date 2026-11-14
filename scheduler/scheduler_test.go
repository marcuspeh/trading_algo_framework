package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/scheduler"
)

func TestBacktest_FireAtFiresMatchingJob(t *testing.T) {
	s := scheduler.NewBacktest(nil)
	var fired []string
	j := job.Job{
		Name: "x", Time: job.Time(9, 0),
		Handler: func(_ job.Context, _ time.Time) ([]order.Order, error) {
			fired = append(fired, "x")
			return nil, nil
		},
	}
	if err := s.Register(j); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)
	if err := s.FireAt(context.Background(), nil, func(_ context.Context, _ scheduler.JobContext, _ job.Job, _ time.Time, _ []order.Order) error {
		return nil
	}, at); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0] != "x" {
		t.Fatalf("fired=%v", fired)
	}
}

func TestBacktest_FireAtIgnoresNonMatchingSlot(t *testing.T) {
	s := scheduler.NewBacktest(nil)
	var fired int
	if err := s.Register(job.Job{
		Name: "x", Time: job.Time(9, 0),
		Handler: func(_ job.Context, _ time.Time) ([]order.Order, error) {
			fired++
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := s.FireAt(context.Background(), nil, func(_ context.Context, _ scheduler.JobContext, _ job.Job, _ time.Time, _ []order.Order) error {
		return nil
	}, at); err != nil {
		t.Fatal(err)
	}
	if fired != 0 {
		t.Fatalf("fired=%d", fired)
	}
}

func TestBacktest_DuplicateRegistrationRejected(t *testing.T) {
	s := scheduler.NewBacktest(nil)
	if err := s.Register(job.Job{
		Name: "x", Time: job.Time(9, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(job.Job{
		Name: "x", Time: job.Time(9, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestBacktest_ValidateRejectsBadJob(t *testing.T) {
	s := scheduler.NewBacktest(nil)
	if err := s.Register(job.Job{Name: ""}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestReal_HasJobsAndJobs(t *testing.T) {
	c := clock.NewBacktest(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	s := scheduler.NewReal(c, nil)
	if s.HasJobs() {
		t.Fatal("expected no jobs")
	}
	if err := s.Register(job.Job{
		Name: "b", Time: job.Time(16, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(job.Job{
		Name: "a", Time: job.Time(9, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if !s.HasJobs() {
		t.Fatal("expected jobs")
	}
	jobs := s.Jobs()
	if jobs[0].Name != "a" || jobs[1].Name != "b" {
		t.Fatalf("jobs=%+v", jobs)
	}
}

func TestReal_NextPendingSlotCollectsAllJobsInSlot(t *testing.T) {
	simStart := time.Date(2024, 1, 2, 15, 0, 0, 0, clock.NYSE)
	c := clock.NewBacktest(simStart)
	s := scheduler.NewReal(c, nil)

	tod := job.Time(16, 0)
	if err := s.Register(job.Job{
		Name: "b_job", Time: tod,
		Handler: func(_ job.Context, _ time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(job.Job{
		Name: "a_job", Time: tod,
		Handler: func(_ job.Context, _ time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	slotTime, jobs, found := s.NextPendingSlot(simStart)
	if !found {
		t.Fatal("expected slot found")
	}
	expectedSlot := time.Date(2024, 1, 2, 16, 0, 0, 0, clock.NYSE)
	if !slotTime.Equal(expectedSlot) {
		t.Fatalf("expected slot %v, got %v", expectedSlot, slotTime)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "a_job" || jobs[1].Name != "b_job" {
		t.Fatalf("expected sorted [a_job, b_job], got [%s, %s]", jobs[0].Name, jobs[1].Name)
	}
}

func TestReal_SkipsNonTradingDays(t *testing.T) {
	// Friday 2024-01-05 16:30 (after market close)
	fridayEvening := time.Date(2024, 1, 5, 16, 30, 0, 0, clock.NYSE)
	c := clock.NewBacktest(fridayEvening)
	s := scheduler.NewReal(c, nil)
	s.SetTradingDay(func(d time.Time) bool {
		return d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
	})

	if err := s.Register(job.Job{
		Name: "close_job", Time: job.Time(16, 0),
		Handler: func(_ job.Context, _ time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	slotTime, jobs, found := s.NextPendingSlot(fridayEvening)
	if !found {
		t.Fatal("expected slot found")
	}
	// Next slot should be Monday 2024-01-08 16:00, skipping Saturday and Sunday
	expectedSlot := time.Date(2024, 1, 8, 16, 0, 0, 0, clock.NYSE)
	if !slotTime.Equal(expectedSlot) {
		t.Fatalf("expected slot %v, got %v", expectedSlot, slotTime)
	}
	if len(jobs) != 1 || jobs[0].Name != "close_job" {
		t.Fatalf("expected close_job, got %+v", jobs)
	}
}
