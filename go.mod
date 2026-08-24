module github.com/marcuspeh/trading-framework

go 1.23.2

require (
	github.com/marcuspeh/live_algo/go_utils/broker v0.0.0
	github.com/marcuspeh/live_algo/go_utils/etf v0.0.0
)

replace github.com/marcuspeh/live_algo/go_utils/broker => ./client/broker

replace github.com/marcuspeh/live_algo/go_utils/etf => ./client/market_data
