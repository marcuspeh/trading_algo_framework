package job_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

func TestJob_Validate(t *testing.T) {
	cases := []struct {
		name string
		j    job.Job
		ok   bool
	}{
		{
			name: "valid",
			j:    job.Job{Name: "x", Time: job.Time(9, 0), Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil }},
			ok:   true,
		},
		{
			name: "missing name",
			j:    job.Job{Time: job.Time(9, 0), Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil }},
			ok:   false,
		},
		{
			name: "missing handler",
			j:    job.Job{Name: "x", Time: job.Time(9, 0)},
			ok:   false,
		},
		{
			name: "bad hour",
			j:    job.Job{Name: "x", Time: job.FixedTime(99, 0), Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil }},
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.j.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestErrInvalidJob_Error(t *testing.T) {
	e := job.ErrInvalidJob{Name: "x", Reason: "bad"}
	if got := e.Error(); got != "invalid job x: bad" {
		t.Fatalf("Error()=%q", got)
	}
}

func TestErrInvalidJob_IsError(t *testing.T) {
	var _ error = job.ErrInvalidJob{}
	_ = errors.New
}
