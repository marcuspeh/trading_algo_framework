package clients

import (
	"context"
	"fmt"
	"time"

	etf "github.com/marcuspeh/live_algo/go_utils/etf"

	tf "github.com/marcuspeh/trading-framework"
)

// ETFClient is the subset of etf.ETFClient functionality the constituents
// adapter needs.
type ETFClient interface {
	GetConstituents(ctx context.Context, symbol string) ([]etf.Constituent, error)
}

// Constituents adapts the ETF client to the framework's
// ConstituentsClient interface. The asOf argument is currently
// informational only — the existing client returns the current
// universe.
type Constituents struct {
	client ETFClient
}

// NewConstituents constructs the constituents adapter.
func NewConstituents(c ETFClient) *Constituents {
	return &Constituents{client: c}
}

// Constituents returns ticker symbols for the supplied ETF.
func (c *Constituents) Constituents(ctx context.Context, etfSymbol string, _ time.Time) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("constituents adapter: nil etf client")
	}
	if etfSymbol == "" {
		return nil, fmt.Errorf("constituents adapter: etf symbol required")
	}
	list, err := c.client.GetConstituents(ctx, etfSymbol)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, c := range list {
		if c.Ticker == "" {
			continue
		}
		out = append(out, c.Ticker)
	}
	return out, nil
}

// StaticConstituents is an in-memory ConstituentsClient used primarily
// by tests and the example strategy.
type StaticConstituents struct {
	// byDate maps etf -> sorted date list, and date -> symbols.
	byDate map[string]map[string][]string
}

// NewStaticConstituents constructs a StaticConstituents.
func NewStaticConstituents(data map[string]map[string][]string) *StaticConstituents {
	cp := map[string]map[string][]string{}
	for etfSym, byDate := range data {
		cp[etfSym] = map[string][]string{}
		for d, syms := range byDate {
			inner := make([]string, len(syms))
			copy(inner, syms)
			cp[etfSym][d] = inner
		}
	}
	return &StaticConstituents{byDate: cp}
}

// Constituents returns the most recent universe whose date is <= asOf.
func (s *StaticConstituents) Constituents(_ context.Context, etfSym string, asOf time.Time) ([]string, error) {
	if etfSym == "" {
		return nil, fmt.Errorf("static-constituents: etf required")
	}
	byDate, ok := s.byDate[etfSym]
	if !ok {
		return nil, tf.ErrNotFound
	}
	var bestDate string
	for d := range byDate {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		if parsed.After(asOf) {
			continue
		}
		if bestDate == "" || d > bestDate {
			bestDate = d
		}
	}
	if bestDate == "" {
		return nil, tf.ErrNotFound
	}
	out := make([]string, len(byDate[bestDate]))
	copy(out, byDate[bestDate])
	return out, nil
}
