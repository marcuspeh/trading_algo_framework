// Package tradingframework is a reusable Go trading/backtesting framework.
//
// Strategy code interacts with the framework through the Context interface,
// scheduled Jobs, and broker-neutral Order values. The same Job handlers run
// unchanged in Backtest and Live modes.
package tradingframework

// Mode identifies how the framework is executing a run.
type Mode int

const (
	// Backtest mode simulates time and fills.
	Backtest Mode = iota
	// Live mode executes against a real broker (e.g. IBKR).
	Live
)

// String returns a human-readable identifier for the mode.
func (m Mode) String() string {
	switch m {
	case Live:
		return "live"
	default:
		return "backtest"
	}
}