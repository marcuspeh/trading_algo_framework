package app

import "github.com/marcuspeh/trading-framework/live"

func newLiveEngine(a *App) (*live.Engine, error) {
	return live.New(live.Config{
		Broker:       a.broker,
		Market:       a.market,
		Constituents: a.constituents,
		Config:       a.config,
		Logger:       a.logger,
		IsTradingDay: a.tradingDay,
		Risk:         a.risk,
		Clock:        a.liveClock,
	})
}
