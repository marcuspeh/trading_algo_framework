package indicator

import (
	"math"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

func bar(close float64) *Bar {
	return &Bar{
		Symbol:    "TEST",
		Timestamp: time.Now(),
		Open:      close,
		High:      close,
		Low:       close,
		Close:     close,
		Volume:    1000,
	}
}

func TestSMA_AveragesCloses(t *testing.T) {
	sma := NewSMA(3)
	if sma.IsReady() {
		t.Fatal("SMA should not be ready before enough bars")
	}
	sma.Update(bar(10))
	sma.Update(bar(20))
	if sma.IsReady() {
		t.Fatal("SMA should not be ready after 2 of 3 bars")
	}
	v := sma.Update(bar(30))
	if !sma.IsReady() {
		t.Fatal("SMA should be ready after 3 bars")
	}
	if v != 20 {
		t.Fatalf("SMA=%f want 20", v)
	}
	// The window slides: (20 + 30 + 40) / 3.
	v = sma.Update(bar(40))
	if math.Abs(v-30) > 1e-9 {
		t.Fatalf("SMA=%f want 30", v)
	}
}

func TestEMA_TracksClose(t *testing.T) {
	ema := NewEMA(3)
	ema.Update(bar(10))
	ema.Update(bar(10))
	v := ema.Update(bar(10))
	if !ema.IsReady() {
		t.Fatal("EMA should be ready")
	}
	if math.Abs(v-10) > 1e-9 {
		t.Fatalf("EMA=%f want 10", v)
	}
}

func TestRSI_AllGainsIsHundred(t *testing.T) {
	rsi := NewRSI(3)
	for i := 0; i < 5; i++ {
		rsi.Update(bar(float64(100 + i)))
	}
	if !rsi.IsReady() {
		t.Fatal("RSI should be ready")
	}
	if rsi.Value() != 100 {
		t.Fatalf("RSI=%f want 100", rsi.Value())
	}
}

func TestBollingerBands_UpdateSatisfiesIndicator(t *testing.T) {
	bb := NewBollingerBands(3, 2)
	var ind Indicator = bb
	for i := 0; i < 5; i++ {
		ind.Update(bar(float64(100 + i)))
	}
	if !ind.IsReady() {
		t.Fatal("BollingerBands should be ready")
	}
	up, mid, low := bb.UpdateBands(bar(105))
	if !(up > mid && mid > low) {
		t.Fatalf("bands out of order: up=%f mid=%f low=%f", up, mid, low)
	}
}

func TestMinMax_TracksWindow(t *testing.T) {
	mm := NewMinMax(3)
	var ind Indicator = mm
	ind.Update(bar(10))
	ind.Update(bar(5))
	ind.Update(bar(20))
	if !mm.IsReady() {
		t.Fatal("MinMax should be ready")
	}
	if mm.Min() != 5 {
		t.Fatalf("Min=%f want 5", mm.Min())
	}
	if mm.Max() != 20 {
		t.Fatalf("Max=%f want 20", mm.Max())
	}
}

func TestFeed_BridgesFrameworkBars(t *testing.T) {
	bars := []order.Bar{
		{Time: time.Now(), Close: 10, Volume: 100},
		{Time: time.Now(), Close: 20, Volume: 100},
		{Time: time.Now(), Close: 30, Volume: 100},
	}
	sma := NewSMA(3)
	v := Feed(sma, "AAPL", bars)
	if !sma.IsReady() {
		t.Fatal("SMA should be ready")
	}
	if v != 20 {
		t.Fatalf("SMA=%f want 20", v)
	}
}

func TestFeedBars_ReadsAllBands(t *testing.T) {
	bars := []order.Bar{
		{Time: time.Now(), Close: 10},
		{Time: time.Now(), Close: 20},
		{Time: time.Now(), Close: 30},
	}
	bb := NewBollingerBands(3, 2)
	up, mid, low := FeedBars(bb, "AAPL", bars)
	if !(up > mid && mid > low) {
		t.Fatalf("bands out of order: up=%f mid=%f low=%f", up, mid, low)
	}
}
