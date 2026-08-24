package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

// ConfigClient is the minimal interface the config adapter needs from
// the existing config client. The concrete *config.ConfigClient from
// the project's client/config subpackage satisfies this.
type ConfigClient interface {
	Get(ctx context.Context, key string) (string, error)
}

// WatcherGetCloser is the subset of a typed config watcher used by the
// adapter. Strategies typically use the existing
// *config.ConfigWatcher[T] which satisfies this interface for any T.
type WatcherGetCloser[T any] interface {
	Get() T
	Close()
}

// TypedFactory is the typed factory signature the framework expects.
// Callers pass functions like:
//
//	func(ctx context.Context, c *config.ConfigClient, key string) (*config.ConfigWatcher[MyType], error)
//
// The framework casts the client to *config.ConfigClient internally.
type TypedFactory[T any] func(ctx context.Context, c ConfigClient, key string) (WatcherGetCloser[T], error)

// Config adapts the existing config client to the framework's Config
// interface. It supports one-shot Get calls and typed reactive watchers.
type Config struct {
	client   ConfigClient
	mu       sync.Mutex
	watchers []func()
}

// NewConfig constructs the config adapter.
func NewConfig(c ConfigClient) *Config {
	return &Config{client: c}
}

// Get returns the raw string value for the supplied key.
func (c *Config) Get(key string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("config adapter: nil client")
	}
	return c.client.Get(context.Background(), key)
}

// Watch starts a typed watcher and returns a framework Watcher. out
// must be a non-nil pointer to T.
//
// Usage:
//
//	type MyConfig struct { Foo string `json:"foo"` }
//	w, err := cfg.Watch("my-key", &MyConfig{}, clients.WatchFactory(func(ctx context.Context, c *config.ConfigClient, key string) (*config.ConfigWatcher[MyConfig], error) {
//	    return config.NewConfigWatcher[MyConfig](ctx, c, key)
//	}))
//	defer w.Stop()
func (c *Config) Watch(key string, out any, factory any) (tf.Watcher, error) {
	if c.client == nil {
		return nil, fmt.Errorf("config adapter: nil client")
	}
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, fmt.Errorf("config adapter: out must be a non-nil pointer")
	}
	target := rv.Elem().Type()

	// Detect factory signature: func(ctx, ConfigClient, string) (WatcherGetCloser[T], error)
	// We accept any concrete type satisfying WatcherGetCloser[?].
	fv := reflect.ValueOf(factory)
	if fv.Kind() != reflect.Func {
		return nil, fmt.Errorf("config adapter: factory must be a function")
	}
	ft := fv.Type()
	if ft.NumIn() != 3 || ft.NumOut() != 2 {
		return nil, fmt.Errorf("config adapter: factory must have signature func(ctx, ConfigClient, string) (Watcher, error)")
	}
	// First result must implement a single-method interface with Get() T and Close().
	if ft.Out(0).Kind() != reflect.Ptr && ft.Out(0).Kind() != reflect.Interface {
		return nil, fmt.Errorf("config adapter: factory's watcher result must be a pointer or interface")
	}

	ctxVal := reflect.ValueOf(context.Background())
	clientVal := reflect.ValueOf(c.client)
	keyVal := reflect.ValueOf(key)

	results := fv.Call([]reflect.Value{ctxVal, clientVal, keyVal})
	if !results[1].IsNil() {
		errVal := results[1].Interface().(error)
		return nil, errVal
	}
	watcher := results[0]

	// Snapshot the latest value on each poll.
	var mu sync.RWMutex
	var latest any
	getMethod := watcher.MethodByName("Get")
	if !getMethod.IsValid() {
		return nil, fmt.Errorf("config adapter: watcher has no Get method")
	}
	closeMethod := watcher.MethodByName("Close")
	if !closeMethod.IsValid() {
		return nil, fmt.Errorf("config adapter: watcher has no Close method")
	}

	mu.Lock()
	latest = getMethod.Call(nil)[0].Interface()
	mu.Unlock()

	pollCtx, cancel := context.WithCancel(context.Background())
	go func() {
		tick := time.NewTicker(1 * time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-tick.C:
				mu.Lock()
				latest = getMethod.Call(nil)[0].Interface()
				mu.Unlock()
			}
		}
	}()

	c.mu.Lock()
	c.watchers = append(c.watchers, func() {
		cancel()
		closeMethod.Call(nil)
	})
	c.mu.Unlock()

	gw := &genericWatcher{
		typ: target,
		get: func() any {
			mu.RLock()
			defer mu.RUnlock()
			return latest
		},
	}
	return gw, nil
}

// StopAll terminates every active watcher.
func (c *Config) StopAll() {
	c.mu.Lock()
	ws := c.watchers
	c.watchers = nil
	c.mu.Unlock()
	for _, fn := range ws {
		fn()
	}
}

// genericWatcher decodes the latest snapshot into the caller-supplied
// pointer on every Value() call. We round-trip through JSON so the
// adapter works for any JSON-tagged struct without reflection writes.
type genericWatcher struct {
	typ reflect.Type
	get func() any
}

func (g *genericWatcher) Value(out any) error {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("config watcher: out must be a pointer")
	}
	if rv.Elem().Type() != g.typ {
		return fmt.Errorf("config watcher: type mismatch (got %s, want %s)", rv.Elem().Type(), g.typ)
	}
	latest := g.get()
	if latest == nil {
		return fmt.Errorf("config watcher: no value yet")
	}
	b, err := json.Marshal(latest)
	if err != nil {
		return fmt.Errorf("config watcher: marshal: %w", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("config watcher: unmarshal: %w", err)
	}
	return nil
}

func (g *genericWatcher) Stop() {
	// Watch lifetime is managed by Config.StopAll / the App shutdown.
}
