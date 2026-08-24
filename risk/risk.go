// Package risk provides the default RiskManager and shared rejection
// reasons. The framework returns rejected orders rather than failing
// the Job; each rejection is logged.
package risk

import (
	"fmt"
	"strings"

	tf "github.com/marcuspeh/trading-framework"
)

type Reason struct {
	Order  tf.Order
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

// Basic is the default RiskManager. It rejects clearly malformed orders
// and lets everything else through.
type Basic struct{}

func NewBasic() *Basic { return &Basic{} }

// Validate returns the subset of orders that pass validation.
//
//   - Symbol must be non-empty
//   - Quantity must be > 0
//   - Type must be Market or Limit
//   - Limit orders must carry a positive Limit price
//
// The Basic manager does not enforce position-size or buying-power
// limits; those are intentionally left for strategies or future risk
// managers to add.
func (b *Basic) Validate(_ tf.Context, orders []tf.Order) ([]tf.Order, error) {
	kept := make([]tf.Order, 0, len(orders))
	var rejected Reasons

	for _, o := range orders {
		switch {
		case strings.TrimSpace(o.Symbol) == "":
			rejected = append(rejected, Reason{Order: o, Reason: "empty symbol"})
		case o.Quantity <= 0:
			rejected = append(rejected, Reason{Order: o, Reason: "quantity must be > 0"})
		case o.Type != tf.Market && o.Type != tf.Limit:
			rejected = append(rejected, Reason{Order: o, Reason: "unsupported order type"})
		case o.Type == tf.Limit && (o.Limit == nil || *o.Limit <= 0):
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
