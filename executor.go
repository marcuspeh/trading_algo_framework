package tradingframework

import "context"

// OrderResult is the result of a submitted Order.
type OrderResult struct {
	// ID is the framework-assigned ID (live mode uses broker IDs).
	ID string
	// Filled is true if the order executed (relevant for backtests).
	Filled bool
	// FillPrice is the price at which the order filled. Zero if unfilled.
	FillPrice float64
	// Error is set if submission failed.
	Error error
}

// Executor is the framework-level abstraction over order submission.
//
// Two implementations exist:
//
//   - BacktestExecutor: deterministic fills against bar data.
//   - LiveExecutor:     delegates to the broker client in clients/.
type Executor interface {
	Submit(ctx context.Context, order Order) (OrderResult, error)
	Positions(ctx context.Context) ([]Position, error)
	OpenOrders(ctx context.Context) ([]Order, error)
	Cancel(ctx context.Context, orderID string) error
}