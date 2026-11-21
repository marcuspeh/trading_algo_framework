// Package fills receives the fill reports a live broker pushes and logs
// them.
//
// A live broker fills asynchronously: the framework submits an order and
// the broker reports the fill later, on a channel of its own. The
// ibkr-proxy pushes those reports as HTTP POSTs to {WEBHOOK_URL}/fills.
// This package is the end of that webhook: it names the order that
// filled and logs the fill.
//
// A backtest has no such report - the simulated broker fills an order
// itself - so the mock broker logs its own fills and never comes here.
package fills

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/marcuspeh/trading-framework/order"
)

// Fill is the report a broker pushes when an order fills.
//
// The field names are the wire names, so the report decodes without a
// translation layer.
type Fill struct {
	// PermID is the broker's permanent order id. The order
	// acknowledgement carries the same id, and that is how a fill is
	// tied to the order that filled.
	PermID int `json:"permId"`

	// ExecID identifies this execution. An order that fills in pieces
	// produces one exec id per piece.
	ExecID string `json:"execId"`

	// Price is the price the order filled at.
	Price float64 `json:"fillPrice"`

	// Quantity is how much filled, in shares.
	Quantity float64 `json:"fillQty"`

	// OrderRef is the order's own reference - the strategy's id for
	// it. A bracket's exit legs carry the entry's reference, because
	// the broker never reports their permanent ids to the framework.
	OrderRef string `json:"orderRef"`

	// Commission is what the broker charged for the fill.
	Commission float64 `json:"commission"`
}

// Match says how a fill was tied to an order.
type Match int

const (
	// NoMatch means the fill is for an order the framework did not
	// place: a manual trade, or another client's.
	NoMatch Match = iota

	// PermIDMatch means the broker's permanent order id matched.
	PermIDMatch

	// RefMatch means the order's reference matched. The only orders
	// that match this way are a bracket's exit legs.
	RefMatch
)

// Registry remembers the orders the framework placed, so a fill can be
// named.
//
// It is safe for concurrent use: the executor writes to it while the
// broker's webhook reads from it.
type Registry struct {
	mu     sync.Mutex
	permID map[int]order.Order
	ref    map[string]order.Order
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		permID: map[int]order.Order{},
		ref:    map[string]order.Order{},
	}
}

// Record remembers an order under the broker's permanent id.
//
// An order with no permanent id - a broker that does not report one -
// is not recorded: nothing would find it again.
func (r *Registry) Record(permID int, o order.Order) {
	if r == nil || permID == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permID[permID] = o
	if o.ID != "" {
		r.ref[o.ID] = o
	}
}

// Lookup finds the order a fill belongs to.
//
// The permanent id is tried first: it names the order exactly. The
// reference is tried second, because a bracket's exit legs are
// reported by the reference their entry carried.
func (r *Registry) Lookup(permID int, ref string) (order.Order, Match) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if o, ok := r.permID[permID]; ok {
		return o, PermIDMatch
	}
	if ref != "" {
		if o, ok := r.ref[ref]; ok {
			return o, RefMatch
		}
	}
	return order.Order{}, NoMatch
}

// Logger records the fills the receiver logs.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// Handler returns the handler that receives the fill reports.
//
// The path is not fixed: the broker's webhook URL decides it, and the
// proxy's default puts the fill webhook at /webhook/fills while a bare
// host puts it at /fills. The body says what it is, so any path works.
func (r *Registry) Handler(log Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		var f Fill
		if err := json.NewDecoder(req.Body).Decode(&f); err != nil {
			log.Warn("fill report: malformed payload", "err", err.Error())
			http.Error(w, "malformed fill report", http.StatusBadRequest)
			return
		}
		r.log(log, f)
		w.WriteHeader(http.StatusOK)
	})
}

// log writes a fill to the log.
func (r *Registry) log(log Logger, f Fill) {
	fields := []any{
		"id", f.PermID,
		"exec_id", f.ExecID,
		"qty", f.Quantity,
		"price", f.Price,
	}
	if f.Commission != 0 {
		fields = append(fields, "commission", f.Commission)
	}
	if f.OrderRef != "" {
		fields = append(fields, "order_ref", f.OrderRef)
	}

	o, match := r.Lookup(f.PermID, f.OrderRef)
	switch match {
	case PermIDMatch:
		fields = append(fields, "symbol", o.Symbol, "side", o.Side.String())
		log.Info("order filled", fields...)

	case RefMatch:
		// The fill named the order by its reference, which means it
		// is one of a bracket's exit legs: the entry's own fill
		// carries the permanent id the framework knows. An exit leg
		// closes the trade, so it trades the opposite side.
		fields = append(fields, "symbol", o.Symbol, "side", opposite(o.Side).String())
		log.Info("order filled", fields...)

	default:
		// A fill for an order the framework did not place is worth
		// knowing about - a manual trade, or another client's - but
		// the framework cannot say what it was.
		log.Warn("fill for an order the framework did not place", fields...)
	}
}

// opposite returns the side that trades against the given one.
func opposite(s order.Side) order.Side {
	if s == order.Buy {
		return order.Sell
	}
	return order.Buy
}
