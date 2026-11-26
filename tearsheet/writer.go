package tearsheet

import (
	"bytes"
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
	defer w.mu.Unlock()

	if !w.day.IsZero() && !sameDay(w.day, at) {
		if err := w.write(w.day); err != nil {
			w.log.Warn("tearsheet not written", "err", err.Error())
		}
	}

	w.rec.OnEquity(at, value)
	w.day = at
}

// OnFill records a fill.
func (w *Writer) OnFill(f order.Fill) {
	w.rec.OnFill(f)
}

// Flush writes the tear sheet for the day the run is in.
//
// A run that ended on a day it never sampled still gets a file: an
// empty tear sheet says more than no file at all.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := w.day
	if day.IsZero() {
		day = time.Now()
	}
	return w.write(day)
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
