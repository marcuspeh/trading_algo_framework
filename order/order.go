package order

import "time"

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

type Bar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

type Result struct {
	ID        string
	Filled    bool
	FillPrice float64
	Error     error
}
