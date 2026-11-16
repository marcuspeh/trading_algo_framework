package context

import (
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/universe"
)

// Options bundles the dependencies required to construct a Context.
type Options struct {
	Clock        clock.Clock
	Portfolio    Portfolio
	Market       MarketClient
	Universe     universe.Client
	Config       Config
	Logger       Logger
	IsTradingDay TradingDayFunc
	Mode         job.Mode

	// Broker reaches the broker to manage resting orders. It is
	// optional: without it, OpenOrders, CancelOrder and UpdateOrder
	// report a missing dependency.
	Broker job.OrderBroker

	// Session resolves the session a timestamp belongs to. When nil, a
	// Session is built from the Clock and the Calendar.
	Session *clock.Session

	// Calendar is the exchange calendar: it says which days the
	// exchange traded and when each session opened and closed. It is
	// required when no Session is supplied.
	Calendar calendar.Provider
}

// GetSession returns the configured Session, building a default one
// from the Clock and the Calendar when none was supplied.
func (opts *Options) GetSession() *clock.Session {
	if opts.Session != nil {
		return opts.Session
	}
	if opts.Calendar == nil {
		return nil
	}
	return clock.NewSession(opts.Clock, opts.Calendar)
}

func (opts *Options) Validate() error {
	if opts.Clock == nil {
		return errMissing("clock")
	}
	if opts.Portfolio == nil {
		return errMissing("portfolio")
	}
	if opts.Market == nil {
		return errMissing("market")
	}
	if opts.Universe == nil {
		return errMissing("universe")
	}
	if opts.Config == nil {
		return errMissing("config")
	}
	if opts.Logger == nil {
		return errMissing("logger")
	}
	if opts.Session == nil && opts.Calendar == nil {
		return errMissing("calendar")
	}

	return nil
}

// GetIsTradingDay returns the configured trading-day predicate.
//
// When none was supplied the predicate comes from the calendar: the
// exchange's own trading days.
func (opts *Options) GetIsTradingDay() TradingDayFunc {
	if opts.IsTradingDay != nil {
		return opts.IsTradingDay
	}
	if opts.Calendar != nil {
		return calendar.NewTradingDayFunc(opts.Calendar)
	}
	if opts.Session != nil {
		return opts.Session.IsTradingDay
	}

	return func(time.Time) bool { return true }
}

// GetMode returns the configured mode, falling back to ModeBacktest.
func (opts *Options) GetMode() job.Mode {
	if opts.Mode == "" {
		return job.ModeBacktest
	}

	return opts.Mode
}
