package tradingframework

// Side is the side of an Order.
type Side int

const (
	// Buy indicates buying the instrument.
	Buy Side = iota
	// Sell indicates selling the instrument.
	Sell
)

// String returns "BUY" or "SELL".
func (s Side) String() string {
	if s == Buy {
		return "BUY"
	}
	return "SELL"
}

// OrderType is the type of an Order.
type OrderType int

const (
	// Market orders execute at the next available price.
	Market OrderType = iota
	// Limit orders execute at the supplied Limit price or better.
	Limit
)

// String returns "MARKET" or "LIMIT".
func (t OrderType) String() string {
	if t == Limit {
		return "LIMIT"
	}
	return "MARKET"
}

// Order is a broker-neutral order returned by a Job handler.
//
// The framework converts these into broker-specific calls via the
// configured Executor. IBKR-specific fields do not belong here.
type Order struct {
	// ID is populated by the framework after submission. Strategies
	// may leave it empty when returning orders.
	ID string

	// Symbol is the ticker or instrument identifier.
	Symbol string

	// Side is Buy or Sell.
	Side Side

	// Quantity is the number of shares/contracts to trade.
	Quantity float64

	// Type is Market or Limit.
	Type OrderType

	// Limit is the limit price, required when Type == Limit.
	Limit *float64
}
