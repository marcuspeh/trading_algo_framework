package tradingframework

// Config is the strategy-facing configuration interface.
//
// It wraps the existing config client and supports both one-shot Get
// calls and long-lived watchers for reactive configuration.
type Config interface {
	// Get fetches a raw string value for the given key. Returns
	// ErrNotFound when the key does not exist.
	Get(key string) (string, error)

	// Watch returns a typed watcher that polls the underlying config
	// client and decodes JSON values into T. factory is a typed
	// factory function (see clients.Config.Watch for the signature).
	Watch(key string, out any, factory any) (Watcher, error)
}

// Watcher exposes the latest decoded value for a watched config key.
type Watcher interface {
	// Value returns the most recently decoded value. out must be a
	// pointer to the target type (matching the type used at Watch time).
	Value(out any) error
	// Stop terminates the background polling goroutine.
	Stop()
}
