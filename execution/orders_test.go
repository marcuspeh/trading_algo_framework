package execution_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/order"
)

// The broker's wire format is the ibkr-proxy SDK's, so the framework
// cannot drift from the service it talks to. This test pins the JSON
// the framework produces so a change to the SDK shows up here.
func TestBrokerRequest_WireFormat(t *testing.T) {
	req := execution.BracketOrderRequest{
		OrderRequest: execution.OrderRequest{
			Contract:      execution.Contract{Symbol: "AAPL"},
			Action:        "BUY",
			TotalQuantity: 10,
			OutsideRth:    true,
		},
		LmtPrice:  186,
		TPPrice:   190,
		SLPrice:   180,
		ParentTIF: "GTC",
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"symbol":        "AAPL",
		"action":        "BUY",
		"totalQuantity": float64(10),
		"outsideRth":    true,
		"lmtPrice":      float64(186),
		"tpPrice":       float64(190),
		"slPrice":       float64(180),
		"parentTif":     "GTC",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s=%v want %v", k, got[k], v)
		}
	}
}

// The live executor must send the whole number of shares the broker
// expects, and refuse a fractional one rather than truncating it.
func TestLiveExecutor_RejectsFractionalQuantity(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)
	_, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10.5, Type: order.Market,
	})
	if err == nil {
		t.Fatal("expected an error for a fractional quantity")
	}
	if len(b.orders) != 0 {
		t.Fatalf("the broker was called with %d orders", len(b.orders))
	}
}

// A limit order with protective legs is sent as a bracket, with the
// order's time in force carried on the parent.
func TestLiveExecutor_BracketOrder(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	limit, tp, sl := 186.0, 190.0, 180.0
	res, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit,
		Limit: &limit, TakeProfit: &tp, StopLoss: &sl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.bracketOrders) != 1 {
		t.Fatalf("expected one bracket order, got %d", len(b.bracketOrders))
	}
	got := b.bracketOrders[0]
	if got.ParentTIF != "GTC" {
		t.Errorf("ParentTIF=%q want GTC", got.ParentTIF)
	}
	if got.LmtPrice != limit || got.TPPrice != tp || got.SLPrice != sl {
		t.Errorf("prices=%+v", got)
	}
	if got.Contract.Symbol != "AAPL" {
		t.Errorf("symbol=%q", got.Contract.Symbol)
	}
	if res.ID != "201" {
		t.Errorf("id=%q want the parent order id", res.ID)
	}
}

// An order with protective legs but no limit price cannot be a
// bracket, and must be rejected rather than silently degraded.
func TestLiveExecutor_BracketWithoutLimitIsRejected(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	tp, sl := 190.0, 180.0
	_, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
		TakeProfit: &tp, StopLoss: &sl,
	})
	if err == nil {
		t.Fatal("expected an error for a bracket without a limit price")
	}
	if len(b.orders) != 0 || len(b.bracketOrders) != 0 {
		t.Fatal("the order was sent to the broker anyway")
	}
}

// A limit order defaults to GTC so that it rests across sessions, the
// same as the backtest.
func TestLiveExecutor_LimitOrderDefaultsToGTC(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	limit := 100.0
	if _, err := exec.Submit(context.Background(), order.Order{
		Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit,
	}); err != nil {
		t.Fatal(err)
	}
	if len(b.limitOrders) != 1 {
		t.Fatalf("expected one limit order, got %d", len(b.limitOrders))
	}
	if b.limitOrders[0].TIF != "GTC" {
		t.Errorf("TIF=%q want GTC", b.limitOrders[0].TIF)
	}
}

// Every submission carries an idempotency key, and two submissions of
// the same strategy order do not share one: the broker's idempotency
// cache is shared across order types, so a reused key would return the
// wrong response.
func TestLiveExecutor_IdempotencyKeyIsFreshPerSubmission(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	o := order.Order{
		ID: "signal-42", Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	}
	for i := 0; i < 2; i++ {
		if _, err := exec.Submit(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(b.keys))
	}
	if b.keys[0] == b.keys[1] {
		t.Error("the two submissions shared an idempotency key")
	}
	if b.keys[0] == "" || b.keys[1] == "" {
		t.Error("an idempotency key was empty")
	}
}

// The strategy's own id travels as the broker's order reference, so a
// human can reconcile the broker's orders with the strategy's.
func TestLiveExecutor_OrderRefCarriesTheStrategyID(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	if _, err := exec.Submit(context.Background(), order.Order{
		ID: "signal-42", Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market,
	}); err != nil {
		t.Fatal(err)
	}
	if b.orders[0].OrderRef != "signal-42" {
		t.Errorf("OrderRef=%q want signal-42", b.orders[0].OrderRef)
	}
}

// A resting order's limit price can be changed, and an unknown order
// id is an error rather than a silent no-op.
func TestLiveExecutor_UpdateOrder(t *testing.T) {
	b := &fakeBroker{}
	exec := execution.NewLive(b)

	if err := exec.Update(context.Background(), "42", 99.5); err != nil {
		t.Fatal(err)
	}
	if len(b.updates) != 1 {
		t.Fatalf("expected one update, got %d", len(b.updates))
	}
	if b.updates[0].LmtPrice == nil || *b.updates[0].LmtPrice != 99.5 {
		t.Errorf("limit=%v want 99.5", b.updates[0].LmtPrice)
	}

	if err := exec.Update(context.Background(), "not-a-number", 99.5); err == nil {
		t.Error("expected an error for an invalid order id")
	}
	if err := exec.Update(context.Background(), "42", 0); err == nil {
		t.Error("expected an error for a non-positive price")
	}
}
