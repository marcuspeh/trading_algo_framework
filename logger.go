package tradingframework

// Logger is the minimal logging interface exposed to strategy code.
//
// The framework provides a TXT-file Logger implementation suitable for
// both backtests and live runs.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}