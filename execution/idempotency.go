package execution

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/marcuspeh/trading-framework/order"
)

// idempotencyKey returns the key sent to the broker with an order.
//
// A strategy that gives its orders ids gets real idempotency: the same
// order submitted twice carries the same key, so the broker returns
// the first response instead of placing it twice. An order with no id
// gets a fresh key each time, because nothing identifies it.
//
// The key is derived rather than being the strategy's id verbatim,
// because the broker's idempotency cache is shared across the market,
// limit and bracket endpoints: the same id used for two different
// order types would otherwise return the first response to the second
// call.
//
// The key should be logged. When a request times out the broker may or
// may not have accepted the order, and the key is what lets a human
// reconcile the two.
func idempotencyKey(o order.Order) string {
	if o.ID == "" {
		// Nothing identifies the order, so nothing can make a repeat
		// of it recognisable as the same submission.
		return randomKey()
	}
	// A strategy that gives its orders ids gets real idempotency: the
	// same order submitted twice carries the same key, and the broker
	// returns the first response rather than placing it twice.
	//
	// The order type is part of the key because the broker's
	// idempotency cache is shared across the market, limit and bracket
	// endpoints: the same id used for two different order types would
	// otherwise return the wrong cached response.
	sum := sha256.Sum256([]byte(o.ID + "|" + orderTypeName(o)))
	return hex.EncodeToString(sum[:16])
}

// orderTypeName renders an order type for the idempotency key.
func orderTypeName(o order.Order) string {
	switch {
	case o.IsBracket():
		return "bracket"
	case o.Type == order.Market:
		return "market"
	default:
		return "limit"
	}
}

// randomKey returns a fresh 128-bit hex key.
func randomKey() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand does not fail on any platform the framework
		// targets; a failure here would mean the process cannot
		// generate entropy at all.
		return "noentropy"
	}
	return hex.EncodeToString(buf[:])
}
