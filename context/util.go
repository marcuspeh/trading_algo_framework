package context

import (
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

func cloneBars(in []order.Bar) []order.Bar {
	out := make([]order.Bar, len(in))
	copy(out, in)
	return out
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

type missingDepError struct {
	name string
}

func (e missingDepError) Error() string {
	return "context: missing dependency: " + e.name
}

func errMissing(name string) error {
	return missingDepError{name: name}
}

type invalidArgError struct {
	msg string
}

func (e invalidArgError) Error() string {
	return "context: " + e.msg
}

func errInvalid(msg string) error {
	return invalidArgError{msg: msg}
}

// sessionKey renders a session's date as a cache key.
func sessionKey(session time.Time) string {
	return session.Format("2006-01-02")
}
