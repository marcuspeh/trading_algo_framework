// Package risk provides the default RiskManager and shared rejection
// reasons. The framework returns rejected orders rather than failing
// the Job; each rejection is logged.
package risk

import (
	"errors"
	"fmt"
	"strings"

	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// Manager validates orders returned by Jobs before they reach the
// Executor. Strategies may swap it for custom logic.
//
// Validate returns the subset of orders that passed validation. When it
// rejects orders it also returns a non-nil error of type Reasons
// describing each rejection, while the approved orders are still
// returned and should still be executed. A rejection is a normal
// outcome, not a failure of the Job.
type Manager interface {
	Validate(ctx job.Context, orders []order.Order) ([]order.Order, error)
}

type Reason struct {
	Order  order.Order
	Reason string
}

func (r Reason) Error() string {
	return fmt.Sprintf("order %s %s qty=%g: %s",
		r.Order.Symbol, r.Order.Side, r.Order.Quantity, r.Reason)
}

// Reasons lists the orders a Manager rejected, and why. It is the error
// type returned by Validate when some orders are rejected.
type Reasons []Reason

func (rs Reasons) Error() string {
	if len(rs) == 0 {
		return ""
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Error()
	}
	return strings.Join(parts, "; ")
}

// Rejected extracts the rejection reasons from a Validate error. It
// returns nil when err is not a Reasons error, or when no orders were
// rejected.
func Rejected(err error) Reasons {
	if err == nil {
		return nil
	}
	var reasons Reasons
	if errors.As(err, &reasons) {
		return reasons
	}
	return nil
}

// Basic is the default Manager. It rejects clearly malformed orders.
type Basic struct{}

func NewBasic() *Basic { return &Basic{} }

// Validate returns the subset of orders that pass validation.
//
//   - Symbol must be non-empty
//   - Quantity must be > 0
//   - Type must be Market or Limit
//   - Limit orders must carry a positive Limit price
//   - An order with protective legs must be a limit order: a bracket
//     has no entry price without one
func (b *Basic) Validate(_ job.Context, orders []order.Order) ([]order.Order, error) {
	kept := make([]order.Order, 0, len(orders))
	var rejected Reasons

	for _, o := range orders {
		switch {
		case strings.TrimSpace(o.Symbol) == "":
			rejected = append(rejected, Reason{Order: o, Reason: "empty symbol"})
		case o.Quantity <= 0:
			rejected = append(rejected, Reason{Order: o, Reason: "quantity must be > 0"})
		case o.Type != order.Market && o.Type != order.Limit:
			rejected = append(rejected, Reason{Order: o, Reason: "unsupported order type"})
		case o.Type == order.Limit && (o.Limit == nil || *o.Limit <= 0):
			rejected = append(rejected, Reason{Order: o, Reason: "limit orders require a positive Limit price"})
		case (o.TakeProfit != nil || o.StopLoss != nil) && (o.Type != order.Limit || o.Limit == nil):
			rejected = append(rejected, Reason{Order: o, Reason: "protective legs require a limit order with a price"})
		default:
			kept = append(kept, o)
		}
	}

	if len(rejected) == 0 {
		return kept, nil
	}
	return kept, rejected
}
