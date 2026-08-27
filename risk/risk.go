// Package risk provides the default RiskManager and shared rejection
// reasons. The framework returns rejected orders rather than failing
// the Job; each rejection is logged.
package risk

import (
	"fmt"
	"strings"

	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// Manager validates orders returned by Jobs before they reach the
// Executor. Strategies may swap it for custom logic.
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

// Basic is the default Manager. It rejects clearly malformed orders.
type Basic struct{}

func NewBasic() *Basic { return &Basic{} }

// Validate returns the subset of orders that pass validation.
//
//   - Symbol must be non-empty
//   - Quantity must be > 0
//   - Type must be Market or Limit
//   - Limit orders must carry a positive Limit price
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
		default:
			kept = append(kept, o)
		}
	}

	if len(rejected) == 0 {
		return kept, nil
	}
	return kept, rejected
}
