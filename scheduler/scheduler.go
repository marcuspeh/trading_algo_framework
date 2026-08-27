// Package scheduler dispatches Jobs at their scheduled TimeOfDay. There
// are two implementations: Real (waits on the wall clock) and Backtest
// (driven deterministically by the engine).
package scheduler

import (
	"context"
	"time"

	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// Logger is the scheduler's logger dependency. The framework's Logger
// satisfies this.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// JobContext is the strategy-facing context the scheduler passes to
// each Job's Handler.
type JobContext = job.Context

// Scheduler registers Jobs and runs them at their scheduled time.
type Scheduler interface {
	Register(j job.Job) error
	// Run blocks until ctx is done, firing each registered Job at its
	// scheduled TimeOfDay. The supplied Handler is invoked for each Job
	// with the framework-supplied Context, the Job, the date, and the
	// orders returned by the Job's Handler.
	Run(ctx context.Context, jc JobContext, h Handler) error
	Jobs() []job.Job
	HasJobs() bool
}

// Handler is invoked by the scheduler for each Job. The Handler is
// responsible for risk validation and execution via the injected Executor.
type Handler func(ctx context.Context, jc JobContext, j job.Job, date time.Time, orders []order.Order) error
