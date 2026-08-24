package risk_test

import (
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/risk"
)

func nopCtx() tf.Context { return ctxStub{} }

type ctxStub struct{}

func (ctxStub) Date() time.Time                       { return time.Time{} }
func (ctxStub) Now() time.Time                        { return time.Time{} }
func (ctxStub) Bars(string, int) ([]tf.Bar, error)    { return nil, nil }
func (ctxStub) Constituents(string) ([]string, error) { return nil, nil }
func (ctxStub) IsTradingDay(time.Time) bool           { return true }
func (ctxStub) Position(string) tf.Position           { return tf.Position{} }
func (ctxStub) Positions() []tf.Position              { return nil }
func (ctxStub) Cash() float64                         { return 0 }
func (ctxStub) PortfolioValue() float64               { return 0 }
func (ctxStub) Mode() tf.Mode                         { return tf.Backtest }
func (ctxStub) Config() tf.Config                     { return nil }
func (ctxStub) Logger() tf.Logger                     { return nil }

func TestBasicRisk_ApprovesValidOrders(t *testing.T) {
	r := risk.NewBasic()
	ord := tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 10, Type: tf.Market}
	kept, err := r.Validate(nopCtx(), []tf.Order{ord})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("expected kept=1, got %d", len(kept))
	}
}

func TestBasicRisk_RejectsBadOrders(t *testing.T) {
	r := risk.NewBasic()

	limit := 50.0
	cases := []tf.Order{
		{Symbol: "", Side: tf.Buy, Quantity: 1, Type: tf.Market},
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 0, Type: tf.Market},
		{Symbol: "AAPL", Side: tf.Buy, Quantity: -1, Type: tf.Market},
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.OrderType(99)},
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Limit},
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Limit, Limit: &limit}, // limit price valid, OK
	}
	kept, err := r.Validate(nopCtx(), cases)
	if err == nil {
		t.Fatal("expected Reasons error")
	}
	if _, ok := err.(risk.Reasons); !ok {
		t.Fatalf("expected Reasons, got %T", err)
	}
	// Last order is valid; the rest should be rejected.
	if len(kept) != 1 {
		t.Fatalf("expected one kept order, got %d", len(kept))
	}
	if kept[0].Limit == nil || *kept[0].Limit != limit {
		t.Fatalf("unexpected kept order: %+v", kept[0])
	}
}
