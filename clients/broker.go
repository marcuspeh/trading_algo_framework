// Package clients contains HTTP clients and adapters that talk to the
// external services (broker, ETF universe) and present them in the
// shape the framework's internal interfaces expect.
//
// Strategies should not import this package directly. The App wires
// the adapters on their behalf.
//
// This package has no submodules - all broker/etf/adapter code lives
// in a single Go package.
package clients

import (
	"context"
	"fmt"
	"time"

	ibkrproxy "github.com/marcuspeh/ibkr-proxy/sdk/go"
	"github.com/marcuspeh/trading-framework/broker"
)

// BrokerConfig describes where the broker service lives.
type BrokerConfig struct {
	BaseURL string
	Port    int

	// Timeout bounds every call to the broker. Zero means
	// DefaultBrokerTimeout.
	Timeout time.Duration
}

// DefaultBrokerTimeout is the deadline applied to a broker call when
// none is configured.
const DefaultBrokerTimeout = 30 * time.Second

// Broker is a live broker client.
//
// It wraps the ibkr-proxy SDK's client and adds a deadline to every
// call. The SDK sets no timeout on its HTTP client, so without this a
// stalled broker would hang the trading session indefinitely.
type Broker struct {
	client  broker.Client
	timeout time.Duration
}

// Compile-time proof that the wrapper satisfies the framework's broker
// interface.
var _ broker.Client = (*Broker)(nil)

// NewBroker returns a live broker client for the given configuration.
func NewBroker(ctx context.Context, config BrokerConfig) *Broker {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultBrokerTimeout
	}
	return &Broker{
		client:  ibkrproxy.NewBrokerClient(ctx, brokerURL(config)),
		timeout: timeout,
	}
}

// NewBrokerWithClient wraps an existing broker client with the
// framework's deadline handling. It is how a test supplies a fake.
func NewBrokerWithClient(c broker.Client, timeout time.Duration) *Broker {
	if timeout <= 0 {
		timeout = DefaultBrokerTimeout
	}
	return &Broker{client: c, timeout: timeout}
}

// brokerURL builds the base URL the SDK expects.
func brokerURL(config BrokerConfig) string {
	return fmt.Sprintf("%s:%d", config.BaseURL, config.Port)
}

// deadline derives a context bounded by the configured timeout.
func (b *Broker) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, b.timeout)
}

func (b *Broker) Health(ctx context.Context) (*broker.HealthResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.Health(ctx)
}

func (b *Broker) PlaceMarketOrder(ctx context.Context, req broker.MarketOrderRequest, idempotencyKey string) (*broker.OrderResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.PlaceMarketOrder(ctx, req, idempotencyKey)
}

func (b *Broker) PlaceLimitOrder(ctx context.Context, req broker.LimitOrderRequest, idempotencyKey string) (*broker.OrderResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.PlaceLimitOrder(ctx, req, idempotencyKey)
}

func (b *Broker) PlaceBracketOrder(ctx context.Context, req broker.BracketOrderRequest, idempotencyKey string) (*broker.BracketOrderResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.PlaceBracketOrder(ctx, req, idempotencyKey)
}

func (b *Broker) UpdateOrder(ctx context.Context, orderID int, req broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.UpdateOrder(ctx, orderID, req)
}

func (b *Broker) CancelOrder(ctx context.Context, orderID int) error {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.CancelOrder(ctx, orderID)
}

func (b *Broker) GetPositions(ctx context.Context) ([]broker.PositionResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.GetPositions(ctx)
}

func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.OpenOrderResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.GetOpenOrders(ctx)
}

func (b *Broker) GetAccountNAV(ctx context.Context) (*broker.AccountNAVResponse, error) {
	ctx, cancel := b.deadline(ctx)
	defer cancel()
	return b.client.GetAccountNAV(ctx)
}
