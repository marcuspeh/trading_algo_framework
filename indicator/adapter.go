package indicator

import (
	"github.com/marcuspeh/trading-framework/order"
)

// FromOrderBar converts a framework bar into an indicator bar.
func FromOrderBar(symbol string, b order.Bar) Bar {
	return Bar{
		Symbol:    symbol,
		Timestamp: b.Time,
		Open:      b.Open,
		High:      b.High,
		Low:       b.Low,
		Close:     b.Close,
		Volume:    float64(b.Volume),
	}
}

// Feed pushes a sequence of framework bars into an indicator and returns
// the latest value. The bars must be in ascending time order.
//
// This is the bridge between the framework's market data and the
// indicator maths:
//
//	sma := indicator.NewSMA(20)
//	bars, _ := ctx.Bars("AAPL", 50)
//	value := indicator.Feed(sma, "AAPL", bars)
func Feed(ind Indicator, symbol string, bars []order.Bar) float64 {
	value := 0.0
	for _, b := range bars {
		value = ind.Update(ptr(FromOrderBar(symbol, b)))
	}
	return value
}

// FeedBars pushes framework bars into an indicator that returns multiple
// values per bar, such as Bollinger Bands or MinMax.
func FeedBars(ind MultiValueIndicator, symbol string, bars []order.Bar) (float64, float64, float64) {
	var a, b, c float64
	for _, bar := range bars {
		a, b, c = ind.UpdateBands(ptr(FromOrderBar(symbol, bar)))
	}
	return a, b, c
}

// MultiValueIndicator is an indicator that produces several values from
// a single bar.
type MultiValueIndicator interface {
	UpdateBands(bar *Bar) (float64, float64, float64)
}

func ptr(b Bar) *Bar { return &b }
