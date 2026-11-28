package execution_test

import (
	"context"
	"testing"

	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/order"
)

// The executor remembers an order under the broker's permanent id, so
// the fill the broker reports later can be named.
func TestBroker_RecordsTheOrderUnderItsPermanentId(t *testing.T) {
	reg := fills.NewRegistry()
	exec := execution.NewBroker(&fakeBroker{}).WithFills(reg)

	if _, err := exec.Submit(context.Background(), order.Order{
		Symbol:   "AAPL",
		Side:     order.Buy,
		Quantity: 10,
		Type:     order.Market,
	}); err != nil {
		t.Fatal(err)
	}

	o, match := reg.Lookup(5001, "")
	if match != fills.PermIDMatch {
		t.Fatalf("match=%v want a permanent id match", match)
	}
	if o.Symbol != "AAPL" || o.Side != order.Buy {
		t.Errorf("order = %+v want the AAPL buy that was submitted", o)
	}
}

// A bracket's parent is recorded under its permanent id, so the fill
// of the entry can be named.
func TestBroker_RecordsABracketsParent(t *testing.T) {
	reg := fills.NewRegistry()
	exec := execution.NewBroker(&fakeBroker{}).WithFills(reg)

	tp, sl := 120.0, 90.0
	if _, err := exec.Submit(context.Background(), order.Order{
		Symbol:     "AAPL",
		Side:       order.Buy,
		Quantity:   10,
		Type:       order.Limit,
		Limit:      &tp,
		TakeProfit: &tp,
		StopLoss:   &sl,
	}); err != nil {
		t.Fatal(err)
	}

	o, match := reg.Lookup(7001, "")
	if match != fills.PermIDMatch {
		t.Fatalf("match=%v want a permanent id match", match)
	}
	if o.Symbol != "AAPL" {
		t.Errorf("order = %+v want the AAPL bracket that was submitted", o)
	}
}
