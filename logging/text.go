// Package logging provides the framework's human-readable TXT file
// logger suitable for both backtests and live runs.
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

// TextLogger writes one log line per call. Each line begins with a
// RFC3339Nano timestamp, the level, and the message. Any structured
// fields are appended as key=value pairs.
type TextLogger struct {
	mu     sync.Mutex
	w      io.Writer
	now    func() time.Time
	closed bool
}

// NewTextLogger returns a TextLogger that writes to w.
func NewTextLogger(w io.Writer) *TextLogger {
	return &TextLogger{w: w, now: time.Now}
}

// NewFileLogger returns a TextLogger that appends to path, creating it
// if necessary.
func NewFileLogger(path string) (*TextLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return NewTextLogger(f), nil
}

// Info logs an informational message.
func (l *TextLogger) Info(msg string, fields ...any) { l.write("INFO", msg, fields) }

// Warn logs a warning.
func (l *TextLogger) Warn(msg string, fields ...any) { l.write("WARN", msg, fields) }

// Error logs an error.
func (l *TextLogger) Error(msg string, fields ...any) { l.write("ERROR", msg, fields) }

// Close releases the underlying writer when it implements io.Closer.
func (l *TextLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if c, ok := l.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// SetClock replaces the time source. Tests use this to make logs
// deterministic.
func (l *TextLogger) SetClock(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

func (l *TextLogger) write(level, msg string, fields []any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := l.now().UTC().Format(time.RFC3339Nano)
	var b strings.Builder
	b.WriteString(ts)
	b.WriteByte(' ')
	b.WriteString(level)
	b.WriteByte(' ')
	b.WriteString(msg)

	if len(fields) > 0 {
		for i := 0; i+1 < len(fields); i += 2 {
			fmt.Fprintf(&b, " %v=%v", fields[i], fields[i+1])
		}
		if len(fields)%2 == 1 {
			fmt.Fprintf(&b, " extra=%v", fields[len(fields)-1])
		}
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(l.w, b.String())
}

// nopLogger is provided for tests and the App's default fallback.
type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Nop returns a Logger that discards all output.
func Nop() tf.Logger { return nopLogger{} }
