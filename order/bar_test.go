package order_test

import (
	"testing"
	"time"

	indicators "github.com/marcuspeh/indicators-go"
	"github.com/marcuspeh/trading-framework/order"
)

// TestBarIsLibraryBar proves the framework's bar type is the same type
// as the library's, so bars can be handed to an indicator without any
// conversion.
func TestBarIsLibraryBar(t *testing.T) {
	var framework order.Bar
	var library indicators.Bar = framework
	_ = library

	// A library bar can be used where a framework bar is expected.
	var back order.Bar = indicators.Bar{Close: 1}
	if back.Close != 1 {
		t.Fatalf("close=%f want 1", back.Close)
	}
}

// TestBarFeedsIndicatorDirectly proves a framework bar can be pushed
// straight into an indicator.
func TestBarFeedsIndicatorDirectly(t *testing.T) {
	bars := []order.Bar{
		{Timestamp: time.Now(), Close: 10},
		{Timestamp: time.Now(), Close: 20},
		{Timestamp: time.Now(), Close: 30},
	}

	sma := indicators.NewSMA(3)
	for i := range bars {
		sma.Update(&bars[i])
	}
	if !sma.IsReady() {
		t.Fatal("SMA should be ready")
	}
	if v := sma.Value(); v != 20 {
		t.Fatalf("SMA=%f want 20", v)
	}
}

// TestBarCarriesAllFields checks the bar shape the framework exposes.
func TestBarCarriesAllFields(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	bar := order.Bar{
		Symbol:    "AAPL",
		Timestamp: ts,
		Open:      1, High: 2, Low: 3, Close: 4, Volume: 500,
	}
	if bar.Symbol != "AAPL" {
		t.Fatalf("symbol=%q", bar.Symbol)
	}
	if !bar.Timestamp.Equal(ts) {
		t.Fatalf("timestamp=%v want %v", bar.Timestamp, ts)
	}
	if bar.Open != 1 || bar.High != 2 || bar.Low != 3 || bar.Close != 4 {
		t.Fatalf("prices=%+v", bar)
	}
	if bar.Volume != 500 {
		t.Fatalf("volume=%f want 500", bar.Volume)
	}
}
