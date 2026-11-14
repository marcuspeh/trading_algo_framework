package execution

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/marcuspeh/trading-framework/order"
)

// idempotencyKey returns the key sent to the broker with an order.
//
// The key is minted fresh for every submission. It is deliberately not
// the strategy's own order id: the broker's idempotency cache is
// shared across the market, limit and bracket endpoints, so reusing a
// key across two different order types would return the first
// response to the second call.
//
// The key is returned to the caller so it can be logged. When a
// request times out the broker may or may not have accepted the order,
// and the key is what lets a human reconcile the two.
func idempotencyKey(o order.Order) string {
	if o.ID != "" {
		// The strategy's id is only used as a suffix, so two
		// submissions of the same strategy order still produce
		// different keys.
		return randomKey() + "-" + o.ID
	}
	return randomKey()
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
