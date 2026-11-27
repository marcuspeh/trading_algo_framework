package tearsheet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// Logger records what the writer does.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
}

// Writer writes tear sheets to a directory.
//
// It is the Observer the engines report to: it collects the run's
// events, and writes a tear sheet when the run reaches the end of a
// day.
//
// The file name comes from the caller, because a backtest and a live
// run name their files differently: a backtest is one run and one
// file, a live run writes one file per day.
type Writer struct {
	mu   sync.Mutex
	dir  string
	name func(day time.Time) string
	log  Logger
	rec  *Recorder
	meta Meta
	day  time.Time

	// store keeps the run's events, and key says which run of which
	// strategy they belong to. Both are empty when the run is not
	// persisted.
	store Store
	key   string
	run   Run
}

// NewWriter returns a Writer that writes into dir.
//
// The name function names the file for a day. The logger is optional.
func NewWriter(dir string, name func(day time.Time) string, log Logger) *Writer {
	if log == nil {
		log = nopLogger{}
	}
	return &Writer{
		dir:  dir,
		name: name,
		log:  log,
		rec:  NewRecorder(),
	}
}

// WithStore keeps the run's events in a store.
//
// The key says which run of which strategy the events belong to, so a
// service that restarts finds its own history back and a strategy
// never sees another one's.
//
// The store is optional: without one the run's events live in the
// writer's memory and are gone when the process stops.
func (w *Writer) WithStore(store Store, key string, run Run) *Writer {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.store = store
	w.key = key
	w.run = run
	return w
}

// Load reads the run's history back from the store.
//
// It is what a live service does after it restarts: the events it
// recorded before are put back, so the tear sheet carries the whole
// run and not only the part since the restart.
//
// Loading a run that was never stored is not an error: the history
// starts empty.
func (w *Writer) Load(ctx context.Context) error {
	w.mu.Lock()
	store, key := w.store, w.key
	w.mu.Unlock()

	if store == nil {
		return nil
	}

	run, evts, err := store.Load(ctx, key)
	if err != nil {
		return err
	}
	if len(evts.Equity) == 0 && len(evts.Fills) == 0 {
		return nil
	}

	w.mu.Lock()
	w.rec.Restore(evts)
	// The run that was loaded is the run being continued.
	if !run.Start.IsZero() {
		w.run.Start = run.Start
	}
	w.mu.Unlock()

	w.log.Info("tearsheet history loaded",
		"key", key,
		"equity_points", len(evts.Equity),
		"fills", len(evts.Fills),
	)
	return nil
}

// save records the run's events in the store.
//
// A store that cannot be written is worth a warning, not a failed
// run: the trading is what matters, and it already happened.
func (w *Writer) save(ctx context.Context) {
	w.mu.Lock()
	store, key, run := w.store, w.key, w.run
	if store == nil {
		w.mu.Unlock()
		return
	}
	evts := w.rec.Events()
	w.mu.Unlock()

	if err := store.Save(ctx, key, run, evts); err != nil {
		w.log.Warn("tearsheet history not saved", "key", key, "err", err.Error())
	}
}

// Meta sets the metadata every tear sheet of the run carries.
//
// The bounds and the starting value are filled in from the run's own
// samples when they are not set.
func (w *Writer) Meta(meta Meta) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.meta = meta
}

// OnEquity records the portfolio's value.
//
// When the day rolls over, the day that ended is written out before
// the new day's first sample joins the series: a tear sheet for a day
// holds that day, and not the one that followed it.
func (w *Writer) OnEquity(at time.Time, value float64) {
	w.mu.Lock()

	if !w.day.IsZero() && !sameDay(w.day, at) {
		if err := w.write(w.day); err != nil {
			w.log.Warn("tearsheet not written", "err", err.Error())
		}
	}

	w.rec.OnEquity(at, value)
	w.day = at
	w.mu.Unlock()

	w.save(context.Background())
}

// OnFill records a fill.
func (w *Writer) OnFill(f order.Fill) {
	w.mu.Lock()
	w.rec.OnFill(f)
	w.mu.Unlock()

	w.save(context.Background())
}

// Flush writes the tear sheet for the day the run is in.
//
// A run that ended on a day it never sampled still gets a file: an
// empty tear sheet says more than no file at all.
func (w *Writer) Flush() error {
	w.mu.Lock()

	day := w.day
	if day.IsZero() {
		day = time.Now()
	}
	err := w.write(day)
	w.mu.Unlock()

	w.save(context.Background())
	return err
}

// Report builds the tear sheet's report from what the writer has
// collected.
//
// It is what the writer renders, and it is how a caller reads the run's
// numbers without reading the file.
func (w *Writer) Report(meta Meta) Report {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.report()
}

// report builds the report. It is called with the lock held.
func (w *Writer) report() Report {
	rep := w.rec.Report(w.meta)
	if len(rep.Equity) > 0 {
		if rep.Meta.Start.IsZero() {
			rep.Meta.Start = rep.Equity[0].Time
		}
		if rep.Meta.End.IsZero() {
			rep.Meta.End = rep.Equity[len(rep.Equity)-1].Time
		}
		if rep.Meta.Starting == 0 {
			rep.Meta.Starting = rep.Equity[0].Value
		}
	}
	rep.Meta.Generated = time.Now()
	return rep
}

// write renders the tear sheet and puts it on disk.
//
// It is called with the lock held.
func (w *Writer) write(day time.Time) error {
	rep := w.report()

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("tearsheet: %w", err)
	}

	var buf bytes.Buffer
	if err := Render(&buf, rep); err != nil {
		return fmt.Errorf("tearsheet: %w", err)
	}

	path := filepath.Join(w.dir, w.name(day))
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("tearsheet: %w", err)
	}

	w.log.Info("tearsheet written",
		"path", path,
		"trades", rep.Metrics.TotalTrades,
		"return", rep.Metrics.TotalReturn,
	)
	return nil
}

// nopLogger discards the writer's log lines.
type nopLogger struct{}

func (nopLogger) Info(string, ...any) {}
func (nopLogger) Warn(string, ...any) {}
