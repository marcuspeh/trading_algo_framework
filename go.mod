module github.com/marcuspeh/trading-framework

go 1.23.2

require (
	github.com/marcuspeh/go-tools v0.0.0-20260322035212-e79095b18af5
	github.com/marcuspeh/live_algo/go_utils/broker v0.0.0
	github.com/marcuspeh/live_algo/go_utils/etf v0.0.0
)

require (
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
)

replace github.com/marcuspeh/live_algo/go_utils/broker => ./client/broker

replace github.com/marcuspeh/live_algo/go_utils/etf => ./client/market_data
