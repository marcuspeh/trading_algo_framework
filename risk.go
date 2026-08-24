package tradingframework

// RiskManager validates orders returned by Jobs before they reach the
// Executor. Strategies may swap it for custom logic.
type RiskManager interface {
	Validate(ctx Context, orders []Order) ([]Order, error)
}
