// Package market defines the interfaces the framework uses to fetch
// historical bars. The clients/ package supplies the real adapter.
package market

import (
	"context"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

type Client interface {
	// Bars returns up to `lookback` bars for a symbol, ending on or
	// before asOf.
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error)

	// BarsBatch returns up to `lookback` bars for each of the symbols,
	// ending on or before asOf.
	//
	// The symbols share one window, so they are asked for together
	// rather than one request each. A symbol the client could not
	// serve is left out of the map; the error is for a request the
	// client could not make at all.
	BarsBatch(ctx context.Context, symbols []string, lookback int, asOf time.Time) (map[string][]order.Bar, error)
}

// BarProvider supplies the bar for a symbol on a given date.
//
// It is how a fill model reads prices: the backtest fills an order
// against the bar of the session it belongs to. The market-data
// adapter implements it, so the fill model and the strategy see the
// same prices.
type BarProvider interface {
	Bar(symbol string, date time.Time) (order.Bar, bool)
}
