package calendar_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/calendar"
)

// fakeCalendar stands in for the market-data-service SDK client.
type fakeCalendar struct {
	days  map[string]*marketdata.MarketCalendarResponse
	err   error
	calls int
}

func (f *fakeCalendar) GetMarketCalendar(_ context.Context, date time.Time) (*marketdata.MarketCalendarResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	key := date.Format("2006-01-02")
	resp, ok := f.days[key]
	if !ok {
		return nil, fmt.Errorf("no calendar for %s", key)
	}
	return resp, nil
}

// stubProvider is a provider that answers every day as a full session.
type stubProvider struct{}

func (stubProvider) TradingDay(time.Time) (bool, error) { return true, nil }
func (stubProvider) EarlyClose(time.Time) (bool, error) { return false, nil }
func (stubProvider) SessionOpen(time.Time) (time.Time, error) {
	return time.Date(2024, time.January, 3, 9, 30, 0, 0, time.UTC), nil
}
func (stubProvider) SessionClose(time.Time) (time.Time, error) {
	return time.Date(2024, time.January, 3, 16, 0, 0, 0, time.UTC), nil
}

// day builds a service answer for a date.
func day(date string, status string, trading bool, earlyClose bool, open, close string) *marketdata.MarketCalendarResponse {
	resp := &marketdata.MarketCalendarResponse{
		Date:         date + "T00:00:00Z",
		Status:       status,
		IsTradingDay: trading,
		IsEarlyClose: earlyClose,
	}
	if open != "" {
		resp.Open = &open
	}
	if close != "" {
		resp.Close = &close
	}
	return resp
}

func nyDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestService_TradingDay(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-01-03": day("2024-01-03", "regular", true, false, "2024-01-03T09:30:00-05:00", "2024-01-03T16:00:00-05:00"),
		"2024-01-06": day("2024-01-06", "weekend", false, false, "", ""),
	}}
	svc := calendar.NewService(src)

	trading, err := svc.TradingDay(nyDate(2024, time.January, 3))
	if err != nil {
		t.Fatalf("TradingDay: %v", err)
	}
	if !trading {
		t.Fatal("2024-01-03 should be a trading day")
	}

	trading, err = svc.TradingDay(nyDate(2024, time.January, 6))
	if err != nil {
		t.Fatalf("TradingDay: %v", err)
	}
	if trading {
		t.Fatal("2024-01-06 should not be a trading day")
	}
}

func TestService_EarlyClose(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-11-29": day("2024-11-29", "early_close", true, true, "2024-11-29T09:30:00-05:00", "2024-11-29T13:00:00-05:00"),
	}}
	svc := calendar.NewService(src)

	early, err := svc.EarlyClose(nyDate(2024, time.November, 29))
	if err != nil {
		t.Fatalf("EarlyClose: %v", err)
	}
	if !early {
		t.Fatal("2024-11-29 should be an early close")
	}
}

func TestService_SessionClose(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-11-29": day("2024-11-29", "early_close", true, true, "2024-11-29T09:30:00-05:00", "2024-11-29T13:00:00-05:00"),
	}}
	svc := calendar.NewService(src)

	close, err := svc.SessionClose(nyDate(2024, time.November, 29))
	if err != nil {
		t.Fatalf("SessionClose: %v", err)
	}
	if close.Hour() != 13 || close.Minute() != 0 {
		t.Fatalf("SessionClose = %v, want 13:00 ET", close)
	}
}

// A day on which the exchange held no session has no open and no
// close: the service does not report session times for it, and the
// framework does not invent them.
func TestService_SessionTimesOnANonSessionDay(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-01-06": day("2024-01-06", "weekend", false, false, "", ""),
	}}
	svc := calendar.NewService(src)

	if _, err := svc.SessionClose(nyDate(2024, time.January, 6)); err == nil {
		t.Fatal("expected an error for the close of a day with no session")
	}
	if _, err := svc.SessionOpen(nyDate(2024, time.January, 6)); err == nil {
		t.Fatal("expected an error for the open of a day with no session")
	}
}

func TestService_SessionOpen(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-01-03": day("2024-01-03", "regular", true, false, "2024-01-03T09:30:00-05:00", "2024-01-03T16:00:00-05:00"),
	}}
	svc := calendar.NewService(src)

	open, err := svc.SessionOpen(nyDate(2024, time.January, 3))
	if err != nil {
		t.Fatalf("SessionOpen: %v", err)
	}
	if open.Hour() != 9 || open.Minute() != 30 {
		t.Fatalf("SessionOpen = %v, want 09:30 ET", open)
	}
}

// The service is asked once per date: a backtest asks about the same
// dates again and again.
func TestService_CachesAnswers(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-01-03": day("2024-01-03", "regular", true, false, "2024-01-03T09:30:00-05:00", "2024-01-03T16:00:00-05:00"),
	}}
	svc := calendar.NewService(src)

	for i := 0; i < 3; i++ {
		if _, err := svc.TradingDay(nyDate(2024, time.January, 3)); err != nil {
			t.Fatalf("TradingDay: %v", err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("service called %d times, want 1", src.calls)
	}
}

// A service failure is reported, not papered over with the framework's
// own rules: trading on a schedule that could not be confirmed is
// worse than not trading.
func TestService_FailureIsReported(t *testing.T) {
	boom := errors.New("service is down")
	src := &fakeCalendar{err: boom}
	svc := calendar.NewService(src)

	trading, err := svc.TradingDay(nyDate(2024, time.January, 3))
	if err == nil {
		t.Fatal("expected an error when the service fails")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if trading {
		t.Fatal("a failed lookup should not report a trading day")
	}
}

// The service answers for the date it was asked about; an answer about
// another date is a bug worth failing on.
func TestService_AnswerForAnotherDateIsRejected(t *testing.T) {
	src := &fakeCalendar{days: map[string]*marketdata.MarketCalendarResponse{
		"2024-01-03": day("2024-01-04", "regular", true, false, "2024-01-04T09:30:00-05:00", "2024-01-04T16:00:00-05:00"),
	}}
	svc := calendar.NewService(src)

	if _, err := svc.TradingDay(nyDate(2024, time.January, 3)); err == nil {
		t.Fatal("expected an error when the service answers about another date")
	}
}
