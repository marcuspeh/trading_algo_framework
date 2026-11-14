package clients_test

import (
	"context"
	"testing"
	"time"

	ibkrproxy "github.com/marcuspeh/ibkr-proxy/sdk/go"
	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// The ibkr-proxy SDK's client must satisfy the framework's broker
// interface, so it can be handed straight to app.WithLive.
var _ broker.Client = (*ibkrproxy.BrokerClient)(nil)

// The framework's live client wraps it and must satisfy it too.
var _ broker.Client = (*clients.Broker)(nil)

// TestLiveWiringCompiles mirrors the live wiring documented in the
// README. It does not talk to any server; it only proves the types line
// up.
func TestLiveWiringCompiles(t *testing.T) {
	ctx := context.Background()

	brokerClient := clients.NewBroker(ctx, clients.BrokerConfig{
		BaseURL: "http://localhost",
		Port:    5001,
	})
	marketClient := clients.NewMarket(marketdata.NewClient("http://localhost:8001"))
	universeClient := clients.NewConstituents(marketdata.NewClient("http://localhost:8001"))

	a := app.NewApp(
		app.WithLive(app.LiveConfig{Broker: brokerClient}),
		app.WithMarketClient(marketClient),
		app.WithUniverseClient(universeClient),
		app.WithConfigClient(&stubConfig{}),
	)
	if err := a.RegisterJob(job.Job{
		Name: "rebalance", Time: job.Time(16, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	// Run is not called: it would try to reach the broker.
}

type stubConfig struct{}

func (stubConfig) Get(string) (string, error)                  { return "", job.ErrNotFound }
