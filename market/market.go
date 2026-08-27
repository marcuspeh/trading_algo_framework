// Package market defines the interface the framework uses to fetch
// historical bars. The clients/ package supplies the real adapter.
package market

import (
	"context"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

type Client interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error)
}
