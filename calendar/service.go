package calendar

import (
	"context"
	"fmt"
	"sync"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
)

// DefaultServiceTimeout bounds a call to the market-data service. The
// SDK sets no timeout on its HTTP client, so without one a stalled
// service would hang the caller indefinitely.
const DefaultServiceTimeout = 30 * time.Second

// CalendarSource is the part of the market-data-service SDK the
// calendar needs.
type CalendarSource interface {
	GetMarketCalendar(ctx context.Context, date time.Time) (*marketdata.MarketCalendarResponse, error)
}

// Service is a Provider backed by the market-data-service.
//
// The service answers for a single calendar date, so every date is one
// request. Answers are cached for the life of the provider: a backtest
// walks the calendar one day at a time and asks about the same dates
// again and again.
//
// A service failure is returned to the caller rather than papered over
// with a local copy of the rules - the whole point of using the
// service is to trade on the exchange's published schedule, and a
// silent fallback would hide the moment that schedule is unavailable.
type Service struct {
	client  CalendarSource
	timeout time.Duration

	mu    sync.Mutex
	cache map[string]*marketdata.MarketCalendarResponse
}

// Compile-time proof that the service satisfies the provider
// interface.
var _ Provider = (*Service)(nil)

// NewService returns a Provider that answers from the market-data
// service.
func NewService(c CalendarSource) *Service {
	return &Service{
		client:  c,
		timeout: DefaultServiceTimeout,
		cache:   map[string]*marketdata.MarketCalendarResponse{},
	}
}

// WithTimeout sets the deadline applied to each call to the service.
func (s *Service) WithTimeout(d time.Duration) *Service {
	if d > 0 {
		s.timeout = d
	}
	return s
}

// TradingDay reports whether the NYSE held a session on the date.
func (s *Service) TradingDay(date time.Time) (bool, error) {
	resp, err := s.describe(date)
	if err != nil {
		return false, err
	}
	return resp.IsTradingDay, nil
}

// EarlyClose reports whether the session on the date was shortened.
func (s *Service) EarlyClose(date time.Time) (bool, error) {
	resp, err := s.describe(date)
	if err != nil {
		return false, err
	}
	return resp.IsEarlyClose, nil
}

// SessionOpen returns the open of the session on the date.
//
// The service reports session times only for a day the exchange traded
// on, so a day it was closed is an error rather than a made-up time.
func (s *Service) SessionOpen(date time.Time) (time.Time, error) {
	resp, err := s.describe(date)
	if err != nil {
		return time.Time{}, err
	}
	if resp.Open == nil {
		return time.Time{}, fmt.Errorf("%w: %s", ErrNoSession, resp.DateOnly())
	}
	open, err := time.Parse(time.RFC3339, *resp.Open)
	if err != nil {
		return time.Time{}, fmt.Errorf("calendar: unparsable open %q for %s: %w", *resp.Open, resp.DateOnly(), err)
	}
	return open, nil
}

// SessionClose returns the close of the session on the date.
//
// The service reports session times only for a day the exchange traded
// on, so a day it was closed is an error rather than a made-up time.
func (s *Service) SessionClose(date time.Time) (time.Time, error) {
	resp, err := s.describe(date)
	if err != nil {
		return time.Time{}, err
	}
	if resp.Close == nil {
		return time.Time{}, fmt.Errorf("%w: %s", ErrNoSession, resp.DateOnly())
	}
	close, err := time.Parse(time.RFC3339, *resp.Close)
	if err != nil {
		return time.Time{}, fmt.Errorf("calendar: unparsable close %q for %s: %w", *resp.Close, resp.DateOnly(), err)
	}
	return close, nil
}

// describe returns the service's answer for the date, fetching it the
// first time the date is asked about.
func (s *Service) describe(date time.Time) (*marketdata.MarketCalendarResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("calendar: nil market-data client")
	}

	key := dateKey(date)
	s.mu.Lock()
	cached, ok := s.cache[key]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	resp, err := s.client.GetMarketCalendar(ctx, serviceDate(date))
	if err != nil {
		return nil, fmt.Errorf("calendar: market-data service: %w", err)
	}
	if got := resp.DateOnly(); got != key {
		return nil, fmt.Errorf("calendar: market-data service answered for %s, asked about %s", got, key)
	}

	s.mu.Lock()
	s.cache[key] = resp
	s.mu.Unlock()
	return resp, nil
}

// dateKey is the calendar day of the date as written, in YYYY-MM-DD
// form. The date is taken as written rather than converted, matching
// the rest of the package.
func dateKey(date time.Time) string {
	y, m, d := date.Date()
	return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
}

// serviceDate is the date to ask the service about.
//
// The service reads the date as a US/Eastern calendar day, so the date
// is re-expressed on the NYSE calendar before it goes out. The fields
// are copied rather than converted, which keeps the day the caller
// wrote even when the time was carried in another zone.
func serviceDate(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, NYSE)
}
