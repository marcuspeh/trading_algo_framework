package execution

import (
	"context"
	"fmt"
	"strconv"

	"github.com/marcuspeh/trading-framework/order"
)

// Live is the framework Executor for live mode. It translates
// broker-neutral Orders into broker-specific calls. Local portfolio
// state is owned by the Reconciler.
type Live struct {
	broker BrokerClient
}

func NewLive(b BrokerClient) *Live {
	return &Live{broker: b}
}

// Submit places a market or limit order with the broker. The returned
// OrderResult.ID is the broker OrderID. The broker reports fills
// asynchronously; the framework does not wait.
func (e *Live) Submit(ctx context.Context, o order.Order) (order.Result, error) {
	if e.broker == nil {
		return order.Result{}, fmt.Errorf("live executor: no broker client")
	}
	if o.Symbol == "" {
		return order.Result{}, fmt.Errorf("live executor: empty symbol")
	}
	if o.Quantity <= 0 {
		return order.Result{}, fmt.Errorf("live executor: non-positive quantity")
	}

	req := MarketOrderRequest{
		Symbol:        o.Symbol,
		Action:        o.Side.String(),
		TotalQuantity: o.Quantity,
	}

	var (
		res *OrderResponse
		err error
	)

	switch o.Type {
	case order.Market:
		res, err = e.broker.PlaceMarketOrder(ctx, req, o.ID)
	case order.Limit:
		if o.Limit == nil || *o.Limit <= 0 {
			return order.Result{}, fmt.Errorf("live executor: limit order requires a positive price")
		}
		lreq := LimitOrderRequest{MarketOrderRequest: req, LmtPrice: *o.Limit}
		res, err = e.broker.PlaceLimitOrder(ctx, lreq, o.ID)
	default:
		return order.Result{}, fmt.Errorf("live executor: unsupported order type")
	}
	if err != nil {
		return order.Result{}, err
	}
	if res == nil {
		return order.Result{}, fmt.Errorf("live executor: nil broker response")
	}
	return order.Result{
		ID:        strconv.Itoa(res.OrderID),
		Filled:    false,
		FillPrice: 0,
	}, nil
}

func (e *Live) Positions(ctx context.Context) ([]order.Position, error) {
	if e.broker == nil {
		return nil, fmt.Errorf("live executor: no broker client")
	}
	bps, err := e.broker.GetPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]order.Position, 0, len(bps))
	for _, bp := range bps {
		out = append(out, toFrameworkPosition(bp))
	}
	return out, nil
}

func (e *Live) OpenOrders(ctx context.Context) ([]order.Order, error) {
	if e.broker == nil {
		return nil, fmt.Errorf("live executor: no broker client")
	}
	bos, err := e.broker.GetOpenOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]order.Order, 0, len(bos))
	for _, bo := range bos {
		out = append(out, toFrameworkOpenOrder(bo))
	}
	return out, nil
}

func (e *Live) Cancel(ctx context.Context, orderID string) error {
	if e.broker == nil {
		return fmt.Errorf("live executor: no broker client")
	}
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("live executor: invalid order id %q", orderID)
	}
	return e.broker.CancelOrder(ctx, id)
}

func toFrameworkPosition(p PositionResponse) order.Position {
	return order.Position{Symbol: p.Symbol, Quantity: p.Quantity, AvgPrice: p.AverageCost}
}

func toFrameworkOpenOrder(o OpenOrderResponse) order.Order {
	ord := order.Order{
		ID:       strconv.Itoa(o.OrderID),
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
