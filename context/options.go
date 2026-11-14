package context

import (
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/portfolio"
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
	// Session is built from the Clock, the default market close and
	// the trading-day predicate.
	Session *clock.Session
}

// GetSession returns the configured Session, building a default one
// from the Clock and the trading-day predicate when none was supplied.
func (opts *Options) GetSession() *clock.Session {
	if opts.Session != nil {
		return opts.Session
	}
	return clock.NewSession(opts.Clock, nil, opts.GetIsTradingDay())
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

	return nil
}

// GetIsTradingDay returns the configured trading-day predicate, falling
// back to portfolio.NYSEWeekdays when none was supplied.
func (opts *Options) GetIsTradingDay() TradingDayFunc {
	if opts.IsTradingDay == nil {
		return portfolio.NYSEWeekdays
	}

	return opts.IsTradingDay
}

// GetMode returns the configured mode, falling back to ModeBacktest.
func (opts *Options) GetMode() job.Mode {
	if opts.Mode == "" {
		return job.ModeBacktest
	}

	return opts.Mode
}
