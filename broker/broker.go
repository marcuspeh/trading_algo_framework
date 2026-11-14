// Package broker defines the interface the framework uses to talk to a
// trading broker.
//
// The request and response types are owned by the ibkr-proxy SDK, so
// the wire format has exactly one definition. This package aliases
// them rather than restating them, which means the SDK's own client
// satisfies broker.Client with no adapter - and the framework cannot
// drift from the service it talks to.
//
// The clients/ package supplies a live client (the SDK's client with a
// deadline on every call) and broker/mock supplies the in-memory broker
// a backtest runs against.
package broker

import (
	"context"
	"time"

	ibkrproxy "github.com/marcuspeh/ibkr-proxy/sdk/go"
)

// FillReporter is implemented by a broker that fills an order at once
// and can say at what price.
//
// A live broker fills asynchronously and reports fills through its own
// channels, so it does not implement this. The mock broker fills
// immediately, and the executor uses this to report the fill price in
// a backtest.
type FillReporter interface {
	// FillPrice returns the price an order filled at.
	FillPrice(orderID int) (float64, bool)
}

// SessionAdvancer is implemented by a broker that simulates fills as
// trading sessions close.
//
// Only the mock broker implements it - a live broker fills against the
// real market - but the backtest engine needs to tell a simulated
// broker that a session has closed, so it can evaluate the orders
// resting at it.
type SessionAdvancer interface {
	// AdvanceSession evaluates the resting orders against the given
	// session's bar.
	AdvanceSession(session time.Time)
}

// Client is the surface the framework needs from a broker.
//
// It is satisfied by *ibkrproxy.BrokerClient, which talks to the
// ibkr-proxy service, and by the mock broker the backtest runs
// against.
type Client interface {
	Health(ctx context.Context) (*HealthResponse, error)
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceBracketOrder(ctx context.Context, req BracketOrderRequest, idempotencyKey string) (*BracketOrderResponse, error)
	UpdateOrder(ctx context.Context, orderID int, req UpdateOrderRequest) (*OrderResponse, error)
	CancelOrder(ctx context.Context, orderID int) error
	GetPositions(ctx context.Context) ([]PositionResponse, error)
	GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error)
	GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error)
}

// The wire types are the ibkr-proxy SDK's, aliased so the framework
// and the SDK can never disagree about the format.
type (
	// Contract groups the instrument fields shared by every order.
	Contract = ibkrproxy.Contract

	// OrderRequest is the common body of a broker order.
	OrderRequest = ibkrproxy.OrderRequest

	// MarketOrderRequest is a market order.
	MarketOrderRequest = ibkrproxy.MarketOrderRequest

	// LimitOrderRequest is a limit order. TIF is the time in force:
	// "DAY", "GTC", "IOC", "FOK" or "GTD".
	LimitOrderRequest = ibkrproxy.LimitOrderRequest

	// BracketOrderRequest is an order with attached take-profit and
	// stop-loss legs. ParentTIF governs the parent; the children are
	// always good-till-cancelled server-side.
	BracketOrderRequest = ibkrproxy.BracketOrderRequest

	// UpdateOrderRequest carries the mutable fields of a resting order.
	UpdateOrderRequest = ibkrproxy.UpdateOrderRequest

	// OrderResponse is the broker's acknowledgement of a placed order.
	OrderResponse = ibkrproxy.OrderResponse

	// BracketOrderResponse acknowledges a bracket order and its legs.
	BracketOrderResponse = ibkrproxy.BracketOrderResponse

	// PositionResponse is a single broker-held position.
	PositionResponse = ibkrproxy.PositionResponse

	// AccountNAVResponse is the account's net asset value.
	AccountNAVResponse = ibkrproxy.AccountNAVResponse

	// HealthResponse reports whether the broker is connected.
	HealthResponse = ibkrproxy.HealthResponse

	// OpenOrderResponse is a resting broker order.
	OpenOrderResponse = ibkrproxy.OpenOrderResponse
)
