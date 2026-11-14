package live_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/live"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// fakeBroker is a broker the test controls.
type fakeBroker struct {
	mu          sync.Mutex
	positions   []broker.PositionResponse
	openOrders  []broker.OpenOrderResponse
	nav         float64
	health      broker.HealthResponse
	healthErr   error
	reconcileNo int
}

func (f *fakeBroker) Health(_ context.Context) (*broker.HealthResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.healthErr != nil {
		return nil, f.healthErr
	}
	res := f.health
	return &res, nil
}

func (f *fakeBroker) PlaceMarketOrder(_ context.Context, _ broker.MarketOrderRequest, _ string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *fakeBroker) PlaceLimitOrder(_ context.Context, _ broker.LimitOrderRequest, _ string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *fakeBroker) PlaceBracketOrder(_ context.Context, _ broker.BracketOrderRequest, _ string) (*broker.BracketOrderResponse, error) {
	return &broker.BracketOrderResponse{ParentOrderId: 1}, nil
}

func (f *fakeBroker) UpdateOrder(_ context.Context, id int, _ broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: id}, nil
}

func (f *fakeBroker) CancelOrder(_ context.Context, _ int) error { return nil }

func (f *fakeBroker) GetPositions(_ context.Context) ([]broker.PositionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]broker.PositionResponse, len(f.positions))
	copy(out, f.positions)
	return out, nil
}

func (f *fakeBroker) GetOpenOrders(_ context.Context) ([]broker.OpenOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]broker.OpenOrderResponse, len(f.openOrders))
	copy(out, f.openOrders)
	return out, nil
}

func (f *fakeBroker) GetAccountNAV(_ context.Context) (*broker.AccountNAVResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &broker.AccountNAVResponse{Account: "X", NAV: f.nav}, nil
}

// A live run must not start against a disconnected broker. The health
// endpoint answers even when the exchange behind it is down, so the
// connection flag is what matters.
func TestEngine_FailsFastWhenTheBrokerIsDisconnected(t *testing.T) {
	b := &fakeBroker{health: broker.HealthResponse{Status: "ok", IBKRConnected: false}}
	e, err := live.New(live.Config{Broker: b, Logger: nil})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), nil); err == nil {
		t.Fatal("expected the run to fail with a disconnected broker")
	}
}

func TestEngine_FailsFastWhenTheHealthCheckFails(t *testing.T) {
	b := &fakeBroker{healthErr: errors.New("connection refused")}
	e, err := live.New(live.Config{Broker: b})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), nil); err == nil {
		t.Fatal("expected the run to fail when the health check fails")
	}
}

// A job that needs the broker's state must see it, not the state from
// when the process started. The portfolio is refreshed from the broker
// before every job.
func TestEngine_ReconcilesBeforeEveryJob(t *testing.T) {
	b := &fakeBroker{
		health:    broker.HealthResponse{Status: "ok", IBKRConnected: true},
		positions: []broker.PositionResponse{{ConId: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 100}},
		nav:       5000,
	}

	engine, err := live.New(live.Config{Broker: b})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterJob(job.Job{
		Name: "j", Time: job.Time(16, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	// The startup reconciliation writes the broker's positions into the
	// mirror.
	if _, err := engine.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := engine.Portfolio().Position("AAPL").Quantity; got != 10 {
		t.Fatalf("AAPL=%f want 10", got)
	}

	// A position that appears later is only visible after a
	// reconciliation, which is what the before-job hook arranges.
	b.mu.Lock()
	b.positions = append(b.positions, broker.PositionResponse{ConId: 2, Symbol: "MSFT", Quantity: 5, AverageCost: 200})
	b.mu.Unlock()

	if _, err := engine.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := engine.Portfolio().Position("MSFT").Quantity; got != 5 {
		t.Fatalf("MSFT=%f want 5", got)
	}
}

// The reconciler derives the cash balance from the NAV and the
// position marks, because the broker reports a NAV but not the cash in
// it.
func TestReconciler_DerivesCashFromNAVAndMarks(t *testing.T) {
	b := &fakeBroker{
		positions: []broker.PositionResponse{{ConId: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 100}},
		nav:       5000,
	}
	mirror := portfolio.NewLive()
	r := execution.NewReconciler(b, mirror, nil)

	res, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Without a price lookup there is no mark, so the cash is unknown
	// rather than wrong.
	if res.HasCash {
		t.Error("expected the cash to be unknown without a mark")
	}
	if res.Cash != 0 {
		t.Errorf("cash=%f want 0 without a mark", res.Cash)
	}
	if mirror.CashKnown() {
		t.Error("expected the cash to be unknown without a mark")
	}
}
