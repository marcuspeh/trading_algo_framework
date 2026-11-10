package indicator_test

import (
	"math"
	"testing"
	"time"

	indicators "github.com/marcuspeh/indicators-go"
	"github.com/marcuspeh/trading-framework/indicator"
	"github.com/marcuspeh/trading-framework/order"
)

// b returns a bar with the given close price.
func b(close float64) *indicators.Bar {
	return &indicators.Bar{
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
	sma := indicators.NewSMA(3)
	if sma.IsReady() {
		t.Fatal("SMA should not be ready before enough bars")
	}
	sma.Update(b(10))
	sma.Update(b(20))
	if sma.IsReady() {
		t.Fatal("SMA should not be ready after 2 of 3 bars")
	}
	v := sma.Update(b(30))
	if !sma.IsReady() {
		t.Fatal("SMA should be ready after 3 bars")
	}
	if v != 20 {
		t.Fatalf("SMA=%f want 20", v)
	}
	// The window slides: (20 + 30 + 40) / 3.
	v = sma.Update(b(40))
	if math.Abs(v-30) > 1e-9 {
		t.Fatalf("SMA=%f want 30", v)
	}
}

func TestEMA_TracksClose(t *testing.T) {
	ema := indicators.NewEMA(3)
	ema.Update(b(10))
	ema.Update(b(10))
	v := ema.Update(b(10))
	if !ema.IsReady() {
		t.Fatal("EMA should be ready")
	}
	if math.Abs(v-10) > 1e-9 {
		t.Fatalf("EMA=%f want 10", v)
	}
}

func TestRSI_AllGainsIsHundred(t *testing.T) {
	rsi := indicators.NewRSI(3)
	for i := 0; i < 5; i++ {
		rsi.Update(b(float64(100 + i)))
	}
	if !rsi.IsReady() {
		t.Fatal("RSI should be ready")
	}
	if rsi.Value() != 100 {
		t.Fatalf("RSI=%f want 100", rsi.Value())
	}
}

func TestBollingerBands_BandsAreOrdered(t *testing.T) {
	bb := indicators.NewBollingerBands(3, 2)
	for i := 0; i < 5; i++ {
		bb.Update(b(float64(100 + i)))
	}
	if !bb.IsReady() {
		t.Fatal("BollingerBands should be ready")
	}
	up, mid, low := bb.Update(b(105))
	if !(up > mid && mid > low) {
		t.Fatalf("bands out of order: up=%f mid=%f low=%f", up, mid, low)
	}
}

func TestMinMax_TracksWindow(t *testing.T) {
	mm := indicators.NewMinMax(3)
	mm.Update(b(10))
	mm.Update(b(5))
	mm.Update(b(20))
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
	sma := indicator.NewSMA(3)
	v := indicator.Feed(sma, "AAPL", bars)
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
	bb := indicator.NewBollinger(3, 2)
	up, mid, low := indicator.FeedBars(bb, "AAPL", bars)
	if !(up > mid && mid > low) {
		t.Fatalf("bands out of order: up=%f mid=%f low=%f", up, mid, low)
	}
}

func TestFeedBars_MinMaxAdapter(t *testing.T) {
	bars := []order.Bar{
		{Time: time.Now(), Close: 10},
		{Time: time.Now(), Close: 5},
		{Time: time.Now(), Close: 20},
	}
	mm := indicator.NewMinMax(3)
	low, high, _ := indicator.FeedBars(mm, "AAPL", bars)
	if low != 5 {
		t.Fatalf("low=%f want 5", low)
	}
	if high != 20 {
		t.Fatalf("high=%f want 20", high)
	}
}

func TestFromOrderBar_CopiesFields(t *testing.T) {
	src := order.Bar{
		Time: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Open: 1, High: 2, Low: 3, Close: 4, Volume: 500,
	}
	got := indicator.FromOrderBar("AAPL", src)
	if got.Symbol != "AAPL" {
		t.Fatalf("symbol=%q", got.Symbol)
	}
	if !got.Timestamp.Equal(src.Time) {
		t.Fatalf("timestamp=%v want %v", got.Timestamp, src.Time)
	}
	if got.Open != 1 || got.High != 2 || got.Low != 3 || got.Close != 4 {
		t.Fatalf("prices=%+v", got)
	}
	if got.Volume != 500 {
		t.Fatalf("volume=%f want 500", got.Volume)
	}
}

// TestUpstreamIndicatorsSatisfyFrameworkInterface proves the library's
// indicators can be used through the framework's Indicator alias.
func TestUpstreamIndicatorsSatisfyFrameworkInterface(t *testing.T) {
	var inds []indicator.Indicator
	inds = append(inds,
		indicator.NewSMA(3),
		indicator.NewEMA(3),
		indicator.NewRSI(3),
		indicator.NewMACD(2, 3, 2),
		indicator.NewATR(2),
		indicator.NewOBV(),
		indicator.NewROC(2),
	)
	for _, ind := range inds {
		if ind == nil {
			t.Fatal("nil indicator")
		}
	}
}
