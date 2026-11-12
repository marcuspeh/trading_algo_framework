package clients

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

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
//
// The SDK's GetBars range is [from, to] inclusive and the service
// interprets both as NYSE calendar dates, so the request is anchored on
// the NYSE calendar: `to` is the trading day of asOf, and `from` reaches
// back far enough to cover `lookback` trading days once weekends and
// holidays are discounted. The result is then trimmed to the most
// recent `lookback` bars.
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

	// The service only serves trading days, so the window has to be
	// wider than `lookback` calendar days to yield `lookback` bars.
	// 1.5 days per bar covers weekends; the extra 10 covers the
	// Christmas and New Year closures, the longest stretch the exchange
	// takes.
	asOfNYSE := clock.NYSETime(asOf)
	from := asOfNYSE.AddDate(0, 0, -(lookback*3/2 + 10))

	resp, err := m.client.GetBars(ctx, symbol, from, asOfNYSE)
	if err != nil {
		return nil, err
	}
	if len(resp.Bars) == 0 {
		return nil, job.ErrNotFound
	}

	bars := make([]order.Bar, 0, len(resp.Bars))
	for _, b := range resp.Bars {
		bars = append(bars, order.Bar{
			Symbol: symbol,
			// The service stamps daily bars at NYSE midnight, so the
			// bar's calendar date is its trading day.
			Timestamp: time.UnixMilli(b.Timestamp).In(clock.NYSE),
			Open:      b.Open,
			High:      b.High,
			Low:       b.Low,
			Close:     b.Close,
			Volume:    b.Volume,
		})
	}
	sort.SliceStable(bars, func(i, j int) bool { return bars[i].Timestamp.Before(bars[j].Timestamp) })

	// Trim to the most recent `lookback` bars at or before asOf.
	end := len(bars)
	for end > 0 && bars[end-1].Timestamp.After(asOfNYSE) {
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

// Universe is the subset of marketdata.Client functionality the
// constituents adapter needs.
type Universe interface {
	GetConstituents(ctx context.Context, symbol string, date time.Time) (*marketdata.ConstituentResponse, error)
}

// constituentsLookbackDays bounds how far back the adapter searches for
// a snapshot when the requested date has none. The service refreshes
// snapshots daily at 8:30 ET and backfills gaps over a 7-day window, so
// a short walk finds the nearest available snapshot.
const constituentsLookbackDays = 7

// Constituents adapts the market-data-service SDK to the framework's
// universe.Client interface.
//
// The service stores one snapshot per calendar day and exposes only
// exact-date lookups, so a date without a snapshot is resolved by
// walking backwards to the most recent one. That matches the
// "most recent snapshot at or before asOf" rule the in-memory
// StaticConstituents implements, so both sources agree on semantics.
type Constituents struct {
	client Universe
}

func NewConstituents(c Universe) *Constituents { return &Constituents{client: c} }

func (c *Constituents) Constituents(ctx context.Context, etfSymbol string, asOf time.Time) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("constituents adapter: nil market-data client")
	}
	if etfSymbol == "" {
		return nil, fmt.Errorf("constituents adapter: etf symbol required")
	}

	// The service keys snapshots by NYSE calendar date.
	date := clock.NYSETime(asOf)
	for i := 0; i <= constituentsLookbackDays; i++ {
		resp, err := c.client.GetConstituents(ctx, etfSymbol, date)
		if err == nil {
			out := make([]string, 0, len(resp.Constituents))
			for _, ticker := range resp.Constituents {
				if ticker == "" {
					continue
				}
				out = append(out, ticker)
			}
			return out, nil
		}
		if !isMissingSnapshot(err) {
			return nil, err
		}
		date = date.AddDate(0, 0, -1)
	}
	return nil, job.ErrNotFound
}

// isMissingSnapshot reports whether err means the service holds no
// snapshot for the requested date, as opposed to a rejected request.
//
// The SDK folds HTTP status codes into a plain error, so the 404 is
// recognizable either by the service's message or, when the response
// body isn't decodable, by the bare status. A different error - an
// unsupported symbol, say - is returned to the caller unchanged.
func isMissingSnapshot(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No constituents snapshot") ||
		strings.Contains(msg, "status: 404")
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
		for i := range cp {
			cp[i].Timestamp = nyseBarTime(cp[i].Timestamp)
		}
		sort.SliceStable(cp, func(i, j int) bool { return cp[i].Timestamp.Before(cp[j].Timestamp) })
		out.bars[s] = cp
	}
	return out
}

// nyseBarTime re-anchors a bar's timestamp to NYSE midnight, keeping
// the calendar date it was written with.
//
// Bars are daily, so only their trading day matters. Anchoring every
// bar on one calendar lets the framework compare bar dates against the
// clock's date without zone-dependent off-by-one errors.
func nyseBarTime(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, clock.NYSE)
}

// Bars returns up to `lookback` bars ending on or before asOf.
//
// Bars are matched on calendar date, so a bar stamped at any time of
// day counts as belonging to that date.
func (s *StaticBars) Bars(_ context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error) {
	if lookback <= 0 {
		return nil, fmt.Errorf("static-bars: lookback must be > 0")
	}
	list, ok := s.bars[symbol]
	if !ok {
		return nil, job.ErrNotFound
	}
	idx := sort.Search(len(list), func(i int) bool {
		return afterDate(list[i].Timestamp, asOf)
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

// afterDate reports whether bar's calendar date falls after date's
// calendar date.
func afterDate(bar, date time.Time) bool {
	by, bm, bd := bar.Date()
	dy, dm, dd := date.Date()
	if by != dy {
		return by > dy
	}
	if bm != dm {
		return bm > dm
	}
	return bd > dd
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
		for i := range inner {
			inner[i].Timestamp = nyseBarTime(inner[i].Timestamp)
		}
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
		if sameBarDate(b.Timestamp, date) {
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
