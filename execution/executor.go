// Package execution provides the Executor interface and its live and
// backtest implementations.
package execution

import (
	"context"

	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/order"
)

type Executor interface {
	Submit(ctx context.Context, o order.Order) (order.Result, error)
	Positions(ctx context.Context) ([]order.Position, error)
	OpenOrders(ctx context.Context) ([]order.Order, error)
	Cancel(ctx context.Context, orderID string) error
	// Update changes the limit price of a resting order.
	Update(ctx context.Context, orderID string, limit float64) error
}

// BarProvider supplies the daily bar for a symbol on a given date.
//
// It lives in the market package, where the market-data contract
// belongs; the alias keeps the name available here.
type BarProvider = market.BarProvider
