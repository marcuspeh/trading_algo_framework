// Package indicator bridges the framework's market data to the
// indicators-go library.
//
// The indicator implementations live in
// github.com/marcuspeh/indicators-go. This package only adapts the
// framework's order.Bar type to the library's indicator.Bar type, and
// provides convenience helpers for feeding a series of bars into an
// indicator.
//
//	sma := indicator.NewSMA(20)
//	bars, _ := ctx.Bars("AAPL", 50)
//	value := indicator.Feed(sma, "AAPL", bars)
package indicator

import (
	indicators "github.com/marcuspeh/indicators-go"
	"github.com/marcuspeh/trading-framework/order"
)

// Indicator is the interface every indicator in the library implements.
type Indicator = indicators.Indicator

// Bar is the library's bar type.
type Bar = indicators.Bar

// The indicators that produce a single value per bar are re-exported
// as-is, so strategy code only needs to import this package.
type (
	SMA  = indicators.SMA
	EMA  = indicators.EMA
	RSI  = indicators.RSI
	MACD = indicators.MACD
	ATR  = indicators.ATR
	OBV  = indicators.OBV
	ROC  = indicators.ROC
)

// NewSMA returns a simple moving average indicator.
func NewSMA(period int) *SMA { return indicators.NewSMA(period) }

// NewEMA returns an exponential moving average indicator.
func NewEMA(period int) *EMA { return indicators.NewEMA(period) }

// NewRSI returns a relative strength index indicator.
func NewRSI(period int) *RSI { return indicators.NewRSI(period) }

// NewMACD returns a moving average convergence/divergence indicator.
func NewMACD(fast, slow, signal int) *MACD { return indicators.NewMACD(fast, slow, signal) }

// NewATR returns an average true range indicator.
func NewATR(period int) *ATR { return indicators.NewATR(period) }

// NewOBV returns an on-balance volume indicator.
func NewOBV() *OBV { return indicators.NewOBV() }

// NewROC returns a rate-of-change indicator.
func NewROC(period int) *ROC { return indicators.NewROC(period) }

// FromOrderBar converts a framework bar into a library bar.
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

// MultiValueIndicator is an indicator that produces several values from
// a single bar.
type MultiValueIndicator interface {
	UpdateBands(bar *Bar) (float64, float64, float64)
}

// FeedBars pushes framework bars into an indicator that returns several
// values per bar.
func FeedBars(ind MultiValueIndicator, symbol string, bars []order.Bar) (float64, float64, float64) {
	var a, b, c float64
	for _, bar := range bars {
		a, b, c = ind.UpdateBands(ptr(FromOrderBar(symbol, bar)))
	}
	return a, b, c
}

// Bollinger adapts the library's BollingerBands to the
// MultiValueIndicator interface.
type Bollinger struct {
	*indicators.BollingerBands
}

// NewBollinger returns a Bollinger Bands adapter.
func NewBollinger(period int, stdDev float64) *Bollinger {
	return &Bollinger{BollingerBands: indicators.NewBollingerBands(period, stdDev)}
}

// UpdateBands feeds a bar into the bands and returns the upper, middle
// and lower band values.
func (b *Bollinger) UpdateBands(bar *Bar) (float64, float64, float64) {
	return b.BollingerBands.Update(bar)
}

// MinMax adapts the library's MinMax to the MultiValueIndicator
// interface.
type MinMax struct {
	*indicators.MinMax
}

// NewMinMax returns a MinMax adapter.
func NewMinMax(period int) *MinMax {
	return &MinMax{MinMax: indicators.NewMinMax(period)}
}

// UpdateBands feeds a bar into the tracker and returns the minimum and
// maximum values.
func (m *MinMax) UpdateBands(bar *Bar) (float64, float64, float64) {
	low, high := m.MinMax.Update(bar)
	return low, high, high
}

func ptr(b Bar) *Bar { return &b }
