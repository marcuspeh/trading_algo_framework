package live

import (
	"context"
	"fmt"
	"strconv"

	tf "github.com/marcuspeh/trading-framework"
)

// Executor is the framework Executor for live mode. It translates
// broker-neutral Orders into broker-specific calls. Local portfolio
// state is owned by the Reconciler.
type Executor struct {
	broker BrokerClient
}

func NewExecutor(b BrokerClient) *Executor {
	return &Executor{broker: b}
}

// Submit places a market or limit order with the broker. The returned
// OrderResult.ID is the broker OrderID.
func (e *Executor) Submit(ctx context.Context, order tf.Order) (tf.OrderResult, error) {
	if e.broker == nil {
		return tf.OrderResult{}, fmt.Errorf("live executor: no broker client")
	}
	if order.Symbol == "" {
		return tf.OrderResult{}, fmt.Errorf("live executor: empty symbol")
	}
	if order.Quantity <= 0 {
		return tf.OrderResult{}, fmt.Errorf("live executor: non-positive quantity")
	}

	idemKey := order.ID

	req := MarketOrderRequest{
		Symbol:        order.Symbol,
		Action:        order.Side.String(),
		TotalQuantity: order.Quantity,
	}

	var (
		res *OrderResponse
		err error
	)

	switch order.Type {
	case tf.Market:
		res, err = e.broker.PlaceMarketOrder(ctx, req, idemKey)
	case tf.Limit:
		if order.Limit == nil || *order.Limit <= 0 {
			return tf.OrderResult{}, fmt.Errorf("live executor: limit order requires a positive price")
		}
		lreq := LimitOrderRequest{MarketOrderRequest: req, LmtPrice: *order.Limit}
		res, err = e.broker.PlaceLimitOrder(ctx, lreq, idemKey)
	default:
		return tf.OrderResult{}, fmt.Errorf("live executor: unsupported order type")
	}
	if err != nil {
		return tf.OrderResult{}, err
	}
	if res == nil {
		return tf.OrderResult{}, fmt.Errorf("live executor: nil broker response")
	}
	return tf.OrderResult{
		ID:        strconv.Itoa(res.OrderID),
		Filled:    false, // the broker reports fills asynchronously; the framework does not wait.
		FillPrice: 0,
	}, nil
}

// Positions asks the broker for current positions.
func (e *Executor) Positions(ctx context.Context) ([]tf.Position, error) {
	if e.broker == nil {
		return nil, fmt.Errorf("live executor: no broker client")
	}
	bps, err := e.broker.GetPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tf.Position, 0, len(bps))
	for _, bp := range bps {
		out = append(out, toFrameworkPosition(bp))
	}
	return out, nil
}

func (e *Executor) OpenOrders(ctx context.Context) ([]tf.Order, error) {
	if e.broker == nil {
		return nil, fmt.Errorf("live executor: no broker client")
	}
	bos, err := e.broker.GetOpenOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tf.Order, 0, len(bos))
	for _, bo := range bos {
		out = append(out, toFrameworkOpenOrder(bo))
	}
	return out, nil
}

// Cancel cancels an order by broker OrderID.
func (e *Executor) Cancel(ctx context.Context, orderID string) error {
	if e.broker == nil {
		return fmt.Errorf("live executor: no broker client")
	}
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("live executor: invalid order id %q", orderID)
	}
	return e.broker.CancelOrder(ctx, id)
}
