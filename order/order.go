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
	// ID is the strategy's own correlation id. It is sent to the
	// broker as the order reference, not as the idempotency key.
	ID string

	Symbol   string
	Side     Side
	Quantity float64
	Type     OrderType
	Limit    *float64

	// TIF is the time in force: "DAY", "GTC", "IOC", "FOK" or "GTD".
	// An empty value means "GTC", so a resting limit order survives
	// the session - the same in backtest and live.
	TIF string

	// TakeProfit and StopLoss attach protective legs to a limit order.
	// An order carrying both is sent as a bracket.
	TakeProfit *float64
	StopLoss   *float64

	// OutsideRTH allows the order to be submitted outside regular
	// trading hours.
	OutsideRTH bool
}

// TimeInForce returns the order's time in force, defaulting to GTC.
func (o Order) TimeInForce() string {
	if o.TIF == "" {
		return "GTC"
	}
	return o.TIF
}

// IsBracket reports whether the order carries both protective legs.
func (o Order) IsBracket() bool {
	return o.TakeProfit != nil && o.StopLoss != nil
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
