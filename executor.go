package tradingframework

import "context"

type OrderResult struct {
	ID        string
	Filled    bool
	FillPrice float64
	Error     error
}

// Executor is the framework-level order-submission abstraction.
// Two implementations exist: backtest (deterministic fills) and
// live (delegates to a broker client).
type Executor interface {
	Submit(ctx context.Context, order Order) (OrderResult, error)
	Positions(ctx context.Context) ([]Position, error)
	OpenOrders(ctx context.Context) ([]Order, error)
	Cancel(ctx context.Context, orderID string) error
}
