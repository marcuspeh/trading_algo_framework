package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/scheduler"
)

func TestRegisterJob_ValidatesAndRejectsDuplicates(t *testing.T) {
	s := scheduler.New(nil)
	if err := s.RegisterJob(tf.Job{Name: "", Time: tf.Time(9, 0), Handler: func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }}); err == nil {
		t.Fatal("expected validation error for empty name")
	}

	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }
	good := tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: dummy}
	if err := s.RegisterJob(good); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := s.RegisterJob(good); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegisterJob_RejectsBadTime(t *testing.T) {
	s := scheduler.New(nil)
	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }
	if err := s.RegisterJob(tf.Job{Name: "x", Time: tf.Time(25, 0), Handler: dummy}); err == nil {
		t.Fatal("expected validation error for bad time")
	}
}

func TestJobsSortedByTimeOfDay(t *testing.T) {
	s := scheduler.New(nil)
	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }
	if err := s.RegisterJob(tf.Job{Name: "c", Time: tf.Time(16, 0), Handler: dummy}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterJob(tf.Job{Name: "a", Time: tf.Time(9, 0), Handler: dummy}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterJob(tf.Job{Name: "b", Time: tf.Time(12, 0), Handler: dummy}); err != nil {
		t.Fatal(err)
	}
	jobs := s.Jobs()
	if got := []string{jobs[0].Name, jobs[1].Name, jobs[2].Name}; got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestRunBacktest_ExecutesJobsInSlotOrder(t *testing.T) {
	s := scheduler.New(nil)

	var mu sync.Mutex
	var order []string

	record := func(name string) tf.JobHandler {
		return func(_ tf.Context, _ time.Time) ([]tf.Order, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			// Return one order so the processor is invoked.
			return []tf.Order{{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Market}}, nil
		}
	}

	if err := s.RegisterJob(tf.Job{Name: "c", Time: tf.Time(16, 0), Handler: record("c")}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterJob(tf.Job{Name: "a", Time: tf.Time(9, 0), Handler: record("a")}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterJob(tf.Job{Name: "b", Time: tf.Time(16, 0), Handler: record("b")}); err != nil {
		t.Fatal(err)
	}

	proc := &captureProc{}
	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := s.RunBacktest(context.Background(), proc, fakeContext{}, date, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(order))
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("unexpected order: %v", order)
	}
	if len(proc.calls) != 3 {
		t.Fatalf("processor should be called once per job invocation, got %d", len(proc.calls))
	}
}

func TestRunBacktest_SkipsPastSlots(t *testing.T) {
	s := scheduler.New(nil)

	called := 0
	h := func(tf.Context, time.Time) ([]tf.Order, error) {
		called++
		return nil, nil
	}
	if err := s.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: h}); err != nil {
		t.Fatal(err)
	}

	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	// Start the clock past the 09:00 slot.
	clockNow := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := s.RunBacktest(context.Background(), &captureProc{}, fakeContext{}, date, clockNow); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatalf("expected no invocations, got %d", called)
	}
}

func TestRunBacktest_HandlerErrorAborts(t *testing.T) {
	s := scheduler.New(nil)
	h := func(tf.Context, time.Time) ([]tf.Order, error) {
		return nil, errors.New("boom")
	}
	if err := s.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: h}); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	err := s.RunBacktest(context.Background(), &captureProc{}, fakeContext{}, date, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error from handler")
	}
}

func TestRunBacktest_NilProcessor(t *testing.T) {
	s := scheduler.New(nil)
	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }
	if err := s.RegisterJob(tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: dummy}); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	err := s.RunBacktest(context.Background(), nil, fakeContext{}, date, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for nil processor")
	}
}

// fakeContext is a minimal tf.Context for the scheduler tests.
type fakeContext struct{}

func (fakeContext) Date() time.Time                       { return time.Time{} }
func (fakeContext) Now() time.Time                        { return time.Time{} }
func (fakeContext) Bars(string, int) ([]tf.Bar, error)    { return nil, nil }
func (fakeContext) Constituents(string) ([]string, error) { return nil, nil }
func (fakeContext) IsTradingDay(time.Time) bool           { return true }
func (fakeContext) Position(string) tf.Position           { return tf.Position{} }
func (fakeContext) Positions() []tf.Position              { return nil }
func (fakeContext) Cash() float64                         { return 0 }
func (fakeContext) PortfolioValue() float64               { return 0 }
func (fakeContext) Mode() tf.Mode                         { return tf.Backtest }
func (fakeContext) Config() tf.Config                     { return nil }
func (fakeContext) Logger() tf.Logger                     { return nil }

// captureProc records every ProcessJob call.
type captureProc struct {
	mu    sync.Mutex
	calls []tf.Order
}

func (c *captureProc) ProcessJob(_ tf.Context, j tf.Job, _ time.Time, orders []tf.Order) error {
	c.mu.Lock()
	c.calls = append(c.calls, orders...)
	c.mu.Unlock()
	return nil
}
