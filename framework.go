// Package tradingframework is a reusable Go trading/backtesting framework.
//
// Strategy code interacts with the framework through the Context interface,
// scheduled Jobs, and broker-neutral Order values. The same Job handlers
// run unchanged in Backtest and Live modes.
package tradingframework

import (
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// Re-exports for back-compat. Strategy code keeps using trading.Job,
// trading.Order, trading.Context, etc.

type Order = order.Order
type Side = order.Side
type OrderType = order.OrderType
type Position = order.Position
type Bar = order.Bar
type OrderResult = order.Result

const (
	Buy  = order.Buy
	Sell = order.Sell
)

const (
	Market = order.Market
	Limit  = order.Limit
)

type Job = job.Job
type JobHandler = job.Handler
type TimeOfDay = job.TimeOfDay
type ErrInvalidJob = job.ErrInvalidJob

// Time is a convenience constructor for TimeOfDay.
func Time(hour, minute int) job.TimeOfDay {
	return job.Time(hour, minute)
}

// Context is the primary interface between strategy code and the framework.
// Strategy code must not know how market data, constituents, broker, fills,
// or configuration work — all of that lives behind Context.
type Context = job.Context

// Logger is the structured logger interface exposed to strategy code.
type Logger = job.Logger

// Config is the strategy-facing configuration interface.
type Config = job.Config

// Watcher is the live config watcher.
type Watcher = job.Watcher

// ErrNotFound is returned when a Context resource is unavailable.
var ErrNotFound = job.ErrNotFound
