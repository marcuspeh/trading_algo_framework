// Package indicator provides streaming technical-analysis indicators.
//
// Every indicator consumes one bar at a time and exposes the latest
// value. The values are validated against the reference TA-Lib
// implementation in validation_test.go.
package indicator

// Indicator defines the interface for a technical analysis indicator.
type Indicator interface {
	// Update adds a new price bar to the indicator and returns its latest calculated value.
	Update(bar *Bar) float64
	// Value returns the most recently calculated value of the indicator.
	Value() float64
	// IsReady checks if the indicator has enough historical data to produce a valid value.
	IsReady() bool
}

// Compile-time checks that every indicator satisfies the interface.
var (
	_ Indicator = (*SMA)(nil)
	_ Indicator = (*EMA)(nil)
	_ Indicator = (*RSI)(nil)
	_ Indicator = (*MACD)(nil)
	_ Indicator = (*BollingerBands)(nil)
	_ Indicator = (*ATR)(nil)
	_ Indicator = (*OBV)(nil)
	_ Indicator = (*ROC)(nil)
	_ Indicator = (*MinMax)(nil)
)
