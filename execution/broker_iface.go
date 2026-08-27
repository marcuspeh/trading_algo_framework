package execution

import "context"

// BrokerClient is the minimal surface the live Executor needs from a
// broker client. The concrete *broker.BrokerClient satisfies it directly.
type BrokerClient interface {
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error)
	PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error)
	CancelOrder(ctx context.Context, orderID int) error
	GetPositions(ctx context.Context) ([]PositionResponse, error)
	GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error)
	GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error)
}

type MarketOrderRequest struct {
	Symbol        string
	Action        string
	TotalQuantity float64
	OrderRef      string
	OutsideRth    bool
}

type LimitOrderRequest struct {
	MarketOrderRequest
	LmtPrice float64
}

type OrderResponse struct {
	PermID  int
	OrderID int
	Status  string
}

type PositionResponse struct {
	ConID       int
	Symbol      string
	Quantity    float64
	AverageCost float64
}

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

type AccountNAVResponse struct {
	Account  string
	NAV      float64
	Currency string
}
