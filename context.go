package tradingframework

import (
	"errors"
	"time"
)

// Context is the primary interface between strategy code and the framework.
// Strategy code must not know how market data, constituents, broker, fills,
// or configuration work — all of that lives behind Context.
type Context interface {
	Date() time.Time
	Now() time.Time
	Bars(symbol string, lookback int) ([]Bar, error)
	Constituents(etf string) ([]string, error)
	IsTradingDay(date time.Time) bool
	Position(symbol string) Position
	Positions() []Position
	Cash() float64
	PortfolioValue() float64
	Mode() Mode
	Config() Config
	Logger() Logger
}

var ErrNotFound = errors.New("not found")
