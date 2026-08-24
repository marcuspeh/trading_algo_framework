package tradingframework

// Portfolio is the framework-level view of positions and cash.
//
// The strategy reads portfolio state through Context, never directly
// from a Portfolio implementation.
type Portfolio interface {
	Cash() float64
	Position(symbol string) Position
	Positions() []Position
	Value() float64
}