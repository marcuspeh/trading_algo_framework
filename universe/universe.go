// Package universe defines the interface the framework uses to fetch
// ETF/index constituents. The clients/ package supplies the real adapter.
package universe

import (
	"context"
	"time"
)

type Client interface {
	Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error)
}
