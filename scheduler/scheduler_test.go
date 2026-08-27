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
