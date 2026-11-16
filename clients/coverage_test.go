package clients_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/calendar/published"
	"github.com/marcuspeh/trading-framework/clients"
)

// A session whose bar the service has not published yet is not
// recorded as covered, so a later request asks for it again rather
// than serving a stale series forever.
//
// The service publishes a session's bar once it has closed, so a request
// that arrives moments after the close can find the bar missing. If the
// adapter treated that range as answered, the session's bar would never
// appear.
func TestMarket_UnpublishedSessionIsAskedForAgain(t *testing.T) {
	dates := sessions(2024, time.January, 10)
	svc := &countingService{dates: dates}
	srv := httptest.NewServer(svc.handler())
	defer srv.Close()

	m := clients.NewMarket(marketdata.NewClient(srv.URL), published.New())

	// A session the service holds no bar for.
	future := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	// The error is expected: the service has no bar that far ahead.
	_, _ = m.Bars(context.Background(), "AAPL", 1, future)
	before := len(svc.requested())

	// Asking again must reach the service, because the session is
	// still outstanding.
	_, _ = m.Bars(context.Background(), "AAPL", 1, future)
	if got := len(svc.requested()); got <= before {
		t.Errorf("the unpublished session was not asked for again: %d requests, want more than %d",
			got, before)
	}
}
