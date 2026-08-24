// Package live provides the live-trading Executor, reconciliation
// logic, and engine loop. The package depends on a small BrokerClient
// interface (defined here) so the framework can be tested without
// touching the real IBKR HTTP client.
package live

import (
	"context"
	"strconv"
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

// BrokerClient is the minimal surface the live engine needs from the
// broker client. The framework's NewApp accepts anything satisfying
// this interface; the existing *broker.BrokerClient satisfies it
// directly.
type BrokerClient interface {
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error)
	CancelOrder(ctx context.Context, orderID int) error
	GetPositions(ctx context.Context) ([]PositionResponse, error)
	GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error)
	GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error)
}

// MarketOrderRequest mirrors the broker client's request shape.
type MarketOrderRequest struct {
	Symbol        string
	Action        string
	TotalQuantity float64
	OrderRef      string
	OutsideRth    bool
}

// LimitOrderRequest mirrors the broker client's request shape.
type LimitOrderRequest struct {
	MarketOrderRequest
	LmtPrice float64
}

// OrderResponse mirrors the broker client's response shape.
type OrderResponse struct {
	PermID  int
	OrderID int
	Status  string
}

// PositionResponse mirrors the broker client's response shape.
type PositionResponse struct {
	ConID       int
	Symbol      string
	Quantity    float64
	AverageCost float64
}

// OpenOrderResponse mirrors the broker client's response shape.
type OpenOrderResponse struct {
	PermID        int
	OrderID       int
	Symbol        string
	Action        string
	TotalQuantity float64
	LmtPrice      *float64
	AuxPrice      *float64
	Status        string
	Remaining     float64
}

// AccountNAVResponse mirrors the broker client's response shape.
type AccountNAVResponse struct {
	Account  string
	NAV      float64
	Currency string
}

// Clock exposes the wall-clock for the live engine.
type Clock interface {
	Now() time.Time
}

// RealClock uses time.Now().
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// toFrameworkPosition converts a broker position to the framework's
// Position type.
func toFrameworkPosition(p PositionResponse) tf.Position {
	return tf.Position{Symbol: p.Symbol, Quantity: p.Quantity, AvgPrice: p.AverageCost}
}

// toFrameworkOpenOrder converts a broker open order to the framework's
// Order type.
func toFrameworkOpenOrder(o OpenOrderResponse) tf.Order {
	ord := tf.Order{
		ID:       strconv.Itoa(o.OrderID),
		Symbol:   o.Symbol,
		Quantity: o.TotalQuantity,
		Type:     tf.Market,
	}
	switch o.Action {
	case "BUY":
		ord.Side = tf.Buy
	case "SELL":
		ord.Side = tf.Sell
	}
	if o.LmtPrice != nil {
		limit := *o.LmtPrice
		ord.Type = tf.Limit
		ord.Limit = &limit
	}
	return ord
}
