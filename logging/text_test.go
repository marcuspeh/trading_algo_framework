package logging_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/marcuspeh/trading-framework/logging"
)

func TestTextLogger_WritesLeveledLines(t *testing.T) {
	var buf bytes.Buffer
	l := logging.NewTextLogger(&buf)

	l.Info("hello", "k", "v", "n", 1)
	l.Warn("careful", "reason", "x")
	l.Error("boom")

	out := buf.String()
	for _, want := range []string{"INFO hello k=v n=1", "WARN careful reason=x", "ERROR boom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestTextLogger_HandlesOddFields(t *testing.T) {
	var buf bytes.Buffer
	l := logging.NewTextLogger(&buf)
	l.Info("odd", "only-one")
	out := buf.String()
	if !strings.Contains(out, "odd extra=only-one") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestNopLogger_NoOutput(t *testing.T) {
	n := logging.Nop()
	n.Info("anything")
	n.Error("anything")
}
