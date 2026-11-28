package clients

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

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

// BarsBatch returns up to `lookback` bars for each of the symbols.
//
// A symbol the store holds no bars for is left out of the map.
func (s *StaticBars) BarsBatch(ctx context.Context, symbols []string, lookback int, asOf time.Time) (map[string][]order.Bar, error) {
	if lookback <= 0 {
		return nil, fmt.Errorf("static-bars: lookback must be > 0")
	}
	out := make(map[string][]order.Bar, len(symbols))
	for _, symbol := range symbols {
		bars, err := s.Bars(ctx, symbol, lookback, asOf)
		if err != nil {
			continue
		}
		out[symbol] = bars
	}
	return out, nil
}

// Bar returns the bar for a symbol on the given date.
//
// It lets the in-memory bars serve as a fill model's price source, so
// the strategy and the fill model see the same prices.
func (s *StaticBars) Bar(symbol string, date time.Time) (order.Bar, bool) {
	list, ok := s.bars[symbol]
	if !ok {
		return order.Bar{}, false
	}
	for _, b := range list {
		if sameDay(b.Timestamp, date) {
			return b, true
		}
	}
	return order.Bar{}, false
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
