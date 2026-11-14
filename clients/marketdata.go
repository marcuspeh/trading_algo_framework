package clients

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/order"
)

// DefaultMarketTimeout bounds a call to the market-data service. The
// SDK sets no timeout on its HTTP client, so without one a stalled
// service would hang the caller indefinitely.
const DefaultMarketTimeout = 30 * time.Second

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
//
// The adapter keeps the bars it has fetched. A backtest walks the
// calendar one day at a time, and without a cache every day would
// re-read the whole window from the service; with one, each day costs
// a single bar.
type Market struct {
	client  MarketSource
	timeout time.Duration

	mu       sync.Mutex
	cache    map[string][]order.Bar
	coverage map[string]*coverage
}

// Compile-time proof that the adapter can also supply prices to a
// fill model.
var _ market.BarProvider = (*Market)(nil)

func NewMarket(c MarketSource) *Market {
	return &Market{
		client:   c,
		timeout:  DefaultMarketTimeout,
		cache:    map[string][]order.Bar{},
		coverage: map[string]*coverage{},
	}
}

// WithTimeout sets the deadline applied to each call to the service.
func (m *Market) WithTimeout(d time.Duration) *Market {
	if d > 0 {
		m.timeout = d
	}
	return m
}

// Bars fetches up to `lookback` bars for symbol ending on or before asOf.
//
// The SDK's GetBars range is [from, to] inclusive and the service
// interprets both as NYSE calendar dates, so the request is anchored on
// the NYSE calendar: `to` is the trading day of asOf, and `from` is the
// trading day `lookback` sessions earlier. The result is then trimmed to
// the most recent `lookback` bars.
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

	from, to := windowFor(lookback, asOf)

	bars, err := m.barsIn(ctx, symbol, from, to)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, job.ErrNotFound
	}

	// Trim to the most recent `lookback` bars at or before asOf.
	end := len(bars)
	for end > 0 && bars[end-1].Timestamp.After(to) {
		end--
	}
	if end == 0 {
		return nil, job.ErrNotFound
	}
	start := end - lookback
	if start < 0 {
		start = 0
	}
	out := make([]order.Bar, end-start)
	copy(out, bars[start:end])
	return out, nil
}

// Bar returns the bar for a symbol on the given date.
//
// It is the fill model's view of the market, and it reads the same
// cached series the strategy reads, so the two can never disagree
// about a price.
func (m *Market) Bar(symbol string, date time.Time) (order.Bar, bool) {
	if m.client == nil || symbol == "" {
		return order.Bar{}, false
	}

	day := nyseDay(date)

	m.mu.Lock()
	cached := m.cache[symbol]
	m.mu.Unlock()

	if bar, ok := barOn(cached, day); ok {
		return bar, true
	}

	// The bar is not cached. Fetch a window around the date and look
	// again.
	from := day.AddDate(0, 0, -10)
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	bars, err := m.barsIn(ctx, symbol, from, day)
	if err != nil {
		return order.Bar{}, false
	}
	return barOn(bars, day)
}

// barsIn returns the cached bars for a symbol, fetching whatever is
// missing.
//
// The caller must not hold the lock.
func (m *Market) barsIn(ctx context.Context, symbol string, from, to time.Time) ([]order.Bar, error) {
	m.mu.Lock()
	cached, entry := m.cache[symbol], m.coverage[symbol]
	m.mu.Unlock()

	// Only the part of the window the service has not already been
	// asked about is fetched. A backtest advancing one day at a time
	// therefore costs one bar per day rather than a whole window.
	needFrom, needTo := from, to
	switch {
	case entry == nil:
		// Nothing cached.
	case !entry.from.After(from) && !entry.to.Before(to):
		// The service has already answered for this whole window.
		needFrom, needTo = to.AddDate(0, 0, 1), to
	case entry.from.After(from) && entry.to.Before(to):
		// The window straddles the cache: refetch it whole.
	case entry.from.After(from):
		// The request reaches further back than the cache does.
		needFrom, needTo = from, entry.from.AddDate(0, 0, -1)
	default:
		// The cache covers the start of the window; only the tail is
		// missing.
		needFrom, needTo = entry.to.AddDate(0, 0, 1), to
	}

	var fetched []order.Bar
	if !needFrom.After(needTo) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		resp, err := m.client.GetBars(ctx, symbol, needFrom, needTo)
		if err != nil {
			return nil, err
		}
		fetched = make([]order.Bar, 0, len(resp.Bars))
		for _, b := range resp.Bars {
			fetched = append(fetched, order.Bar{
				Symbol: symbol,
				// The service stamps daily bars at NYSE midnight, so
				// the bar's calendar date is its trading day.
				Timestamp: time.UnixMilli(b.Timestamp).In(clock.NYSE),
				Open:      b.Open,
				High:      b.High,
				Low:       b.Low,
				Close:     b.Close,
				Volume:    b.Volume,
			})
		}
	}

	merged := mergeBars(cached, fetched)

	m.mu.Lock()
	m.cache[symbol] = merged
	m.coverage[symbol] = mergeCoverage(entry, needFrom, needTo)
	m.mu.Unlock()

	return merged, nil
}

// coverage records the range of dates the service has been asked
// about for a symbol.
//
// The range matters as much as the bars: the service may hold no bar
// for a date - before the data starts, say - and without knowing that
// the adapter would ask for the same empty stretch again and again.
type coverage struct {
	from time.Time
	to   time.Time
}

// mergeCoverage widens the covered range to include a fetched one.
func mergeCoverage(c *coverage, from, to time.Time) *coverage {
	if c == nil {
		return &coverage{from: from, to: to}
	}
	out := *c
	if from.Before(out.from) {
		out.from = from
	}
	if to.After(out.to) {
		out.to = to
	}
	return &out
}

// mergeBars combines two sorted series into one, dropping duplicates.
func mergeBars(a, b []order.Bar) []order.Bar {
	out := make([]order.Bar, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})

	deduped := out[:0]
	for i, bar := range out {
		if i > 0 && bar.Timestamp.Equal(out[i-1].Timestamp) {
			// A later copy wins: the freshly fetched bar is the more
			// current of the two.
			deduped[len(deduped)-1] = bar
			continue
		}
		deduped = append(deduped, bar)
	}
	return deduped
}

// barOn finds the bar for a date in a series.
func barOn(bars []order.Bar, day time.Time) (order.Bar, bool) {
	for _, b := range bars {
		if sameDay(b.Timestamp, day) {
			return b, true
		}
	}
	return order.Bar{}, false
}

// sameDay reports whether two times fall on the same calendar date.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// nyseDay returns midnight of the given date on the NYSE calendar.
func nyseDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, clock.NYSE)
}

// windowMargin is how many extra sessions the request window reaches
// back beyond the ones asked for.
//
// The most recent session's bar may not be published yet - the service
// publishes a session's bar once it has closed - so the window has to
// reach a little further back to be sure of finding the requested
// number of bars.
const windowMargin = 5

// windowFor returns the calendar range covering `lookback` trading
// days ending on or before asOf.
//
// The window is computed with the trading calendar rather than a
// day-count estimate, so a request for 20 bars asks for about 20
// sessions' worth of range rather than a fixed multiple of it.
func windowFor(lookback int, asOf time.Time) (from, to time.Time) {
	to = nyseDay(asOf)

	want := lookback + windowMargin
	candidate := to
	count := 0
	for i := 0; i < want*3+30; i++ {
		if calendar.TradingDay(candidate) {
			count++
			if count == want {
				break
			}
		}
		candidate = candidate.AddDate(0, 0, -1)
	}
	return candidate, to
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

// Constituents returns the constituents of an ETF as of the given date.
//
// The date is the day the snapshot was published, not the session the
// caller is trading: the service refreshes snapshots at 08:30 ET, so
// today's holdings are public before the open and there is nothing to
// look ahead to.
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
