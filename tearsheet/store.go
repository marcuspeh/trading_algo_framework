package tearsheet

import (
	"context"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// Run identifies one of a strategy's runs.
//
// A run is what a tear sheet describes: a backtest over a window, or a
// live session. The strategy it belongs to is part of it, so two
// strategies can keep their history apart.
type Run struct {
	// Strategy is the identifier of the strategy the run belongs to.
	Strategy string

	// Mode is how the run was traded: "backtest" or "live".
	Mode string

	// Start and End are the bounds of the run.
	Start time.Time
	End   time.Time

	// Starting is what the account was worth when the run started.
	Starting float64

	// Currency is what the amounts are in.
	Currency string
}

// Events are the recorded events of a run: the portfolio's value at
// each sample, and the fills that moved it.
//
// The closed trades are not stored. They are derived from the fills by
// the recorder, so a run that is loaded back is matched the same way
// the run that recorded it was.
type Events struct {
	// Equity is the portfolio's value at each sample, oldest first.
	Equity []Point

	// Fills are the fills of the run, oldest first.
	Fills []order.Fill
}

// Store persists the events of a strategy's runs.
//
// It is what makes a run's history outlive the process that recorded
// it: a live service that restarts loads its history back and carries
// on where it left off.
//
// A store is optional. Without one the tear sheets are written from
// the run's own memory and nothing is kept.
type Store interface {
	// Save records the run's events, replacing what was stored under
	// the key before. Saving the same run twice leaves one copy of it.
	Save(ctx context.Context, key string, run Run, evts Events) error

	// Load returns the run and the events stored under the key.
	//
	// A key that was never saved returns a zero Run and no events.
	Load(ctx context.Context, key string) (Run, Events, error)

	// Runs lists the runs of a strategy, newest first.
	Runs(ctx context.Context, strategy string) ([]Run, error)
}
