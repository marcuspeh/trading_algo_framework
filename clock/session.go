package clock

import (
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
)

// Session ties a Clock to the exchange's calendar.
//
// It answers two questions the framework keeps asking: when does the
// current session end, and which session has already finished. The
// second is the anchor - the most recent session whose close has
// passed - and it is what stops a job from seeing data that had not
// happened yet.
//
// One Session is built by the App and handed to everything that needs
// it: the Context, the backtest engine, the mock broker and the live
// reconciler. Sharing one is what keeps them from disagreeing about
// which session a timestamp belongs to.
//
// The session's boundaries come from a calendar provider, which is the
// exchange's own schedule. A provider that cannot answer fails the
// run: the framework will not guess when a session ended.
type Session struct {
	clk        Clock
	openFn     func(time.Time) time.Time
	closeFn    func(time.Time) time.Time
	tradingDay func(time.Time) bool
}

// NewSession returns a Session for the given clock and calendar.
//
// The provider is the exchange calendar: it says which days the
// exchange traded and when each session opened and closed. A nil
// provider is a programming error - a session without a calendar
// cannot say when a session ends - so it panics.
func NewSession(clk Clock, provider calendar.Provider) *Session {
	if clk == nil {
		clk = NewReal()
	}
	if provider == nil {
		panic("clock: a session needs a calendar provider")
	}
	return &Session{
		clk:        clk,
		openFn:     calendar.NewOpenFunc(provider),
		closeFn:    calendar.NewCloseFunc(provider),
		tradingDay: calendar.NewTradingDayFunc(provider),
	}
}

// Clock returns the underlying clock.
func (s *Session) Clock() Clock { return s.clk }

// IsTradingDay reports whether the exchange is open on the date.
func (s *Session) IsTradingDay(date time.Time) bool {
	return s.tradingDay(date)
}

// NextTradingDay returns the next trading day strictly after the given
// date.
//
// It uses the session's own trading-day predicate, so a strategy that
// runs on a different exchange's days gets those days.
func (s *Session) NextTradingDay(date time.Time) time.Time {
	candidate := date.AddDate(0, 0, 1)
	for i := 0; i < 14; i++ {
		if s.tradingDay(candidate) {
			return candidate
		}
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}

// PreviousTradingDay returns the most recent trading day strictly
// before the given date.
func (s *Session) PreviousTradingDay(date time.Time) time.Time {
	candidate := date.AddDate(0, 0, -1)
	for i := 0; i < 14; i++ {
		if s.tradingDay(candidate) {
			return candidate
		}
		candidate = candidate.AddDate(0, 0, -1)
	}
	return candidate
}

// Open returns the open of the session on the given date.
func (s *Session) Open(date time.Time) time.Time {
	return s.openFn(date)
}

// Close returns the close of the session on the given date.
func (s *Session) Close(date time.Time) time.Time {
	return s.closeFn(date)
}

// Anchor returns the most recent session whose close has passed.
//
// A job that fires before the close reads this session's predecessor:
// at 09:00 on Tuesday the anchor is Monday, because Monday's close has
// passed and Tuesday's has not. A job that fires at or after the close
// reads the current session.
//
// The anchor is the framework's answer to lookahead. Market data,
// fills and mark-to-market all resolve their date through it, so they
// can never disagree about which session a timestamp belongs to.
func (s *Session) Anchor(now time.Time) time.Time {
	d := calendarStartOfDay(now)
	for i := 0; i < 14; i++ {
		if s.tradingDay(d) && !s.closeFn(d).After(now) {
			return d
		}
		d = s.PreviousTradingDay(d)
	}
	// Unreachable: the NYSE never closes for more than a week.
	return d
}

// calendarStartOfDay returns midnight of the given date in the NYSE
// calendar, taking the date as written rather than converted.
func calendarStartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, calendar.NYSE)
}
