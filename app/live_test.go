package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// liveFakeBroker is a broker the test controls.
type liveFakeBroker struct {
	mu        sync.Mutex
	positions []broker.PositionResponse
	nav       float64
}

func (f *liveFakeBroker) Health(_ context.Context) (*broker.HealthResponse, error) {
	return &broker.HealthResponse{Status: "ok", IBKRConnected: true}, nil
}

func (f *liveFakeBroker) PlaceMarketOrder(_ context.Context, _ broker.MarketOrderRequest, _ string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *liveFakeBroker) PlaceLimitOrder(_ context.Context, _ broker.LimitOrderRequest, _ string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: 1}, nil
}

func (f *liveFakeBroker) PlaceBracketOrder(_ context.Context, _ broker.BracketOrderRequest, _ string) (*broker.BracketOrderResponse, error) {
	return &broker.BracketOrderResponse{ParentOrderId: 1}, nil
}

func (f *liveFakeBroker) UpdateOrder(_ context.Context, id int, _ broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{OrderId: id}, nil
}

func (f *liveFakeBroker) CancelOrder(_ context.Context, _ int) error { return nil }

func (f *liveFakeBroker) GetPositions(_ context.Context) ([]broker.PositionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]broker.PositionResponse, len(f.positions))
	copy(out, f.positions)
	return out, nil
}

func (f *liveFakeBroker) GetOpenOrders(_ context.Context) ([]broker.OpenOrderResponse, error) {
	return nil, nil
}

func (f *liveFakeBroker) GetAccountNAV(_ context.Context) (*broker.AccountNAVResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &broker.AccountNAVResponse{Account: "X", NAV: f.nav}, nil
}

// liveFakeUniverse and liveFakeConfig satisfy the app's dependencies.
type liveFakeUniverse struct{}

func (liveFakeUniverse) Constituents(context.Context, string, time.Time) ([]string, error) {
	return nil, nil
}

type liveFakeConfig struct{}

func (liveFakeConfig) Get(string) (string, error) { return "", job.ErrNotFound }

// In live mode ctx.Cash() must be the account's cash, derived from the
// NAV and the position marks.
//
// The broker reports a net asset value but not the cash in it, so the
// reconciler derives cash as NAV minus the market value of the
// positions. The app has to give the reconciler a price lookup for
// that; without one the cash stays unknown and ctx.Cash() is zero, and
// a strategy that sizes its positions off cash silently misbehaves.
func TestApp_LiveCashIsDerived(t *testing.T) {
	b := &liveFakeBroker{
		positions: []broker.PositionResponse{
			{ConId: 1, Symbol: "AAPL", Quantity: 10, AverageCost: 50},
		},
		nav: 100_000,
	}

	// Bars for the last several sessions, at 50. The reconciler marks
	// the position at the session the clock is anchored to, which is
	// the last session whose close has passed.
	bars := map[string][]order.Bar{}
	for _, d := range recentSessions() {
		bars["AAPL"] = append(bars["AAPL"], order.Bar{
			Symbol: "AAPL", Timestamp: d, Close: 50,
		})
	}

	a := NewApp(
		WithCalendarProvider(published.New()),
		WithLive(LiveConfig{Broker: b}),
		WithMarketClient(clients.NewStaticBars(bars)),
		WithUniverseClient(liveFakeUniverse{}),
		WithConfigClient(liveFakeConfig{}),
		WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "noop", Time: job.Time(16, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	// A cancelled context stops the live loop as soon as it starts,
	// but the startup reconciliation has already run by then.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.RunContext(ctx); err != nil {
		t.Fatal(err)
	}

	mirror := a.portfolio.(*portfolio.Live)
	if !mirror.CashKnown() {
		t.Fatal("the cash was not derived: the app did not give the reconciler a price lookup")
	}
	if got := mirror.Cash(); got != 99_500 {
		t.Errorf("cash=%f want 99500", got)
	}
	if got := mirror.Value(); got != 100_000 {
		t.Errorf("portfolio value=%f want 100000 (the broker's NAV)", got)
	}
}

// The live stack listens for the fills the broker reports, so a fill is
// logged when it happens rather than when the order was submitted.
func TestApp_LiveListensForFills(t *testing.T) {
	b := &liveFakeBroker{nav: 100_000}
	log := &captureLogger{}

	a := NewApp(
		WithCalendarProvider(published.New()),
		WithLive(LiveConfig{Broker: b, FillWebhook: "127.0.0.1:0"}),
		WithMarketClient(clients.NewStaticBars(map[string][]order.Bar{})),
		WithUniverseClient(liveFakeUniverse{}),
		WithConfigClient(liveFakeConfig{}),
		WithLogger(log),
		WithTearsheetDir(t.TempDir()),
	)
	if err := a.RegisterJob(job.Job{
		Name: "noop", Time: job.Time(16, 0),
		Handler: func(job.Context, time.Time) ([]order.Order, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.RunContext(ctx); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, line := range log.all() {
		if strings.Contains(line, "listening for fill webhooks") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("the run did not listen for fills: %v", log.all())
	}
}

// captureLogger keeps the lines the app logged.
type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (c *captureLogger) Info(msg string, fields ...any)  { c.add(msg, fields) }
func (c *captureLogger) Warn(msg string, fields ...any)  { c.add(msg, fields) }
func (c *captureLogger) Error(msg string, fields ...any) { c.add(msg, fields) }

func (c *captureLogger) add(msg string, fields []any) {
	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(&b, " %v=%v", fields[i], fields[i+1])
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, b.String())
}

func (c *captureLogger) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// recentSessions returns the last ten sessions, most recent first.
func recentSessions() []time.Time {
	out := make([]time.Time, 0, 10)
	day := time.Now().In(calendar.NYSE)
	for i := 0; len(out) < 10; i++ {
		if published.TradingDay(day) {
			out = append(out, time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, calendar.NYSE))
		}
		day = day.AddDate(0, 0, -1)
	}
	return out
}
