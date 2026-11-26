package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/job"
)

// fakeLiveBroker is a broker a live run can talk to.
type fakeLiveBroker struct {
	mu        sync.Mutex
	nav       float64
	positions []broker.PositionResponse
}

func (f *fakeLiveBroker) Health(context.Context) (*broker.HealthResponse, error) {
	return &broker.HealthResponse{Status: "ok", IBKRConnected: true}, nil
}

func (f *fakeLiveBroker) PlaceMarketOrder(context.Context, broker.MarketOrderRequest, string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *fakeLiveBroker) PlaceLimitOrder(context.Context, broker.LimitOrderRequest, string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *fakeLiveBroker) PlaceBracketOrder(context.Context, broker.BracketOrderRequest, string) (*broker.BracketOrderResponse, error) {
	return &broker.BracketOrderResponse{ParentOrderId: 1}, nil
}

func (f *fakeLiveBroker) UpdateOrder(context.Context, int, broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *fakeLiveBroker) CancelOrder(context.Context, int) error { return nil }

func (f *fakeLiveBroker) GetPositions(context.Context) ([]broker.PositionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]broker.PositionResponse, len(f.positions))
	copy(out, f.positions)
	return out, nil
}

func (f *fakeLiveBroker) GetOpenOrders(context.Context) ([]broker.OpenOrderResponse, error) {
	return nil, nil
}

func (f *fakeLiveBroker) GetAccountNAV(context.Context) (*broker.AccountNAVResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &broker.AccountNAVResponse{Account: "X", NAV: f.nav, Currency: "USD"}, nil
}

// TestApp_LiveWritesATearsheet checks that a live run leaves a tear
// sheet for the day behind.
func TestApp_LiveWritesATearsheet(t *testing.T) {
	dir := t.TempDir()

	a := app.NewApp(
		app.WithCalendarProvider(published.New()),
		app.WithLive(app.LiveConfig{Broker: &fakeLiveBroker{nav: 5_000}}),
		app.WithMarketClient(fakeMarket{}),
		app.WithUniverseClient(fakeUniverse{}),
		app.WithConfigClient(fakeConfig{}),
		app.WithTearsheetDir(dir),
	)
	if err := a.RegisterJob(job.Job{
		Name: "demo", Time: job.Time(16, 0), Handler: noopJob,
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := a.RunContext(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the tearsheet directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one tearsheet, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "live-") {
		t.Errorf("expected a daily tearsheet, got %q", entries[0].Name())
	}

	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	// The account's value is the broker's own NAV.
	if !strings.Contains(string(body), "$5,000.00") {
		t.Error("expected the tearsheet to carry the account's value")
	}
}
