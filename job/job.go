// Package job is the strategy execution abstraction. A Job runs at a
// scheduled time, receives a Context and the current date (sourced from
// the injected Clock), and may return zero or more orders.
//
// There is no OnDay/OnStart/OnEnd lifecycle; everything a strategy wants
// to execute is a Job.
package job

import (
	"errors"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// Mode identifies which stack the strategy is running against.
type Mode string

const (
	// ModeBacktest means orders are simulated against historical bars.
	ModeBacktest Mode = "backtest"
	// ModeLive means orders are sent to a real broker.
	ModeLive Mode = "live"
)

func (m Mode) String() string { return string(m) }

// Handler is the single strategy execution abstraction.
type Handler func(ctx Context, date time.Time) ([]order.Order, error)

// JobHandler is an alias for Handler, kept for readability at call sites.
type JobHandler = Handler

// Job is a registered unit of strategy work.
type Job struct {
	Name string
	// Time is when the job fires. Use Time for a fixed wall-clock time,
	// or AtClose/BeforeClose/AtOpen for a slot tied to the session.
	Time    Slot
	Handler Handler
}

// Context is the read-only strategy-facing surface; the framework provides
// a concrete implementation behind this interface.
type Context interface {
	Today() time.Time
	Now() time.Time
	// Mode reports whether the strategy is running in backtest or live
	// mode. The framework guarantees both stacks behave identically, so
	// strategies rarely need it.
	Mode() Mode
	Bars(symbol string, lookback int) ([]order.Bar, error)
	Constituents(etf string) ([]string, error)
	IsTradingDay(date time.Time) bool
	Position(symbol string) order.Position
	Positions() []order.Position
	Cash() float64
	PortfolioValue() float64
	Config() Config
	Logger() Logger
}

// Config and Logger interfaces are satisfied by user-provided adapters.
type Config interface {
	Get(key string) (string, error)
	Watch(key string, out any, factory any) (Watcher, error)
}

type Watcher interface {
	Value(out any) error
	Stop()
}

type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// ErrNotFound is returned by Context implementations when a resource is
// not available.
var ErrNotFound = errors.New("not found")

// ErrInvalidJob is returned when a Job fails validation.
type ErrInvalidJob struct {
	Name   string
	Reason string
}

func (e ErrInvalidJob) Error() string {
	return "invalid job " + e.Name + ": " + e.Reason
}

func (j Job) Validate() error {
	if j.Name == "" {
		return ErrInvalidJob{Name: j.Name, Reason: "name is required"}
	}
	if j.Handler == nil {
		return ErrInvalidJob{Name: j.Name, Reason: "handler is required"}
	}
	if err := j.Time.Validate(); err != nil {
		return ErrInvalidJob{Name: j.Name, Reason: err.Error()}
	}
	return nil
}
