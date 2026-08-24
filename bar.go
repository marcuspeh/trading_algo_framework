package tradingframework

import "time"

// Bar is a single OHLCV bar. For backtests the framework uses a single
// deterministic daily bar per simulated date.
type Bar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}
