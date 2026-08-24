package tradingframework

import (
	"errors"
	"time"
)

// Context is the primary interface between strategy code and the framework.
//
// Strategy code must not know how market data is retrieved, where
// constituents are stored, how the broker works, how backtest fills work,
// or how configuration is retrieved. All of that lives behind Context.
type Context interface {
	// Date returns the current simulated or live trading date.
	// The time component is zero in backtests.
	Date() time.Time

	// Now returns the current simulated or live wall-clock time.
	Now() time.Time

	// Bars returns the most recent N daily bars for a symbol, oldest first.
	Bars(symbol string, lookback int) ([]Bar, error)

	// Constituents returns the ticker symbols that currently make up the
	// given ETF or index universe.
	Constituents(etf string) ([]string, error)

	// IsTradingDay reports whether the given date is a trading day.
	// The framework intentionally does not decide this; strategies do.
	IsTradingDay(date time.Time) bool

	// Position returns the current position in a symbol.
	Position(symbol string) Position

	// Positions returns all current positions.
	Positions() []Position

	// Cash returns the current available cash.
	Cash() float64

	// PortfolioValue returns the current total portfolio value.
	PortfolioValue() float64

	// Mode returns whether the framework is running a Backtest or Live.
	Mode() Mode

	// Config returns the configuration interface for reading strategy
	// parameters and feature flags from the existing config client.
	Config() Config

	// Logger returns the structured logger for the current run.
	Logger() Logger
}

// ErrNotFound is returned by Context implementations when a requested
// resource is not available.
var ErrNotFound = errors.New("not found")