package tearsheet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// sampleReport is a run with a little of everything in it.
func sampleReport() Report {
	r := NewRecorder()
	r.OnEquity(day(1), 10_000)
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Price: 100, Time: day(1)})
	r.OnEquity(day(2), 10_500)
	r.OnFill(order.Fill{Symbol: "AAPL", Side: order.Sell, Quantity: 10, Price: 110, Time: day(2)})
	r.OnEquity(day(3), 11_000)
	r.OnEquity(day(4), 10_500)
	r.OnEquity(day(5), 12_000)

	return r.Report(Meta{
		Mode:      "backtest",
		Start:     day(1),
		End:       day(5),
		Starting:  10_000,
		Currency:  "USD",
		Generated: time.Date(2025, 1, 6, 9, 0, 0, 0, time.UTC),
	})
}

func TestRender_WritesAPageWithTheRunOnIt(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, sampleReport()); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := b.String()
	for _, want := range []string{
		"<!doctype html>",
		"Total return",
		"Portfolio value",
		"Drawdown",
		"Monthly returns",
		"Trades",
		"AAPL",
		"prefers-color-scheme: dark",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("expected the page to contain %q", want)
		}
	}
}

// TestRender_EmbedsTheChartDataAsJSON guards the chart's data payload:
// it has to reach the script as JSON, not as a quoted string.
func TestRender_EmbedsTheChartDataAsJSON(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, sampleReport()); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := b.String()
	if !strings.Contains(page, `[{"x":`) {
		t.Error("expected the chart data to be embedded as JSON")
	}
	if strings.Contains(page, `\"x\":`) {
		t.Error("expected the chart data not to be escaped as a string")
	}
}

func TestRender_FetchesNothingFromTheNetwork(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, sampleReport()); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := b.String()
	for _, unwanted := range []string{"http://", "https://", "<link", "src="} {
		if strings.Contains(page, unwanted) {
			t.Errorf("expected the page to be self-contained, found %q", unwanted)
		}
	}
}

func TestRender_SurvivesARunWithNothingInIt(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, Report{Meta: Meta{Mode: "live"}}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(b.String(), "Total return") {
		t.Error("expected an empty run to still render a page")
	}
}

func TestWriter_WritesTheFile(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, func(day time.Time) string {
		return "live-" + day.Format("2006-01-02") + ".html"
	}, nil)
	w.Meta(Meta{Mode: "live", Currency: "USD"})
	w.OnEquity(day(1), 10_000)
	w.OnEquity(day(2), 10_500)

	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	path := filepath.Join(dir, "live-2025-01-02.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "$10,500.00") {
		t.Error("expected the tear sheet to carry the ending value")
	}
}

// TestWriter_WritesTheDayThatEndedBeforeTheNextOneStarts checks that a
// day's tear sheet holds that day, and not the one that followed it.
func TestWriter_WritesTheDayThatEnded(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, func(day time.Time) string {
		return "live-" + day.Format("2006-01-02") + ".html"
	}, nil)
	w.Meta(Meta{Mode: "live", Currency: "USD"})

	w.OnEquity(day(1), 10_000)
	w.OnEquity(day(2), 10_500)
	// The third day starts, which ends the second.
	w.OnEquity(day(3), 20_000)

	body, err := os.ReadFile(filepath.Join(dir, "live-2025-01-02.html"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "$10,500.00") {
		t.Error("expected the second day's tear sheet to hold the second day")
	}
	if strings.Contains(string(body), "$20,000.00") {
		t.Error("expected the second day's tear sheet to stop before the third day")
	}
}
