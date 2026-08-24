// Package tradingframework is a reusable Go trading/backtesting framework.
//
// Strategy code interacts with the framework through the Context interface,
// scheduled Jobs, and broker-neutral Order values. The same Job handlers run
// unchanged in Backtest and Live modes.
package tradingframework

type Mode int

const (
	Backtest Mode = iota
	Live
)

func (m Mode) String() string {
	if m == Live {
		return "live"
	}
	return "backtest"
}
