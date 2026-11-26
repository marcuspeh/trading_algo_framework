// Package tearsheet turns the record of a run into a performance page.
//
// A run leaves two traces behind: the portfolio's value over time, and
// the fills that moved it. This package collects both, turns them into
// trades and metrics, and renders them as a single self-contained HTML
// file.
//
// Nothing in here talks to a broker or a market: the engines report
// what happened, and this package does the arithmetic and the drawing.
package tearsheet

import (
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// Observer receives the events a tear sheet is built from.
//
// The engines report the portfolio's value through it, and the brokers
// report their fills through it.
type Observer interface {
	// OnEquity records the portfolio's value at a point in time.
	OnEquity(at time.Time, value float64)

	// OnFill records a fill.
	OnFill(f order.Fill)
}

// Point is a sample of the portfolio's value.
type Point struct {
	// Time is when the sample was taken.
	Time time.Time

	// Value is the portfolio's value then.
	Value float64

	// Open is how many positions the portfolio held then. It is what
	// the exposure is measured from.
	Open int
}

// Trade is a round trip: an entry and the exit that closed it.
//
// A trade that is still open is not reported: there is no exit to
// report it against.
type Trade struct {
	// Symbol is the instrument that traded.
	Symbol string

	// Side is the direction of the entry: "LONG" or "SHORT".
	Side string

	// Quantity is how much traded.
	Quantity float64

	// EntryTime and ExitTime bracket the trade.
	EntryTime time.Time
	ExitTime  time.Time

	// EntryPrice and ExitPrice are the prices it traded at.
	EntryPrice float64
	ExitPrice  float64

	// PnL is the profit or loss in account currency, after the
	// commission both sides paid.
	PnL float64

	// ReturnPct is the PnL as a percentage of the capital the trade
	// took, in percentage points: 5.2 means 5.2%.
	ReturnPct float64

	// Duration is how long the trade was open.
	Duration time.Duration
}

// Monthly is the return of one calendar month.
type Monthly struct {
	// Month is the month, as "2006-01".
	Month string

	// Return is the month's return as a fraction: 0.05 is 5%.
	Return float64
}

// Meta describes the run the tear sheet is about.
type Meta struct {
	// Mode is the run's mode: "backtest" or "live".
	Mode string

	// Start and End bracket the run.
	Start time.Time
	End   time.Time

	// Starting is the portfolio's value when the run began.
	Starting float64

	// Currency is the account's currency.
	Currency string

	// Generated is when the tear sheet was written.
	Generated time.Time
}

// Metrics are the numbers a tear sheet leads with.
type Metrics struct {
	StartingValue float64
	EndingValue   float64

	// TotalReturn is the whole run's return as a fraction.
	TotalReturn float64

	// CAGR is the annualised return, compounded over the run.
	CAGR float64

	// Volatility is the annualised standard deviation of the daily
	// returns.
	Volatility float64

	// Sharpe is the annualised return per unit of volatility. The
	// risk-free rate is zero.
	Sharpe float64

	// Sortino is the annualised return per unit of downside volatility.
	Sortino float64

	// MaxDrawdown is the deepest peak-to-trough fall as a positive
	// fraction: 0.12 is a 12% drawdown.
	MaxDrawdown float64

	// MaxDrawdownFrom and MaxDrawdownTo bracket the deepest
	// drawdown.
	MaxDrawdownFrom time.Time
	MaxDrawdownTo   time.Time

	// WinRate is the share of closed trades that made money.
	WinRate float64

	// ProfitFactor is the gross profit over the gross loss.
	ProfitFactor float64

	// AvgWin and AvgLoss are the average winning and losing trade.
	AvgWin  float64
	AvgLoss float64

	// BestTrade and WorstTrade are the largest single win and loss.
	BestTrade  float64
	WorstTrade float64

	// TotalTrades is the number of closed trades.
	TotalTrades int

	// OpenPositions is how many positions were still open at the end.
	OpenPositions int

	// Exposure is the share of the samples that had a position open.
	Exposure float64

	// TotalFees is the commission paid over the run.
	TotalFees float64
}

// Report is everything a tear sheet is made of.
type Report struct {
	Meta Meta

	// Equity is every sample of the portfolio's value, in the order
	// they were taken.
	Equity []Point

	// Daily is the last sample of each trading day. The charts and the
	// metrics are built from it: a day is the unit a return is
	// measured in.
	Daily []Point

	// Drawdown is the fall from the running peak of the daily series,
	// as a fraction of the peak. It is never positive.
	Drawdown []Point

	Trades  []Trade
	Monthly []Monthly
	Metrics Metrics
}
