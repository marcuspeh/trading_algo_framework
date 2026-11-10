package order

import (
	indicator "github.com/marcuspeh/indicators-go"
)

type Side int

const (
	Buy Side = iota
	Sell
)

func (s Side) String() string {
	if s == Buy {
		return "BUY"
	}
	return "SELL"
}

type OrderType int

const (
	Market OrderType = iota
	Limit
)

func (t OrderType) String() string {
	if t == Limit {
		return "LIMIT"
	}
	return "MARKET"
}

type Order struct {
	ID       string
	Symbol   string
	Side     Side
	Quantity float64
	Type     OrderType
	Limit    *float64
}

type Position struct {
	Symbol   string
	Quantity float64
	AvgPrice float64
}

// Bar is a daily OHLCV bar.
//
// It is an alias of the bar type from github.com/marcuspeh/indicators-go,
// so bars can be handed straight to the indicator library without any
// conversion.
type Bar = indicator.Bar

type Result struct {
	ID        string
	Filled    bool
	FillPrice float64
	Error     error
}
