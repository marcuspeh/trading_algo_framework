// Package calendar answers questions about NYSE trading sessions.
//
// Every date the framework reasons about is anchored on the exchange
// calendar: which days the exchange trades, which days it closes
// early, and when each session opens and closes.
//
// The package is a leaf - it depends on nothing in the framework, so
// clock, clients, scheduler and portfolio can all use it without
// creating an import cycle.
//
// The answers come from a Provider. The framework's own copy of the
// NYSE rules is gone: the exchange publishes its schedule and the
// market-data-service serves it, so Service is what the framework
// trades on. A provider that cannot answer says so - the framework
// treats that as fatal rather than guessing.
package calendar

import (
	"errors"
	"fmt"
	"time"
)

// Provider answers NYSE session questions.
//
// A provider may consult a remote service, so every method returns an
// error. The framework treats a provider failure as fatal: trading on a
// schedule it could not confirm is worse than not trading at all.
//
// The session times are only defined for a day the exchange traded on.
// Asking for the open or the close of a day it was closed is an error,
// not a zero time.
type Provider interface {
	// TradingDay reports whether the NYSE held a session on the date.
	TradingDay(date time.Time) (bool, error)

	// EarlyClose reports whether the session on the date was shortened,
	// closing at 13:00 ET instead of 16:00 ET.
	EarlyClose(date time.Time) (bool, error)

	// SessionOpen returns the open of the session on the date.
	SessionOpen(date time.Time) (time.Time, error)

	// SessionClose returns the close of the session on the date.
	SessionClose(date time.Time) (time.Time, error)
}

// ErrNoSession reports that the exchange held no session on a date.
//
// It is distinct from a failure: the provider answered, and the answer
// was that there was no session. A provider that cannot answer at all
// returns some other error, which the framework treats as fatal.
//
// The distinction matters because a strategy may trade on days the
// exchange was closed - a different exchange's calendar, or its own
// schedule. Those days need a session to resolve a slot against, and
// the standard session is what stands in for them.
var ErrNoSession = errors.New("calendar: no session on this date")

// ProviderError is the value a fatal provider failure panics with.
//
// The framework's calendar hooks are plain functions - one returning a
// bool, one returning a time - and those have nowhere to put an error.
// A provider that fails inside one raises a ProviderError, and the App
// turns it back into an error return so a strategy sees a failed run
// rather than a crashed process.
type ProviderError struct {
	// Provider is the provider that failed.
	Provider Provider

	// Err is the underlying failure.
	Err error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("calendar: provider failed: %v", e.Err)
}

// Unwrap gives the underlying failure to errors.Is and errors.As.
func (e *ProviderError) Unwrap() error { return e.Err }

// NewTradingDayFunc adapts a Provider to the trading-day predicate the
// clock, the scheduler and the context take.
//
// A provider failure panics with a *ProviderError: the predicate has
// no way to report an error, and continuing would mean trading on a
// schedule the framework could not confirm.
func NewTradingDayFunc(p Provider) func(time.Time) bool {
	return func(date time.Time) bool {
		ok, err := p.TradingDay(date)
		if err != nil {
			panic(&ProviderError{Provider: p, Err: err})
		}
		return ok
	}
}

// NewEarlyCloseFunc adapts a Provider to the early-close predicate.
func NewEarlyCloseFunc(p Provider) func(time.Time) bool {
	return func(date time.Time) bool {
		early, err := p.EarlyClose(date)
		if err != nil {
			panic(&ProviderError{Provider: p, Err: err})
		}
		return early
	}
}

// NewOpenFunc adapts a Provider to the session-open function.
//
// Like NewTradingDayFunc, a provider failure panics with a
// *ProviderError.
func NewOpenFunc(p Provider) func(time.Time) time.Time {
	return func(date time.Time) time.Time {
		open, err := p.SessionOpen(date)
		if err != nil {
			panic(&ProviderError{Provider: p, Err: err})
		}
		return open
	}
}

// NewCloseFunc adapts a Provider to the session-close function the
// clock and the App take.
//
// Like NewTradingDayFunc, a provider failure panics with a
// *ProviderError.
func NewCloseFunc(p Provider) func(time.Time) time.Time {
	return func(date time.Time) time.Time {
		close, err := p.SessionClose(date)
		if err != nil {
			panic(&ProviderError{Provider: p, Err: err})
		}
		return close
	}
}

// WithCloseFunc returns a Provider that answers SessionClose with fn
// and delegates everything else to p.
//
// It is how a house view of the schedule replaces the exchange's: a
// venue that closes at a different hour, or a data vendor whose early
// closes are not the exchange's.
func WithCloseFunc(p Provider, fn func(time.Time) time.Time) Provider {
	return overridden{Provider: p, closeFn: fn}
}

// WithTradingDayFunc returns a Provider that answers TradingDay with
// fn and delegates everything else to p.
//
// It is how a strategy that trades on a different exchange's days
// replaces the exchange's.
func WithTradingDayFunc(p Provider, fn func(time.Time) bool) Provider {
	return overridden{Provider: p, tradingDayFn: fn}
}

// WithSessionFallback returns a Provider that answers SessionOpen and
// SessionClose with the standard session on the days p reports no
// session for.
//
// A strategy that trades on its own days needs a session to resolve
// its slots against, and the exchange calendar has none for a day it
// was closed. The standard session - 09:30 to 16:00 ET - stands in.
//
// A provider failure is not a missing session: it is still an error,
// and still fatal.
func WithSessionFallback(p Provider) Provider {
	return sessionFallback{Provider: p}
}

// sessionFallback is a Provider that supplies the standard session on
// the days the wrapped provider has no session for.
type sessionFallback struct {
	Provider
}

// SessionOpen answers with the standard open when there is no session.
func (f sessionFallback) SessionOpen(date time.Time) (time.Time, error) {
	open, err := f.Provider.SessionOpen(date)
	if errors.Is(err, ErrNoSession) {
		return StandardSessionOpen(date), nil
	}
	return open, err
}

// SessionClose answers with the standard close when there is no session.
func (f sessionFallback) SessionClose(date time.Time) (time.Time, error) {
	close, err := f.Provider.SessionClose(date)
	if errors.Is(err, ErrNoSession) {
		return StandardSessionClose(date), nil
	}
	return close, err
}

// The standard session times, in ET. These are the times the exchange
// normally opens and closes, and they stand in on a day the exchange
// was closed.
const (
	// StandardOpenHour and StandardOpenMinute are the regular equity
	// session open.
	StandardOpenHour   = 9
	StandardOpenMinute = 30

	// StandardCloseHour and StandardCloseMinute are the regular equity
	// session close.
	StandardCloseHour   = 16
	StandardCloseMinute = 0
)

// StandardSessionOpen returns the open of the standard session on the
// date, in ET.
//
// The date is taken as written rather than converted.
func StandardSessionOpen(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, StandardOpenHour, StandardOpenMinute, 0, 0, NYSE)
}

// StandardSessionClose returns the close of the standard session on
// the date, in ET: 16:00.
//
// The date is taken as written rather than converted.
func StandardSessionClose(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, StandardCloseHour, StandardCloseMinute, 0, 0, NYSE)
}

// overridden is a Provider with some of its answers replaced.
type overridden struct {
	Provider

	closeFn      func(time.Time) time.Time
	tradingDayFn func(time.Time) bool
}

// TradingDay answers with the override, when there is one.
func (o overridden) TradingDay(date time.Time) (bool, error) {
	if o.tradingDayFn != nil {
		return o.tradingDayFn(date), nil
	}
	return o.Provider.TradingDay(date)
}

// SessionClose answers with the override, when there is one.
func (o overridden) SessionClose(date time.Time) (time.Time, error) {
	if o.closeFn != nil {
		return o.closeFn(date), nil
	}
	return o.Provider.SessionClose(date)
}
