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
