package execution_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

type fakeBroker struct {
	mu            sync.Mutex
	orders        []execution.MarketOrderRequest
	limitOrders   []execution.LimitOrderRequest
	bracketOrders []execution.BracketOrderRequest
	updates       []execution.UpdateOrderRequest
	positions     []execution.PositionResponse
	openOrders    []execution.OpenOrderResponse
	nav           float64
	failNext      bool
	cancels       []int
	keys          []string
}

func (f *fakeBroker) Health(_ context.Context) (*execution.HealthResponse, error) {
	return &execution.HealthResponse{Status: "ok", IBKRConnected: true}, nil
}

func (f *fakeBroker) PlaceMarketOrder(_ context.Context, req execution.MarketOrderRequest, key string) (*execution.OrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return nil, errors.New("broker down")
	}
	f.orders = append(f.orders, req)
	f.keys = append(f.keys, key)
	return &execution.OrderResponse{OrderId: len(f.orders)}, nil
}

func (f *fakeBroker) PlaceLimitOrder(_ context.Context, req execution.LimitOrderRequest, key string) (*execution.OrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limitOrders = append(f.limitOrders, req)
	f.keys = append(f.keys, key)
	return &execution.OrderResponse{OrderId: 100 + len(f.limitOrders)}, nil
}

func (f *fakeBroker) PlaceBracketOrder(_ context.Context, req execution.BracketOrderRequest, key string) (*execution.BracketOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bracketOrders = append(f.bracketOrders, req)
	f.keys = append(f.keys, key)
	return &execution.BracketOrderResponse{ParentOrderId: 200 + len(f.bracketOrders)}, nil
}

func (f *fakeBroker) UpdateOrder(_ context.Context, id int, req execution.UpdateOrderRequest) (*execution.OrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, req)
	return &execution.OrderResponse{OrderId: id}, nil
}

func (f *fakeBroker) CancelOrder(_ context.Context, id int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
	return nil
}

func (f *fakeBroker) GetPositions(_ context.Context) ([]execution.PositionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]execution.PositionResponse, len(f.positions))
	copy(out, f.positions)
	return out, nil
}

func (f *fakeBroker) GetOpenOrders(_ context.Context) ([]execution.OpenOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]execution.OpenOrderResponse, len(f.openOrders))
	copy(out, f.openOrders)
	return out, nil
}

func (f *fakeBroker) GetAccountNAV(_ context.Context) (*execution.AccountNAVResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &execution.AccountNAVResponse{Account: "X", NAV: f.nav}, nil
}

func TestLiveExecutor_SubmitMarket(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "1" {
		t.Fatalf("ID=%s", res.ID)
	}
	if b.orders[0].Action != "BUY" {
		t.Fatalf("action=%s", b.orders[0].Action)
	}
}

func TestLiveExecutor_SubmitLimit(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)
	limit := 99.0
	_, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Sell, Quantity: 5, Type: order.Limit, Limit: &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.limitOrders[0].LmtPrice != 99 {
		t.Fatalf("price=%f", b.limitOrders[0].LmtPrice)
	}
}

func TestLiveExecutor_Cancel(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)
	if err := exec.Cancel(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if b.cancels[0] != 42 {
		t.Fatalf("cancels=%v", b.cancels)
	}
	if err := exec.Cancel(context.Background(), "not-a-number"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestReconciler_OverwritesMirrorWithBrokerTruth(t *testing.T) {
	b := &fakeBroker{
		positions: []execution.PositionResponse{
			{ConId: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 100},
			{ConId: 2, Symbol: "MSFT", Quantity: -3, AverageCost: 250},
		},
		nav: 5000,
	}
	mirror := portfolio.NewLive()
	r := execution.NewReconciler(b, mirror, nil)
	res, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Positions != 2 || !res.HasNAV || res.NAV != 5000 {
		t.Fatalf("res=%+v", res)
	}
	if got := mirror.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("AAPL=%f", got)
	}
}
