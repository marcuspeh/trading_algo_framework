package tradingframework

// RiskManager validates orders returned by Jobs before they reach the
// Executor. The default implementation rejects invalid quantities,
// empty symbols, and unsupported order types.
//
// The interface exists so risk logic can evolve without changing
// strategy code.
type RiskManager interface {
	Validate(ctx Context, orders []Order) ([]Order, error)
}