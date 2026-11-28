package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/live"
)

// A run that cannot listen for its fills fails rather than logging
// nothing about them and looking healthy.
func TestEngine_FailsFastWhenTheFillListenerCannotBind(t *testing.T) {
	b := &fakeBroker{health: broker.HealthResponse{Status: "ok", IBKRConnected: true}}
	engine, err := live.New(live.Config{
		Broker:      b,
		Calendar:    published.New(),
		Fills:       fills.NewRegistry(),
		FillWebhook: "127.0.0.1:-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = engine.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("expected the run to fail when the fill listener cannot bind")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("error = %v want it to name the listener", err)
	}
}
