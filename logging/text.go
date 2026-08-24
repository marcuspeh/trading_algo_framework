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

// TextLogger writes one line per call. Each line begins with an
// RFC3339Nano timestamp, the level, and the message, followed by
// key=value fields.
type TextLogger struct {
	mu     sync.Mutex
	w      io.Writer
	now    func() time.Time
	closed bool
}

func NewTextLogger(w io.Writer) *TextLogger {
	return &TextLogger{w: w, now: time.Now}
}

func NewFileLogger(path string) (*TextLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return NewTextLogger(f), nil
}

func (l *TextLogger) Info(msg string, fields ...any)  { l.write("INFO", msg, fields) }
func (l *TextLogger) Warn(msg string, fields ...any)  { l.write("WARN", msg, fields) }
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

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Nop returns a Logger that discards all output.
func Nop() tf.Logger { return nopLogger{} }
