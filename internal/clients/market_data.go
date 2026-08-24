// Package clients contains adapters that wrap the existing client/
// packages (broker, market_data, config) so the framework's internal
// interfaces get real data.
//
// Strategies should not import this package directly. The App wires
// the adapters on their behalf.
package clients

import (
	"context"
	"fmt"
	"sort"
	"time"

	broker "github.com/marcuspeh/live_algo/go_utils/broker"

	tf "github.com/marcuspeh/trading-framework"
)

// BrokerOHLC is the minimal subset of broker functionality the market-
// data adapter needs.
type BrokerOHLC interface {
	GetOHLC(ctx context.Context, symbols []string) ([]broker.OHLCResponse, error)
}

// MarketData adapts the broker client's GetOHLC endpoint to the
// framework's MarketDataClient interface.
//
// Note: the underlying endpoint returns a single OHLC bar per symbol
// (current/latest). Historical lookback is not supported by the broker
// client today; backtests should pre-load bars via the BarSource
// passed to the backtest engine, not through this adapter.
type MarketData struct {
	client BrokerOHLC
}

// NewMarketData constructs the market-data adapter.
func NewMarketData(c BrokerOHLC) *MarketData {
	return &MarketData{client: c}
}

// Bars returns a single-bar slice for the requested symbol, tagged with
// the broker-supplied date. lookback is capped at 1; requesting more
// returns an error because the broker endpoint does not yet support
// historical windows.
func (m *MarketData) Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error) {
	if m.client == nil {
		return nil, fmt.Errorf("market-data adapter: nil broker client")
	}
	if lookback <= 0 {
		return nil, fmt.Errorf("market-data adapter: lookback must be > 0")
	}
	if lookback > 1 {
		return nil, fmt.Errorf("market-data adapter: broker endpoint only supports single-bar lookups; got lookback=%d", lookback)
	}
	if symbol == "" {
		return nil, fmt.Errorf("market-data adapter: symbol is required")
	}

	resp, err := m.client.GetOHLC(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	for _, r := range resp {
		if r.Symbol != symbol {
			continue
		}
		parsed, perr := time.Parse("2006-01-02", r.Date)
		if perr != nil {
			parsed = asOf
		}
		return []tf.Bar{{
			Time:   parsed,
			Open:   r.Open,
			High:   r.High,
			Low:    r.Low,
			Close:  r.Close,
			Volume: int64(r.Volume),
		}}, nil
	}
	return nil, tf.ErrNotFound
}

// StaticBars is an in-memory MarketDataClient used primarily by tests
// and the example strategy. It serves pre-loaded bars by date.
type StaticBars struct {
	// bars maps symbol -> ascending-by-date bar list.
	bars map[string][]tf.Bar
}

// NewStaticBars constructs a StaticBars from the supplied map.
func NewStaticBars(bars map[string][]tf.Bar) *StaticBars {
	out := &StaticBars{bars: map[string][]tf.Bar{}}
	for s, list := range bars {
		cp := make([]tf.Bar, len(list))
		copy(cp, list)
		sort.SliceStable(cp, func(i, j int) bool { return cp[i].Time.Before(cp[j].Time) })
		out.bars[s] = cp
	}
	return out
}

// Bars returns up to `lookback` bars ending on or before asOf.
func (s *StaticBars) Bars(_ context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error) {
	if lookback <= 0 {
		return nil, fmt.Errorf("static-bars: lookback must be > 0")
	}
	list, ok := s.bars[symbol]
	if !ok {
		return nil, tf.ErrNotFound
	}
	// Find the last bar whose Time is <= asOf.
	idx := sort.Search(len(list), func(i int) bool {
		return list[i].Time.After(asOf)
	})
	end := idx
	if end == 0 {
		return nil, tf.ErrNotFound
	}
	start := end - lookback
	if start < 0 {
		start = 0
	}
	out := make([]tf.Bar, end-start)
	copy(out, list[start:end])
	return out, nil
}

// StaticBarSource is a BarProvider for backtest fills backed by the
// same data as StaticBars.
type StaticBarSource struct {
	bars map[string][]tf.Bar
}

// NewStaticBarSource builds a StaticBarSource.
func NewStaticBarSource(bars map[string][]tf.Bar) *StaticBarSource {
	cp := map[string][]tf.Bar{}
	for s, list := range bars {
		inner := make([]tf.Bar, len(list))
		copy(inner, list)
		cp[s] = inner
	}
	return &StaticBarSource{bars: cp}
}

// Bar returns the bar for symbol on the supplied date (exact date match).
func (s *StaticBarSource) Bar(symbol string, date time.Time) (tf.Bar, bool) {
	list, ok := s.bars[symbol]
	if !ok {
		return tf.Bar{}, false
	}
	for _, b := range list {
		if sameBarDate(b.Time, date) {
			return b, true
		}
	}
	return tf.Bar{}, false
}

func sameBarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
