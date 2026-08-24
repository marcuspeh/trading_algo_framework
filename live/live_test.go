package live_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/portfolio"
)

type fakeBroker struct {
	mu          sync.Mutex
	orders      []live.MarketOrderRequest
	limitOrders []live.LimitOrderRequest
	positions   []live.PositionResponse
	openOrders  []live.OpenOrderResponse
	nav         float64
	failNext    bool
	cancels     []int
}

func (f *fakeBroker) PlaceMarketOrder(_ context.Context, req live.MarketOrderRequest, _ string) (*live.OrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return nil, errors.New("broker down")
	}
	f.orders = append(f.orders, req)
	return &live.OrderResponse{OrderID: len(f.orders)}, nil
}

func (f *fakeBroker) PlaceLimitOrder(_ context.Context, req live.LimitOrderRequest, _ string) (*live.OrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limitOrders = append(f.limitOrders, req)
	return &live.OrderResponse{OrderID: 100 + len(f.limitOrders)}, nil
}

func (f *fakeBroker) CancelOrder(_ context.Context, id int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
	return nil
}

func (f *fakeBroker) GetPositions(_ context.Context) ([]live.PositionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]live.PositionResponse, len(f.positions))
	copy(out, f.positions)
	return out, nil
}

func (f *fakeBroker) GetOpenOrders(_ context.Context) ([]live.OpenOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]live.OpenOrderResponse, len(f.openOrders))
	copy(out, f.openOrders)
	return out, nil
}

func (f *fakeBroker) GetAccountNAV(_ context.Context) (*live.AccountNAVResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &live.AccountNAVResponse{Account: "X", NAV: f.nav}, nil
}

type fakeMarket struct{}

func (fakeMarket) Bars(_ context.Context, _ string, _ int, _ time.Time) ([]tf.Bar, error) {
	return nil, nil
}

type fakeConstituents struct{}

func (fakeConstituents) Constituents(_ context.Context, _ string, _ time.Time) ([]string, error) {
	return nil, nil
}

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error) { return "", nil }
func (fakeConfig) Watch(string, any, any) (tf.Watcher, error) {
	return nil, nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

func TestExecutor_SubmitMarket(t *testing.T) {
	broker := &fakeBroker{}
	exec := live.NewExecutor(broker)
	res, err := exec.Submit(context.Background(), tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 10, Type: tf.Market})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "1" {
		t.Fatalf("expected ID=1, got %s", res.ID)
	}
	if len(broker.orders) != 1 || broker.orders[0].Action != "BUY" {
		t.Fatalf("broker order mismatch: %+v", broker.orders)
	}
}

func TestExecutor_SubmitLimit(t *testing.T) {
	broker := &fakeBroker{}
	exec := live.NewExecutor(broker)
	limit := 99.0
	_, err := exec.Submit(context.Background(), tf.Order{Symbol: "AAPL", Side: tf.Sell, Quantity: 5, Type: tf.Limit, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if len(broker.limitOrders) != 1 || broker.limitOrders[0].LmtPrice != 99 {
		t.Fatalf("limit order: %+v", broker.limitOrders)
	}
}

func TestExecutor_SubmitValidates(t *testing.T) {
	exec := live.NewExecutor(&fakeBroker{})
	if _, err := exec.Submit(context.Background(), tf.Order{}); err == nil {
		t.Fatal("expected empty-symbol error")
	}
	if _, err := exec.Submit(context.Background(), tf.Order{Symbol: "AAPL", Quantity: -1, Type: tf.Market}); err == nil {
		t.Fatal("expected qty error")
	}
	limit := 0.0
	if _, err := exec.Submit(context.Background(), tf.Order{Symbol: "AAPL", Quantity: 1, Type: tf.Limit, Limit: &limit}); err == nil {
		t.Fatal("expected limit price error")
	}
}

func TestExecutor_PositionsAndOpenOrders(t *testing.T) {
	broker := &fakeBroker{
		positions: []live.PositionResponse{
			{ConID: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 100},
		},
		openOrders: []live.OpenOrderResponse{
			{PermID: 2, OrderID: 99, Symbol: "MSFT", Action: "BUY", TotalQuantity: 5, Status: "Submitted"},
		},
	}
	exec := live.NewExecutor(broker)
	pos, err := exec.Positions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pos) != 1 || pos[0].Symbol != "AAPL" || pos[0].Quantity != 10 {
		t.Fatalf("positions: %+v", pos)
	}
	opens, err := exec.OpenOrders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 1 || opens[0].Side != tf.Buy {
		t.Fatalf("open orders: %+v", opens)
	}
}

func TestExecutor_Cancel(t *testing.T) {
	broker := &fakeBroker{}
	exec := live.NewExecutor(broker)
	if err := exec.Cancel(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}
	if len(broker.cancels) != 1 || broker.cancels[0] != 123 {
		t.Fatalf("cancels: %+v", broker.cancels)
	}
	if err := exec.Cancel(context.Background(), "not-a-number"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestReconciler_OverwritesMirrorWithBrokerTruth(t *testing.T) {
	broker := &fakeBroker{
		positions: []live.PositionResponse{
			{ConID: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 100},
			{ConID: 2, Symbol: "MSFT", Quantity: -3, AverageCost: 250},
		},
		openOrders: []live.OpenOrderResponse{},
		nav:        5000,
	}
	mirror := portfolio.NewMirror()
	r := live.NewReconciler(broker, mirror, nopLogger{})
	res, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Positions != 2 || !res.HasNAV || res.NAV != 5000 {
		t.Fatalf("result: %+v", res)
	}
	if got := mirror.Position("AAPL").Quantity; got != 10 {
		t.Fatalf("AAPL: %f", got)
	}
	if got := mirror.Position("MSFT").Quantity; got != -3 {
		t.Fatalf("MSFT: %f", got)
	}
	if mirror.Value() != 5000 {
		t.Fatalf("value: %f", mirror.Value())
	}
}

func TestReconciler_RequiresBroker(t *testing.T) {
	r := live.NewReconciler(nil, portfolio.NewMirror(), nopLogger{})
	if _, err := r.Reconcile(context.Background()); err == nil {
		t.Fatal("expected error for nil broker")
	}
}
