package execution_test

import (
	"context"
	"sync"
	"testing"

	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/order"
)

// recordingBroker records the requests it receives.
type recordingBroker struct {
	mu      sync.Mutex
	markets []execution.MarketOrderRequest
	limits  []execution.LimitOrderRequest
	brackets []execution.BracketOrderRequest
	keys    []string
}

func (b *recordingBroker) Health(_ context.Context) (*execution.HealthResponse, error) {
	return &execution.HealthResponse{Status: "ok", IBKRConnected: true}, nil
}

func (b *recordingBroker) PlaceMarketOrder(_ context.Context, req execution.MarketOrderRequest, key string) (*execution.OrderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.markets = append(b.markets, req)
	b.keys = append(b.keys, key)
	return &execution.OrderResponse{OrderId: len(b.markets)}, nil
}

func (b *recordingBroker) PlaceLimitOrder(_ context.Context, req execution.LimitOrderRequest, key string) (*execution.OrderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.limits = append(b.limits, req)
	b.keys = append(b.keys, key)
	return &execution.OrderResponse{OrderId: 100 + len(b.limits)}, nil
}

func (b *recordingBroker) PlaceBracketOrder(_ context.Context, req execution.BracketOrderRequest, key string) (*execution.BracketOrderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.brackets = append(b.brackets, req)
	b.keys = append(b.keys, key)
	return &execution.BracketOrderResponse{ParentOrderId: 200 + len(b.brackets)}, nil
}

func (b *recordingBroker) UpdateOrder(_ context.Context, id int, _ execution.UpdateOrderRequest) (*execution.OrderResponse, error) {
	return &execution.OrderResponse{OrderId: id}, nil
}

func (b *recordingBroker) CancelOrder(_ context.Context, id int) error { return nil }

func (b *recordingBroker) GetPositions(_ context.Context) ([]execution.PositionResponse, error) {
	return nil, nil
}

func (b *recordingBroker) GetOpenOrders(_ context.Context) ([]execution.OpenOrderResponse, error) {
	return nil, nil
}

func (b *recordingBroker) GetAccountNAV(_ context.Context) (*execution.AccountNAVResponse, error) {
	return &execution.AccountNAVResponse{NAV: 1000}, nil
}

// The executor is the same in both modes, so a strategy that returns
// an order produces the same broker request whether the run is a
// backtest or a live session. This is the property that makes the
// framework's central claim true: the same handler, the same orders.
//
// The mock broker and the live client are both broker.Client, so the
// two paths are exercised through the same executor.
func TestExecutor_IdenticalOrdersInBothModes(t *testing.T) {
	orders := []order.Order{
		{
			Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
		},
		{
			Symbol: "MSFT", Side: order.Sell, Quantity: 5, Type: order.Limit,
			Limit: ptr(99.0),
		},
		{
			Symbol: "NVDA", Side: order.Buy, Quantity: 3, Type: order.Limit,
			Limit: ptr(186.0), TakeProfit: ptr(190.0), StopLoss: ptr(180.0),
			OutsideRTH: true,
		},
	}

	// The backtest path: the mock broker.
	mockBroker := &recordingBroker{}
	backtest := execution.NewBroker(mockBroker)

	// The live path: the same executor over a different broker.
	liveBroker := &recordingBroker{}
	live := execution.NewBroker(liveBroker)

	for _, o := range orders {
		if _, err := backtest.Submit(context.Background(), o); err != nil {
			t.Fatal(err)
		}
		if _, err := live.Submit(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}

	if len(mockBroker.markets) != len(liveBroker.markets) {
		t.Fatalf("market orders: backtest=%d live=%d", len(mockBroker.markets), len(liveBroker.markets))
	}
	for i := range mockBroker.markets {
		a, b := mockBroker.markets[i], liveBroker.markets[i]
		if a.Contract.Symbol != b.Contract.Symbol ||
			a.Action != b.Action ||
			a.TotalQuantity != b.TotalQuantity {
			t.Errorf("market order %d differs: %+v vs %+v", i, a, b)
		}
	}

	if len(mockBroker.limits) != len(liveBroker.limits) {
		t.Fatalf("limit orders: backtest=%d live=%d", len(mockBroker.limits), len(liveBroker.limits))
	}
	for i := range mockBroker.limits {
		a, b := mockBroker.limits[i], liveBroker.limits[i]
		if a.LmtPrice != b.LmtPrice || a.TIF != b.TIF {
			t.Errorf("limit order %d differs: %+v vs %+v", i, a, b)
		}
		if a.TIF != "GTC" {
			t.Errorf("limit order %d TIF=%q want GTC", i, a.TIF)
		}
	}

	if len(mockBroker.brackets) != len(liveBroker.brackets) {
		t.Fatalf("bracket orders: backtest=%d live=%d", len(mockBroker.brackets), len(liveBroker.brackets))
	}
	for i := range mockBroker.brackets {
		a, b := mockBroker.brackets[i], liveBroker.brackets[i]
		if a.LmtPrice != b.LmtPrice || a.TPPrice != b.TPPrice || a.SLPrice != b.SLPrice {
			t.Errorf("bracket order %d differs: %+v vs %+v", i, a, b)
		}
		if a.ParentTIF != "GTC" {
			t.Errorf("bracket order %d ParentTIF=%q want GTC", i, a.ParentTIF)
		}
		if !a.OutsideRth {
			t.Errorf("bracket order %d lost its outside-RTH flag", i)
		}
	}
}

func ptr(f float64) *float64 { return &f }
