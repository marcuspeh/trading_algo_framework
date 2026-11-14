package execution

import (
	"context"
	"fmt"
	"strconv"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/order"
)

// Broker is the framework's Executor.
//
// It is the only executor: a backtest and a live run differ only in
// which broker.Client they talk to. The order translation - an
// order.Order into a market, limit or bracket request - happens here,
// once, so both modes send the same thing.
type Broker struct {
	client Client
}

// NewBroker returns an Executor that submits to the given broker.
func NewBroker(c Client) *Broker {
	return &Broker{client: c}
}

// Client returns the broker the executor submits to.
func (e *Broker) Client() Client { return e.client }

// Submit places an order with the broker.
//
// The broker acknowledges the order; it reports fills asynchronously,
// so the result carries the broker's order id rather than a fill.
func (e *Broker) Submit(ctx context.Context, o order.Order) (order.Result, error) {
	if e.client == nil {
		return order.Result{}, fmt.Errorf("executor: no broker client")
	}
	if o.Symbol == "" {
		return order.Result{}, fmt.Errorf("executor: empty symbol")
	}
	if o.Quantity <= 0 {
		return order.Result{}, fmt.Errorf("executor: non-positive quantity")
	}

	// The broker takes a whole number of shares.
	qty := int(o.Quantity)
	if float64(qty) != o.Quantity {
		return order.Result{}, fmt.Errorf(
			"executor: quantity %v is not a whole number of shares", o.Quantity)
	}

	base := broker.OrderRequest{
		Contract:      broker.Contract{Symbol: o.Symbol},
		Action:        o.Side.String(),
		TotalQuantity: qty,
		OrderRef:      o.ID,
		OutsideRth:    o.OutsideRTH,
	}

	key := idempotencyKey(o)

	switch {
	case o.IsBracket():
		if o.Type != order.Limit || o.Limit == nil || *o.Limit <= 0 {
			return order.Result{}, fmt.Errorf(
				"executor: a bracket order requires a positive limit price")
		}
		res, err := e.client.PlaceBracketOrder(ctx, broker.BracketOrderRequest{
			OrderRequest: base,
			LmtPrice:     *o.Limit,
			TPPrice:      *o.TakeProfit,
			SLPrice:      *o.StopLoss,
			ParentTIF:    o.TimeInForce(),
		}, key)
		if err != nil {
			return order.Result{}, err
		}
		if res == nil {
			return order.Result{}, fmt.Errorf("executor: nil broker response")
		}
		return order.Result{ID: strconv.Itoa(res.ParentOrderId)}, nil

	case o.Type == order.Market:
		res, err := e.client.PlaceMarketOrder(ctx, broker.MarketOrderRequest{OrderRequest: base}, key)
		if err != nil {
			return order.Result{}, err
		}
		if res == nil {
			return order.Result{}, fmt.Errorf("executor: nil broker response")
		}
		return order.Result{ID: strconv.Itoa(res.OrderId)}, nil

	case o.Type == order.Limit:
		if o.Limit == nil || *o.Limit <= 0 {
			return order.Result{}, fmt.Errorf("executor: limit order requires a positive price")
		}
		res, err := e.client.PlaceLimitOrder(ctx, broker.LimitOrderRequest{
			OrderRequest: base,
			LmtPrice:     *o.Limit,
			TIF:          o.TimeInForce(),
		}, key)
		if err != nil {
			return order.Result{}, err
		}
		if res == nil {
			return order.Result{}, fmt.Errorf("executor: nil broker response")
		}
		return order.Result{ID: strconv.Itoa(res.OrderId)}, nil

	default:
		return order.Result{}, fmt.Errorf("executor: unsupported order type")
	}
}

// Positions returns the broker's positions.
func (e *Broker) Positions(ctx context.Context) ([]order.Position, error) {
	if e.client == nil {
		return nil, fmt.Errorf("executor: no broker client")
	}
	bps, err := e.client.GetPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]order.Position, 0, len(bps))
	for _, bp := range bps {
		out = append(out, toFrameworkPosition(bp))
	}
	return out, nil
}

// OpenOrders returns the orders resting at the broker.
func (e *Broker) OpenOrders(ctx context.Context) ([]order.Order, error) {
	if e.client == nil {
		return nil, fmt.Errorf("executor: no broker client")
	}
	bos, err := e.client.GetOpenOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]order.Order, 0, len(bos))
	for _, bo := range bos {
		out = append(out, toFrameworkOpenOrder(bo))
	}
	return out, nil
}

// Cancel cancels a resting order.
func (e *Broker) Cancel(ctx context.Context, orderID string) error {
	if e.client == nil {
		return fmt.Errorf("executor: no broker client")
	}
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("executor: invalid order id %q", orderID)
	}
	return e.client.CancelOrder(ctx, id)
}

// Update changes the limit price of a resting order.
func (e *Broker) Update(ctx context.Context, orderID string, limit float64) error {
	if e.client == nil {
		return fmt.Errorf("executor: no broker client")
	}
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("executor: invalid order id %q", orderID)
	}
	if limit <= 0 {
		return fmt.Errorf("executor: limit price must be positive")
	}
	_, err = e.client.UpdateOrder(ctx, id, broker.UpdateOrderRequest{LmtPrice: &limit})
	return err
}

// toFrameworkPosition maps a broker position to the framework's.
func toFrameworkPosition(p broker.PositionResponse) order.Position {
	return order.Position{Symbol: p.Symbol, Quantity: p.Quantity, AvgPrice: p.AverageCost}
}

// toFrameworkOpenOrder maps a resting broker order to the framework's.
func toFrameworkOpenOrder(o broker.OpenOrderResponse) order.Order {
	ord := order.Order{
		ID:       strconv.Itoa(o.OrderId),
		Symbol:   o.Symbol,
		Quantity: o.TotalQuantity,
		Type:     order.Market,
	}
	if o.Action == "BUY" {
		ord.Side = order.Buy
	} else if o.Action == "SELL" {
		ord.Side = order.Sell
	}
	if o.LmtPrice != nil {
		limit := *o.LmtPrice
		ord.Type = order.Limit
		ord.Limit = &limit
	}
	return ord
}
