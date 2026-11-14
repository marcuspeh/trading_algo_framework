package execution

import (
	"context"
	"fmt"
	"strconv"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/order"
)

// Live is the framework Executor for live mode. It translates
// broker-neutral Orders into broker-specific calls. Local portfolio
// state is owned by the Reconciler.
type Live struct {
	broker Client
}

func NewLive(b Client) *Live {
	return &Live{broker: b}
}

// Submit places a market, limit or bracket order with the broker. The
// returned Result.ID is the broker's order id. The broker reports
// fills asynchronously; the framework does not wait.
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

	// The broker takes a whole number of shares.
	qty := int(o.Quantity)
	if float64(qty) != o.Quantity {
		return order.Result{}, fmt.Errorf(
			"live executor: quantity %v is not a whole number of shares", o.Quantity)
	}

	base := broker.OrderRequest{
		Contract:      broker.Contract{Symbol: o.Symbol},
		Action:        o.Side.String(),
		TotalQuantity: qty,
		OrderRef:      o.ID,
		OutsideRth:    o.OutsideRTH,
	}

	var (
		res *broker.OrderResponse
		err error
	)

	switch {
	case o.IsBracket():
		if o.Type != order.Limit || o.Limit == nil || *o.Limit <= 0 {
			return order.Result{}, fmt.Errorf(
				"live executor: a bracket order requires a positive limit price")
		}
		breq := broker.BracketOrderRequest{
			OrderRequest: base,
			LmtPrice:     *o.Limit,
			TPPrice:      *o.TakeProfit,
			SLPrice:      *o.StopLoss,
			ParentTIF:    o.TimeInForce(),
		}
		bres, berr := e.broker.PlaceBracketOrder(ctx, breq, idempotencyKey(o))
		if berr != nil {
			return order.Result{}, berr
		}
		if bres == nil {
			return order.Result{}, fmt.Errorf("live executor: nil broker response")
		}
		return order.Result{ID: strconv.Itoa(bres.ParentOrderId)}, nil

	case o.Type == order.Market:
		res, err = e.broker.PlaceMarketOrder(ctx, broker.MarketOrderRequest{OrderRequest: base}, idempotencyKey(o))

	case o.Type == order.Limit:
		if o.Limit == nil || *o.Limit <= 0 {
			return order.Result{}, fmt.Errorf("live executor: limit order requires a positive price")
		}
		lreq := broker.LimitOrderRequest{
			OrderRequest: base,
			LmtPrice:     *o.Limit,
			TIF:          o.TimeInForce(),
		}
		res, err = e.broker.PlaceLimitOrder(ctx, lreq, idempotencyKey(o))

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
		ID:        strconv.Itoa(res.OrderId),
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

// Update changes the limit price of a resting order.
func (e *Live) Update(ctx context.Context, orderID string, limit float64) error {
	if e.broker == nil {
		return fmt.Errorf("live executor: no broker client")
	}
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("live executor: invalid order id %q", orderID)
	}
	if limit <= 0 {
		return fmt.Errorf("live executor: limit price must be positive")
	}
	_, err = e.broker.UpdateOrder(ctx, id, broker.UpdateOrderRequest{LmtPrice: &limit})
	return err
}

func toFrameworkPosition(p broker.PositionResponse) order.Position {
	return order.Position{Symbol: p.Symbol, Quantity: p.Quantity, AvgPrice: p.AverageCost}
}

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
