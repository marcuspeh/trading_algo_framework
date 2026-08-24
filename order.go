package tradingframework

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
