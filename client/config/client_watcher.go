package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type ClientWatcher[configStruct any, clientStruct any] struct {
	client       *ConfigClient
	key          string
	value        any
	lastRawValue string
	initClient   func(ctx context.Context, value *configStruct) (clientStruct, error)
	mu           sync.RWMutex
	cancel       context.CancelFunc
}

func NewClientWatcher[configStruct any, clientStruct any](ctx context.Context, client *ConfigClient, key string, initClient func(ctx context.Context, value *configStruct) (clientStruct, error)) (*ClientWatcher[configStruct, clientStruct], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	w := &ClientWatcher[configStruct, clientStruct]{
		client:     client,
		key:        key,
		cancel:     cancel,
		initClient: initClient,
	}

	val, err := client.Get(watchCtx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to initial fetch config key %s: %w", key, err)
	}

	{
		w.mu.Lock()
		defer w.mu.Unlock()

		var config configStruct
		err := json.Unmarshal([]byte(val), &config)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config key %s: %w", key, err)
		}

		w.value, err = w.initClient(watchCtx, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to init client for config key %s: %w", key, err)
		}
		w.lastRawValue = val
	}

	go w.watch(watchCtx)

	return w, nil
}

func (w *ClientWatcher[configStruct, clientStruct]) watch(ctx context.Context) {
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

				var config configStruct
				err := json.Unmarshal([]byte(val), &config)
				if err != nil {
					log.Printf("failed to unmarshal config key %s: %v", w.key, err)
					continue
				}

				newValue, err := w.initClient(ctx, &config)
				if err != nil {
					log.Printf("failed to init client for config key %s: %v", w.key, err)
					continue
				}

				w.value = newValue
				w.lastRawValue = val
			}
		}
	}
}

func (w *ClientWatcher[configStruct, clientStruct]) Get() clientStruct {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.value.(clientStruct)
}

func (w *ClientWatcher[configStruct, clientStruct]) Close() {
	closeClient, ok := w.value.(CloseClient)
	if ok {
		closeClient.Close()
	}

	w.cancel()
}

type CloseClient interface {
	Close()
}