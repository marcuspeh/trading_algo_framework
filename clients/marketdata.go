package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

// ETF HTTP client and constituent types.

type ETFClient struct {
	baseURL string
}

func NewETFClient(baseURL string) *ETFClient {
	if baseURL == "" {
		baseURL = "http://localhost:8001"
	}
	return &ETFClient{baseURL: baseURL}
}

func (c *ETFClient) GetConstituents(ctx context.Context, symbol string) ([]Constituent, error) {
	url := fmt.Sprintf("%s/constituents?symbol=%s", c.baseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from remote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Detail string `json:"detail"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return nil, fmt.Errorf("remote server returned error: %s", errResp.Detail)
		}
		return nil, fmt.Errorf("remote server returned status: %d", resp.StatusCode)
	}

	var result ConstituentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Constituents, nil
}

type Constituent struct {
	Ticker string  `json:"ticker"`
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type ConstituentResponse struct {
	Symbol       string        `json:"symbol"`
	Constituents []Constituent `json:"constituents"`
	Source       string        `json:"source"`
}

// MarketSource is the subset of marketdata.Client functionality the
// market adapter needs. It mirrors the SDK's two public methods.
type MarketSource interface {
	GetBars(ctx context.Context, ticker string, from, to time.Time) (*marketdata.BarsResponse, error)
	GetConstituents(ctx context.Context, symbol string, date time.Time) (*marketdata.ConstituentResponse, error)
}

// Market adapts the market-data-service SDK to the framework's
// market.Client interface.
//
// The SDK's GetBars endpoint returns a date range; we translate the
// (symbol, lookback, asOf) request into [from, asOf] inclusive and
// trim to the most recent `lookback` rows.
type Market struct {
	client MarketSource
}

func NewMarket(c MarketSource) *Market { return &Market{client: c} }

// Bars fetches up to `lookback` bars for symbol ending on or before asOf.
// The SDK's GetBars range is [from, to] inclusive; we set `to` = asOf and
// compute `from` from the lookback window.
func (m *Market) Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error) {
	if m.client == nil {
		return nil, fmt.Errorf("market adapter: nil market-data client")
	}
	if lookback <= 0 {
		return nil, fmt.Errorf("market adapter: lookback must be > 0")
	}
	if symbol == "" {
		return nil, fmt.Errorf("market adapter: symbol is required")
	}

	// Conservative from-date: the SDK doesn't filter by lookback, so
	// over-request by `lookback` trading days and trim the result.
	from := asOf.AddDate(0, 0, -lookback*2)
	resp, err := m.client.GetBars(ctx, symbol, from, asOf)
	if err != nil {
		return nil, err
	}
	if len(resp.Bars) == 0 {
		return nil, job.ErrNotFound
	}

	bars := make([]order.Bar, 0, len(resp.Bars))
	for _, b := range resp.Bars {
		bars = append(bars, order.Bar{
			Time:   time.UnixMilli(b.Timestamp),
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: int64(b.Volume),
		})
	}
	sort.SliceStable(bars, func(i, j int) bool { return bars[i].Time.Before(bars[j].Time) })

	// Trim to the most recent `lookback` bars at or before asOf.
	end := len(bars)
	for end > 0 && bars[end-1].Time.After(asOf) {
		end--
	}
	if end == 0 {
		return nil, job.ErrNotFound
	}
	start := end - lookback
	if start < 0 {
		start = 0
	}
	return bars[start:end], nil
}

// Universe is the subset of ETFClient functionality the constituents
// adapter needs.
type Universe interface {
	GetConstituents(ctx context.Context, symbol string) ([]Constituent, error)
}

// Constituents adapts the ETF client to the framework's
// universe.Client interface. The asOf argument is currently
// informational only - the existing client returns the current universe.
type Constituents struct {
	client Universe
}

func NewConstituents(c Universe) *Constituents { return &Constituents{client: c} }

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

// StaticBars is an in-memory market.Client used primarily by tests and
// the example strategy. It serves pre-loaded bars by date.
type StaticBars struct {
	bars map[string][]order.Bar
}

func NewStaticBars(bars map[string][]order.Bar) *StaticBars {
	out := &StaticBars{bars: map[string][]order.Bar{}}
	for s, list := range bars {
		cp := make([]order.Bar, len(list))
		copy(cp, list)
		sort.SliceStable(cp, func(i, j int) bool { return cp[i].Time.Before(cp[j].Time) })
		out.bars[s] = cp
	}
	return out
}

// Bars returns up to `lookback` bars ending on or before asOf.
func (s *StaticBars) Bars(_ context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error) {
	if lookback <= 0 {
		return nil, fmt.Errorf("static-bars: lookback must be > 0")
	}
	list, ok := s.bars[symbol]
	if !ok {
		return nil, job.ErrNotFound
	}
	idx := sort.Search(len(list), func(i int) bool {
		return list[i].Time.After(asOf)
	})
	end := idx
	if end == 0 {
		return nil, job.ErrNotFound
	}
	start := end - lookback
	if start < 0 {
		start = 0
	}
	out := make([]order.Bar, end-start)
	copy(out, list[start:end])
	return out, nil
}

// StaticBarSource is a BarProvider for backtest fills backed by the
// same data as StaticBars.
type StaticBarSource struct {
	bars map[string][]order.Bar
}

func NewStaticBarSource(bars map[string][]order.Bar) *StaticBarSource {
	cp := map[string][]order.Bar{}
	for s, list := range bars {
		inner := make([]order.Bar, len(list))
		copy(inner, list)
		cp[s] = inner
	}
	return &StaticBarSource{bars: cp}
}

// Bar returns the bar for symbol on the supplied date (exact date match).
func (s *StaticBarSource) Bar(symbol string, date time.Time) (order.Bar, bool) {
	list, ok := s.bars[symbol]
	if !ok {
		return order.Bar{}, false
	}
	for _, b := range list {
		if sameBarDate(b.Time, date) {
			return b, true
		}
	}
	return order.Bar{}, false
}

func sameBarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// StaticConstituents is an in-memory universe.Client used primarily by
// tests and the example strategy.
type StaticConstituents struct {
	byDate map[string]map[string][]string
}

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
		return nil, job.ErrNotFound
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
		return nil, job.ErrNotFound
	}
	out := make([]string, len(byDate[bestDate]))
	copy(out, byDate[bestDate])
	return out, nil
}
