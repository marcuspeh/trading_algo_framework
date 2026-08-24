package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type ConfigWatcher[T any] struct {
	client       *ConfigClient
	key          string
	value        T
	lastRawValue string
	mu           sync.RWMutex
	cancel       context.CancelFunc
}

func NewConfigWatcher[T any](ctx context.Context, client *ConfigClient, key string) (*ConfigWatcher[T], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	w := &ConfigWatcher[T]{
		client: client,
		key:    key,
		cancel: cancel,
	}

	val, err := client.Get(watchCtx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to initial fetch config key %s: %w", key, err)
	}

	{
		w.mu.Lock()
		defer w.mu.Unlock()

		err := json.Unmarshal([]byte(val), &w.value)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config key %s: %w", key, err)
		}
		w.lastRawValue = val
	}

	go w.watch(watchCtx)

	return w, nil
}

func (w *ConfigWatcher[T]) watch(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			val, err := w.client.Get(ctx, w.key)
			if err != nil {
				log.Printf("failed to watch config key %s: %v", w.key, err)
				continue
			}

			{
				w.mu.Lock()
				defer w.mu.Unlock()
				if val == w.lastRawValue {
					continue
				}

				err := json.Unmarshal([]byte(val), &w.value)
				if err != nil {
					log.Printf("failed to unmarshal config key %s: %v", w.key, err)
					continue
				}
				w.lastRawValue = val
			}
		}
	}
}

func (w *ConfigWatcher[T]) Get() T {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.value
}

func (w *ConfigWatcher[T]) Close() {
	w.cancel()
}
