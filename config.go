package tradingframework

type Config interface {
	Get(key string) (string, error)
	Watch(key string, out any, factory any) (Watcher, error)
}

type Watcher interface {
	Value(out any) error
	Stop()
}
