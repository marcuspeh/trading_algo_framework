// Package broker defines the canonical request/response types and the
// Client interface used to talk to a trading broker.
//
// These types are the single source of truth for the broker wire format.
// The clients/ package implements Client over HTTP, and the execution
// package consumes Client. Both speak the same types, so a concrete
// broker client can be handed straight to app.WithLive.
package broker

import "context"

// Client is the minimal surface the framework needs from a broker.
type Client interface {
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error)
	CancelOrder(ctx context.Context, orderID int) error
	GetPositions(ctx context.Context) ([]PositionResponse, error)
	GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error)
	GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error)
}

// OrderRequest is the common body of a broker order.
type OrderRequest struct {
	Symbol        string  `json:"symbol"`
	Action        string  `json:"action"`
	TotalQuantity float64 `json:"totalQuantity"`
	OrderRef      string  `json:"orderRef,omitempty"`
	OutsideRTH    bool    `json:"outsideRth"`
}

// MarketOrderRequest is a market order.
type MarketOrderRequest struct {
	OrderRequest
}

// LimitOrderRequest is a limit order.
type LimitOrderRequest struct {
	OrderRequest
	LmtPrice float64 `json:"lmtPrice"`
}

// BracketOrderRequest is an order with attached take-profit and
// stop-loss legs.
type BracketOrderRequest struct {
	OrderRequest
	LmtPrice float64 `json:"lmtPrice"`
	TPPrice  float64 `json:"tpPrice"`
	SLPrice  float64 `json:"slPrice"`
}

// UpdateOrderRequest carries the mutable fields of a resting order.
type UpdateOrderRequest struct {
	LmtPrice      *float64 `json:"lmtPrice,omitempty"`
	AuxPrice      *float64 `json:"auxPrice,omitempty"`
	TotalQuantity *float64 `json:"totalQuantity,omitempty"`
}

// OrderResponse is the broker's acknowledgement of a placed order.
type OrderResponse struct {
	PermID  int    `json:"permId"`
	OrderID int    `json:"orderId"`
	Status  string `json:"status"`
}

// BracketOrderResponse acknowledges a bracket order and its legs.
type BracketOrderResponse struct {
	ParentPermID  int    `json:"parentPermId"`
	ParentOrderID int    `json:"parentOrderId"`
	Status        string `json:"status"`
}

// PositionResponse is a single broker-held position.
type PositionResponse struct {
	ConID       int     `json:"conId"`
	Symbol      string  `json:"symbol"`
	Quantity    float64 `json:"quantity"`
	AverageCost float64 `json:"averageCost"`
}

// AccountNAVResponse is the account's net asset value.
type AccountNAVResponse struct {
	Account  string  `json:"account"`
	NAV      float64 `json:"nav"`
	Currency string  `json:"currency"`
}

// OpenOrderResponse is a resting broker order.
type OpenOrderResponse struct {
	PermID        int      `json:"permId"`
	OrderID       int      `json:"orderId"`
	Symbol        string   `json:"symbol"`
	Action        string   `json:"action"`
	TotalQuantity float64  `json:"totalQuantity"`
	LmtPrice      *float64 `json:"lmtPrice,omitempty"`
	AuxPrice      *float64 `json:"auxPrice,omitempty"`
	Status        string   `json:"status"`
	Remaining     float64  `json:"remaining"`
}

// OHLCResponse is a daily bar as returned by the broker's OHLC endpoint.
type OHLCResponse struct {
	Symbol string  `json:"symbol"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Date   string  `json:"date"`
}
