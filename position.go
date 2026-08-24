package tradingframework

// Position represents an open position in a single instrument.
type Position struct {
	Symbol   string
	Quantity float64
	AvgPrice float64
}