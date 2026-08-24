package tradingframework

type Portfolio interface {
	Cash() float64
	Position(symbol string) Position
	Positions() []Position
	Value() float64
}
