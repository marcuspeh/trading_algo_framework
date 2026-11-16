package risk_test

import (
	"errors"
	"testing"

	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/risk"
)

func TestBasic_KeepsValidOrders(t *testing.T) {
	m := risk.NewBasic()
	orders := []order.Order{
		{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market},
	}
	kept, err := m.Validate(nil, orders)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept=%d want 1", len(kept))
	}
}

func TestBasic_RejectsMalformedOrders(t *testing.T) {
	m := risk.NewBasic()
	limit := 0.0
	orders := []order.Order{
		{Symbol: "", Quantity: 10, Type: order.Market},
		{Symbol: "AAPL", Quantity: 0, Type: order.Market},
		{Symbol: "AAPL", Quantity: 10, Type: order.OrderType(99)},
		{Symbol: "AAPL", Quantity: 10, Type: order.Limit, Limit: &limit},
	}
	kept, err := m.Validate(nil, orders)
	if len(kept) != 0 {
		t.Fatalf("kept=%d want 0", len(kept))
	}
	reasons := risk.Rejected(err)
	if len(reasons) != 4 {
		t.Fatalf("reasons=%d want 4", len(reasons))
	}
}

// TestBasic_PartialRejectionKeepsGoodOrders verifies that one bad order
// does not discard the good ones.
func TestBasic_PartialRejectionKeepsGoodOrders(t *testing.T) {
	m := risk.NewBasic()
	orders := []order.Order{
		{Symbol: "AAPL", Quantity: 10, Type: order.Market},
		{Symbol: "", Quantity: 10, Type: order.Market},
		{Symbol: "MSFT", Quantity: 5, Type: order.Market},
	}
	kept, err := m.Validate(nil, orders)
	if len(kept) != 2 {
		t.Fatalf("kept=%d want 2", len(kept))
	}
	reasons := risk.Rejected(err)
	if len(reasons) != 1 {
		t.Fatalf("reasons=%d want 1", len(reasons))
	}
}

func TestRejected_NonReasonsErrorReturnsNil(t *testing.T) {
	if got := risk.Rejected(errors.New("boom")); got != nil {
		t.Fatalf("reasons=%v want nil", got)
	}
	if got := risk.Rejected(nil); got != nil {
		t.Fatalf("reasons=%v want nil", got)
	}
}

func TestReasons_ErrorMessage(t *testing.T) {
	rs := risk.Reasons{{Order: order.Order{Symbol: "AAPL", Side: order.Buy, Quantity: 10}, Reason: "empty symbol"}}
	if got := rs.Error(); got == "" {
		t.Fatal("expected a message")
	}
}

// An order with protective legs but no entry price cannot be a
// bracket, so it is rejected rather than sent as a plain order.
func TestBasic_ProtectiveLegsRequireALimitPrice(t *testing.T) {
	tp, sl := 190.0, 180.0
	limit := 186.0

	cases := []struct {
		name  string
		order order.Order
		ok    bool
	}{
		{
			name:  "market order with legs",
			order: order.Order{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Market, TakeProfit: &tp, StopLoss: &sl},
			ok:    false,
		},
		{
			name:  "limit order with legs and a price",
			order: order.Order{Symbol: "AAPL", Side: order.Buy, Quantity: 10, Type: order.Limit, Limit: &limit, TakeProfit: &tp, StopLoss: &sl},
			ok:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kept, err := risk.NewBasic().Validate(nil, []order.Order{tc.order})
			if tc.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(kept) != 1 {
					t.Fatalf("kept=%d want 1", len(kept))
				}
				return
			}
			if len(risk.Rejected(err)) != 1 {
				t.Fatalf("expected one rejection, got %v", err)
			}
		})
	}
}
